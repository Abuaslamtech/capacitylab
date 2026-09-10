package engine

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Abuaslamtech/capacitylab/internal/config"
	"github.com/Abuaslamtech/capacitylab/internal/load"
	"github.com/Abuaslamtech/capacitylab/internal/runtime"
)

// TierResult stores the capacity findings for a specific hardware tier
type TierResult struct {
	CPUs                float64
	MemoryMB            int64
	MaxObservedUsers    int
	SustainableCapacity int
	Bottleneck          string
	Status              string // "Insufficient", "Recommended", "Over-provisioned"
}

// MatrixEngine evaluates progressive workloads across multiple CPU/RAM combinations
type MatrixEngine struct {
	cfg          *config.Config
	dockerClient *runtime.DockerClient
	runner       *load.Runner
}

// NewMatrixEngine initializes a multi-tier sizing runner
func NewMatrixEngine(cfg *config.Config, dockerClient *runtime.DockerClient) *MatrixEngine {
	return &MatrixEngine{
		cfg:          cfg,
		dockerClient: dockerClient,
		runner:       load.NewRunner(cfg),
	}
}

// RunMatrix tests the configured resource tiers on the target container
func (m *MatrixEngine) RunMatrix(ctx context.Context) ([]TierResult, error) {
	containerName := m.cfg.Services.API.Container
	cpuTiers := m.cfg.Services.API.TestMatrix.CPU
	memTiers := m.cfg.Services.API.TestMatrix.Memory

	if len(cpuTiers) == 0 {
		cpuTiers = []string{"0.5", "1.0"}
	}
	if len(memTiers) == 0 {
		memTiers = []string{"256MB", "512MB"}
	}

	var results []TierResult

	for _, cpuStr := range cpuTiers {
		cpus, err := strconv.ParseFloat(cpuStr, 64)
		if err != nil {
			continue
		}

		for _, memStr := range memTiers {
			memMB := ParseMemoryMB(memStr)

			fmt.Println("\n─────────────────────────────────────────────────────────────")
			fmt.Printf("⚙️  Applying Resource Limit: %.1f vCPU / %d MB RAM\n", cpus, memMB)
			fmt.Println("─────────────────────────────────────────────────────────────")

			// Dynamically update container cgroups in milliseconds
			err := m.dockerClient.UpdateResources(ctx, containerName, cpus, memMB)
			if err != nil {
				fmt.Printf("⚠️  Failed to apply cgroup limit: %v. Skipping tier.\n", err)
				continue
			}

			// Create fresh runner and wait for container network to settle
			m.runner = load.NewRunner(m.cfg)
			if err := m.waitForHealthy(ctx, 8*time.Second); err != nil {
				fmt.Printf("⚠️  Target health check warning: %v\n", err)
			}

			// Run capacity sweep for this tier
			maxUsers, bottleneck := m.evaluateTier(ctx, containerName)
			sustainable := int(float64(maxUsers) * 0.70)

			status := "Recommended"
			if sustainable < (m.cfg.Workload.MaxUsers / 10) {
				status = "Insufficient"
			} else if sustainable >= m.cfg.Workload.MaxUsers {
				status = "Over-provisioned"
			}

			results = append(results, TierResult{
				CPUs:                cpus,
				MemoryMB:            memMB,
				MaxObservedUsers:    maxUsers,
				SustainableCapacity: sustainable,
				Bottleneck:          bottleneck,
				Status:              status,
			})
		}
	}

	return results, nil
}

func (m *MatrixEngine) evaluateTier(ctx context.Context, containerName string) (int, string) {
	currentUsers := m.cfg.Workload.StartUsers
	stepDuration := 5 * time.Second
	maxObserved := 0
	bottleneck := "None detected within target range"

	for currentUsers <= m.cfg.Workload.MaxUsers {
		fmt.Printf("   Testing %3d VUs... ", currentUsers)

		var peakCPU float64
		sampleDone := make(chan bool)

		go func() {
			ticker := time.NewTicker(400 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-sampleDone:
					return
				case <-ticker.C:
					metrics, err := m.dockerClient.GetMetrics(ctx, containerName)
					if err == nil && metrics.CPUPercent > peakCPU {
						peakCPU = metrics.CPUPercent
					}
				}
			}
		}()

		metrics, err := m.runner.RunStage(ctx, currentUsers, stepDuration)
		close(sampleDone)

		if err != nil {
			fmt.Printf("failed (%v)\n", err)
			break
		}

		fmt.Printf("p95: %5.1fms | CPU: %4.1f%% | Errors: %3.1f%%\n", metrics.P95Ms, peakCPU, metrics.ErrorPercent)

		// Check SLA breach
		if peakCPU > m.cfg.Thresholds.MaxCPUPercent {
			bottleneck = fmt.Sprintf("API CPU exceeded threshold (%.1f%% > %.0f%%)", peakCPU, m.cfg.Thresholds.MaxCPUPercent)
			break
		}
		if metrics.P95Ms > m.cfg.Thresholds.MaxP95LatencyMs {
			bottleneck = fmt.Sprintf("p95 latency breached SLA (%.1fms > %.0fms)", metrics.P95Ms, m.cfg.Thresholds.MaxP95LatencyMs)
			break
		}
		if metrics.ErrorPercent > m.cfg.Thresholds.MaxErrorRatePercent {
			bottleneck = fmt.Sprintf("Error rate breached threshold (%.1f%% > %.1f%%)", metrics.ErrorPercent, m.cfg.Thresholds.MaxErrorRatePercent)
			break
		}

		maxObserved = currentUsers
		currentUsers += m.cfg.Workload.Step
	}

	return maxObserved, bottleneck
}

// ParseMemoryMB converts "512MB", "1GB", etc. into megabytes as an integer
func ParseMemoryMB(mem string) int64 {
	mem = strings.ToUpper(strings.TrimSpace(mem))
	if strings.HasSuffix(mem, "GB") {
		val, _ := strconv.ParseInt(strings.TrimSuffix(mem, "GB"), 10, 64)
		return val * 1024
	}
	if strings.HasSuffix(mem, "MB") {
		val, _ := strconv.ParseInt(strings.TrimSuffix(mem, "MB"), 10, 64)
		return val
	}
	val, _ := strconv.ParseInt(mem, 10, 64)
	return val
}

func (m *MatrixEngine) waitForHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.cfg.Application.URL, nil)
		if err == nil {
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode < 500 {
					return nil
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("target did not become healthy within %v", timeout)
}
