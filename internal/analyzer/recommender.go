package analyzer

import (
	"fmt"
	"math"
)

// CloudCostBreakdown details pricing across distinct cloud categories
type CloudCostBreakdown struct {
	BudgetVPS   string `json:"budget_vps"`  // e.g. Hetzner Cloud / DO Droplet
	PaaS        string `json:"paas"`        // e.g. Render / Fly.io / Railway
	Hyperscaler string `json:"hyperscaler"` // e.g. AWS ECS/Fargate + RDS
}

// HardwareRecommendation details recommended production cloud sizing
type HardwareRecommendation struct {
	TargetVUs         int
	MaxObservedVUs    int
	SustainableVUs    int
	SafetyHeadroomPct float64

	IsRemoteTarget  bool
	SizingRationale string

	APITierName       string
	APIVCPU           string
	APIRAM            string
	APIPeakCPUPercent float64
	APIPeakMemoryMB   float64

	PostgresVCPU   string
	PostgresRAM    string
	PostgresAdvice string

	RedisVCPU string
	RedisRAM  string

	Costs           CloudCostBreakdown
	APICostEst      string
	PostgresCostEst string
	RedisCostEst    string
	TotalCostEst    string

	Overprovisioned      bool
	OverprovisionWarning string
}

// ComputeRecommendation calculates candidate starting hardware recommendations based on test findings.
func ComputeRecommendation(
	targetVUs int,
	maxObservedVUs int,
	peakAPICPU float64,
	peakAPIMemMB float64,
	pgActiveConns int,
	pgMaxConns int,
	redisMemMB float64,
	safetyFactor float64,
) HardwareRecommendation {
	if safetyFactor <= 0 || safetyFactor > 1.0 {
		safetyFactor = 0.70
	}
	headroomPct := math.Round((1.0-safetyFactor)*1000.0) / 10.0
	sustainable := int(float64(maxObservedVUs) * safetyFactor)
	isRemote := (peakAPICPU == 0 && peakAPIMemMB == 0)

	rec := HardwareRecommendation{
		TargetVUs:         targetVUs,
		MaxObservedVUs:    maxObservedVUs,
		SustainableVUs:    sustainable,
		SafetyHeadroomPct: headroomPct,
		IsRemoteTarget:    isRemote,
		APIPeakCPUPercent: peakAPICPU,
		APIPeakMemoryMB:   peakAPIMemMB,
	}

	if isRemote {
		rec.SizingRationale = fmt.Sprintf("Candidate estimate for %d concurrent users (~%d sustainable operating load with %.0f%% operational headroom). For container CPU/RAM cgroups, profile locally via Docker.", maxObservedVUs, sustainable, headroomPct)
	} else {
		rec.SizingRationale = fmt.Sprintf("Derived from measured container peaks (%.1f%% CPU, %.1f MB RAM) with %.0f%% operational headroom.", peakAPICPU, peakAPIMemMB, headroomPct)
	}

	// 1. Calculate API vCPU based on peak CPU and target ~60% load
	var vcpu float64 = 1.0
	if peakAPICPU > 0 {
		vcpu = math.Ceil((peakAPICPU/60.0)*2.0) / 2.0
	}
	if vcpu < 0.5 {
		vcpu = 0.5
	}

	// 2. Calculate API RAM with 50% buffer over peak usage
	var ramMB float64 = 512
	if peakAPIMemMB > 0 {
		ramMB = peakAPIMemMB * 1.5
	}

	// Map API to standard Cloud Tiers
	if vcpu <= 0.5 && ramMB <= 512 {
		rec.APITierName = "Micro Tier"
		rec.APIVCPU = "0.5 vCPU"
		rec.APIRAM = "512 MB RAM"
	} else if vcpu <= 1.0 && ramMB <= 1024 {
		rec.APITierName = "Small Tier"
		rec.APIVCPU = "1.0 vCPU"
		rec.APIRAM = "1 GB RAM"
	} else if vcpu > 1.0 && ramMB <= 2048 {
		rec.APITierName = "Compute-Optimized Tier"
		rec.APIVCPU = fmt.Sprintf("%.1f vCPU", vcpu)
		rec.APIRAM = fmt.Sprintf("%.0f GB RAM", math.Max(math.Ceil(ramMB/1024), 1.0))
	} else if vcpu <= 1.0 && ramMB > 2048 {
		rec.APITierName = "Memory-Optimized Tier"
		rec.APIVCPU = "1.0 vCPU"
		rec.APIRAM = fmt.Sprintf("%.0f GB RAM", math.Ceil(ramMB/1024))
	} else {
		rec.APITierName = "Medium Tier"
		rec.APIVCPU = fmt.Sprintf("%.1f vCPU", math.Max(vcpu, 2.0))
		rec.APIRAM = fmt.Sprintf("%.0f GB RAM", math.Max(math.Ceil(ramMB/1024), 2.0))
	}

	// 3. PostgreSQL Sizing
	pgVCPU := "1.0 vCPU"
	pgRAM := "1 GB RAM"
	pgAdvice := "Standard connection pool adequate"
	if pgActiveConns > 20 || pgMaxConns >= 100 {
		pgRAM = "2 GB RAM"
		pgAdvice = fmt.Sprintf("Recommended max_connections: %d + PgBouncer connection pooler", int(math.Max(float64(pgActiveConns)*1.5, 50)))
	}
	if pgActiveConns > 50 {
		pgVCPU = "2.0 vCPU"
		pgRAM = "4 GB RAM"
	}
	rec.PostgresVCPU = pgVCPU
	rec.PostgresRAM = pgRAM
	rec.PostgresAdvice = pgAdvice

	// 4. Redis Sizing
	if redisMemMB > 200 {
		rec.RedisVCPU = "0.5 vCPU"
		rec.RedisRAM = fmt.Sprintf("%d MB RAM", int(math.Ceil(redisMemMB*1.5)))
	} else {
		rec.RedisVCPU = "0.25 vCPU"
		rec.RedisRAM = "256 MB RAM"
	}

	// 5. Cloud Cost Estimates
	switch rec.APITierName {
	case "Micro Tier":
		rec.Costs = CloudCostBreakdown{
			BudgetVPS:   "$4–$6/mo (Hetzner CAX11 / DO $4)",
			PaaS:        "$7–$14/mo (Render Starter / Fly.io 256MB)",
			Hyperscaler: "$15–$25/mo (AWS t4g.nano + RDS micro)",
		}
		rec.APICostEst = "$4/mo (e.g. Hetzner CAX11 / DO $4 Droplet)"
		rec.PostgresCostEst = "$10/mo (1 GB node / Managed DB)"
		rec.RedisCostEst = "$5/mo"
		rec.TotalCostEst = "$19/mo"
	case "Small Tier":
		rec.Costs = CloudCostBreakdown{
			BudgetVPS:   "$6–$10/mo (Hetzner CX22 / DO 1GB)",
			PaaS:        "$14–$21/mo (Render Starter Web + DB / Fly.io)",
			Hyperscaler: "$25–$45/mo (AWS t4g.small + RDS db.t4g.micro)",
		}
		rec.APICostEst = "$6–$8/mo (e.g. Hetzner CX22 / DO 1GB / AWS t4g.small)"
		rec.PostgresCostEst = "$15/mo (2 GB node / Managed DB)"
		rec.RedisCostEst = "$5/mo"
		rec.TotalCostEst = "$26–$28/mo"
	case "Compute-Optimized Tier":
		rec.Costs = CloudCostBreakdown{
			BudgetVPS:   "$12–$20/mo (Hetzner CPX21 / DO 2vCPU)",
			PaaS:        "$25–$40/mo (Render Standard / Railway Team)",
			Hyperscaler: "$45–$80/mo (AWS c6g.medium + RDS db.t4g.small)",
		}
		rec.APICostEst = "$14–$20/mo (e.g. Hetzner CPX21 / DO 2vCPU / AWS c6g.medium)"
		rec.PostgresCostEst = "$15–$25/mo"
		rec.RedisCostEst = "$5–$10/mo"
		rec.TotalCostEst = "$34–$55/mo"
	case "Memory-Optimized Tier":
		rec.Costs = CloudCostBreakdown{
			BudgetVPS:   "$14–$24/mo (Hetzner CX32 / DO 4GB)",
			PaaS:        "$30–$50/mo (Render Pro / Fly.io 4GB)",
			Hyperscaler: "$50–$90/mo (AWS r6g.medium + RDS db.t4g.small)",
		}
		rec.APICostEst = "$12–$24/mo (e.g. Hetzner CX32 / DO 4GB / AWS r6g.medium)"
		rec.PostgresCostEst = "$20–$30/mo"
		rec.RedisCostEst = "$5–$10/mo"
		rec.TotalCostEst = "$37–$64/mo"
	default:
		rec.Costs = CloudCostBreakdown{
			BudgetVPS:   "$20–$35/mo (Hetzner CPX31 / DO 4GB)",
			PaaS:        "$45–$75/mo (Render Pro Plus / Railway Business)",
			Hyperscaler: "$70–$130/mo (AWS t4g.medium + RDS db.m6g.large)",
		}
		rec.APICostEst = "$18–$30/mo (e.g. Hetzner CPX31 / DO 4GB / AWS t4g.medium)"
		rec.PostgresCostEst = "$25–$40/mo"
		rec.RedisCostEst = "$10/mo"
		rec.TotalCostEst = "$53–$80/mo"
	}

	// 6. Over-provisioning check
	if targetVUs > 0 && sustainable > targetVUs*2 {
		rec.Overprovisioned = true
		rec.OverprovisionWarning = fmt.Sprintf("System exceeds target load by %d%% (Cost optimization opportunity: downscale to save cloud spend).",
			int(((float64(sustainable)-float64(targetVUs))/float64(targetVUs))*100))
	}

	return rec
}
