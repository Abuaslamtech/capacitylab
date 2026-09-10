package analyzer

import (
	"fmt"

	"github.com/Abuaslamtech/capacitylab/internal/config"
	"github.com/Abuaslamtech/capacitylab/internal/load"
	"github.com/Abuaslamtech/capacitylab/internal/monitor"
	"github.com/Abuaslamtech/capacitylab/internal/runtime"
)

// BottleneckItem represents a diagnosed system constraint
type BottleneckItem struct {
	Component   string // e.g. "API Compute", "PostgreSQL Pool", "Redis Memory"
	Severity    string // "CRITICAL", "WARNING", "HEALTHY"
	Summary     string // Descriptive explanation of the issue
	Remediation string // Concrete, actionable recommendation
}

// BottleneckReport contains the full cross-service diagnostic breakdown
type BottleneckReport struct {
	HasBreach   bool
	Primary     BottleneckItem
	Secondary   []BottleneckItem
	NonLimiting []string
}

// StateSnapshot captures system telemetry at the moment of evaluation
type StateSnapshot struct {
	LoadMetrics load.StageMetrics
	APIMetrics  runtime.ContainerMetrics
	PGMetrics   *monitor.PostgresMetrics
	RedisMetrics *monitor.RedisMetrics
}

// Classifier correlates cross-service metrics to diagnose root causes
type Classifier struct {
	thresholds config.Thresholds
}

// NewClassifier initializes a bottleneck evaluation engine
func NewClassifier(thresholds config.Thresholds) *Classifier {
	return &Classifier{
		thresholds: thresholds,
	}
}

// Diagnose analyzes the telemetry snapshot and classifies the primary constraint
func (c *Classifier) Diagnose(snap StateSnapshot) BottleneckReport {
	report := BottleneckReport{
		HasBreach: false,
	}

	var candidates []BottleneckItem
	var nonLimiting []string

	// ─── 1. Check PostgreSQL Connection Pool Starvation (Trap 5) ───
	if snap.PGMetrics != nil && snap.PGMetrics.MaxConnections > 0 {
		active := snap.PGMetrics.ActiveConnections
		max := snap.PGMetrics.MaxConnections
		ratio := float64(active) / float64(max)

		if ratio >= 0.90 || (active >= max && max > 0) {
			candidates = append(candidates, BottleneckItem{
				Component:   "PostgreSQL Connection Pool",
				Severity:    "CRITICAL",
				Summary:     fmt.Sprintf("Database connections exhausted (%d/%d active). Incoming queries queued.", active, max),
				Remediation: "Enlarge the backend database connection pool or deploy PgBouncer connection pooler.",
			})
		} else if ratio > 0.70 {
			candidates = append(candidates, BottleneckItem{
				Component:   "PostgreSQL Connection Pool",
				Severity:    "WARNING",
				Summary:     fmt.Sprintf("High connection utilization (%d/%d active, %.0f%%).", active, max, ratio*100),
				Remediation: "Monitor pool growth; approaching max_connections ceiling.",
			})
		} else {
			nonLimiting = append(nonLimiting, fmt.Sprintf("PostgreSQL Connections (%d/%d active)", active, max))
		}
	}

	// ─── 2. Check PostgreSQL Lock Contention & Cache Misses ───
	if snap.PGMetrics != nil {
		if snap.PGMetrics.WaitingLocks > 5 {
			candidates = append(candidates, BottleneckItem{
				Component:   "PostgreSQL Lock Contention",
				Severity:    "CRITICAL",
				Summary:     fmt.Sprintf("Detected %d blocked queries waiting on database table/row locks.", snap.PGMetrics.WaitingLocks),
				Remediation: "Optimize transaction lengths and inspect queries holding exclusive locks.",
			})
		}
		if snap.PGMetrics.CacheHitRatio > 0 && snap.PGMetrics.CacheHitRatio < 80.0 {
			candidates = append(candidates, BottleneckItem{
				Component:   "PostgreSQL Buffer Cache",
				Severity:    "WARNING",
				Summary:     fmt.Sprintf("Low buffer cache hit ratio (%.1f%%). Reading data from disk I/O.", snap.PGMetrics.CacheHitRatio),
				Remediation: "Increase shared_buffers in postgresql.conf or index frequently queried columns.",
			})
		} else if snap.PGMetrics.CacheHitRatio >= 90.0 {
			nonLimiting = append(nonLimiting, fmt.Sprintf("PostgreSQL Cache (%.1f%% hit ratio)", snap.PGMetrics.CacheHitRatio))
		}
	}

	// ─── 3. Check API Container CPU Saturation ───
	if snap.APIMetrics.CPUPercent >= c.thresholds.MaxCPUPercent {
		candidates = append(candidates, BottleneckItem{
			Component:   "API Compute",
			Severity:    "CRITICAL",
			Summary:     fmt.Sprintf("Container CPU utilization reached %.1f%% (exceeded %.0f%% limit).", snap.APIMetrics.CPUPercent, c.thresholds.MaxCPUPercent),
			Remediation: "Scale container compute allocation (increase vCPU) or optimize CPU-bound routes.",
		})
	} else if snap.APIMetrics.ThrottledPeriods > 50 {
		candidates = append(candidates, BottleneckItem{
			Component:   "API CPU Throttling",
			Severity:    "WARNING",
			Summary:     fmt.Sprintf("Linux cgroup throttled container %d times (%dµs total).", snap.APIMetrics.ThrottledPeriods, snap.APIMetrics.ThrottledTimeUs),
			Remediation: "Container breached its CFS CPU quota. Increase container CPU limit.",
		})
	} else if snap.APIMetrics.CPUPercent < 60.0 {
		nonLimiting = append(nonLimiting, fmt.Sprintf("API CPU Headroom (%.1f%% used)", snap.APIMetrics.CPUPercent))
	}

	// ─── 4. Check Application Memory (OOM Risk) ───
	if snap.APIMetrics.MemoryPercent >= c.thresholds.MaxMemoryPercent {
		candidates = append(candidates, BottleneckItem{
			Component:   "Application Memory",
			Severity:    "CRITICAL",
			Summary:     fmt.Sprintf("RAM reached %.1f%% (%.1f MB / %.1f MB). Imminent OOM kill risk.", snap.APIMetrics.MemoryPercent, snap.APIMetrics.MemoryUsedMB, snap.APIMetrics.MemoryLimitMB),
			Remediation: "Increase container memory allocation or inspect application memory heap for leaks.",
		})
	} else if snap.APIMetrics.MemoryPercent < 50.0 && snap.APIMetrics.MemoryLimitMB > 0 {
		nonLimiting = append(nonLimiting, fmt.Sprintf("Memory Footprint (%.1f MB used)", snap.APIMetrics.MemoryUsedMB))
	}

	// ─── 5. Check Redis Evictions & Cache Pressure ───
	if snap.RedisMetrics != nil {
		if snap.RedisMetrics.EvictedKeys > 0 {
			candidates = append(candidates, BottleneckItem{
				Component:   "Redis Cache Evictions",
				Severity:    "CRITICAL",
				Summary:     fmt.Sprintf("Redis evicted %d keys due to maxmemory saturation.", snap.RedisMetrics.EvictedKeys),
				Remediation: "Increase Redis memory limit or configure aggressive TTL expirations.",
			})
		} else if snap.RedisMetrics.UsedMemoryMB > 0 {
			nonLimiting = append(nonLimiting, fmt.Sprintf("Redis Memory (%.1f MB, 0 evictions)", snap.RedisMetrics.UsedMemoryMB))
		}
	}

	// ─── 6. Check Latency & Error Rate SLA Breaches ───
	if snap.LoadMetrics.P95Ms > c.thresholds.MaxP95LatencyMs {
		report.HasBreach = true
		if len(candidates) == 0 {
			// Latency spiked but no container or DB resource was exhausted
			candidates = append(candidates, BottleneckItem{
				Component:   "Downstream I/O or Event Loop Delay",
				Severity:    "CRITICAL",
				Summary:     fmt.Sprintf("p95 latency (%.1fms) breached SLA (%.0fms) while host resources were unconstrained.", snap.LoadMetrics.P95Ms, c.thresholds.MaxP95LatencyMs),
				Remediation: "Investigate synchronous blocking I/O calls, external API timeouts, or thread pool starvation.",
			})
		}
	}

	if snap.LoadMetrics.ErrorPercent > c.thresholds.MaxErrorRatePercent {
		report.HasBreach = true
		if len(candidates) == 0 {
			candidates = append(candidates, BottleneckItem{
				Component:   "Application Server Error Rate",
				Severity:    "CRITICAL",
				Summary:     fmt.Sprintf("Error rate (%.1f%%) breached SLA (%.1f%%).", snap.LoadMetrics.ErrorPercent, c.thresholds.MaxErrorRatePercent),
				Remediation: "Inspect application error logs for unhandled exceptions or 5xx responses.",
			})
		}
	}

	var criticals []BottleneckItem
	var warnings []BottleneckItem

	for _, cand := range candidates {
		if cand.Severity == "CRITICAL" {
			criticals = append(criticals, cand)
		} else {
			warnings = append(warnings, cand)
		}
	}

	if len(criticals) > 0 {
		report.HasBreach = true
		report.Primary = criticals[0]
		report.Secondary = append(criticals[1:], warnings...)
	} else {
		report.HasBreach = false
		report.Primary = BottleneckItem{
			Component:   "None",
			Severity:    "HEALTHY",
			Summary:     "All SLA thresholds were maintained across the tested range.",
			Remediation: "System is operating comfortably within current hardware parameters.",
		}
		report.Secondary = warnings
	}

	report.NonLimiting = nonLimiting
	return report
}
