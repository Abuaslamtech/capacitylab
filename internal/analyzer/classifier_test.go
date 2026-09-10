package analyzer

import (
	"testing"

	"github.com/Abuaslamtech/capacitylab/internal/config"
	"github.com/Abuaslamtech/capacitylab/internal/load"
	"github.com/Abuaslamtech/capacitylab/internal/monitor"
	"github.com/Abuaslamtech/capacitylab/internal/runtime"
)

func TestClassifier_Diagnose(t *testing.T) {
	thresholds := config.Thresholds{
		MaxCPUPercent:       85.0,
		MaxMemoryPercent:    80.0,
		MaxP95LatencyMs:     250.0,
		MaxErrorRatePercent: 1.0,
	}

	c := NewClassifier(thresholds)

	tests := []struct {
		name                 string
		snapshot             StateSnapshot
		wantBreach           bool
		wantPrimaryComponent string
		wantPrimarySeverity  string
	}{
		{
			name: "All healthy within SLA",
			snapshot: StateSnapshot{
				LoadMetrics: load.StageMetrics{
					RPS:          500,
					P95Ms:        45.0,
					ErrorPercent: 0.0,
				},
				APIMetrics: runtime.ContainerMetrics{
					CPUPercent:    40.0,
					MemoryPercent: 30.0,
					MemoryUsedMB:  150.0,
					MemoryLimitMB: 512.0,
				},
			},
			wantBreach:           false,
			wantPrimaryComponent: "None",
			wantPrimarySeverity:  "HEALTHY",
		},
		{
			name: "PostgreSQL connection exhaustion",
			snapshot: StateSnapshot{
				LoadMetrics: load.StageMetrics{
					RPS:          200,
					P95Ms:        300.0,
					ErrorPercent: 2.0,
				},
				APIMetrics: runtime.ContainerMetrics{
					CPUPercent: 30.0,
				},
				PGMetrics: &monitor.PostgresMetrics{
					ActiveConnections: 95,
					MaxConnections:    100,
					CacheHitRatio:     98.0,
				},
			},
			wantBreach:           true,
			wantPrimaryComponent: "PostgreSQL Connection Pool",
			wantPrimarySeverity:  "CRITICAL",
		},
		{
			name: "PostgreSQL lock contention",
			snapshot: StateSnapshot{
				LoadMetrics: load.StageMetrics{
					RPS:          100,
					P95Ms:        450.0,
					ErrorPercent: 0.5,
				},
				APIMetrics: runtime.ContainerMetrics{
					CPUPercent: 25.0,
				},
				PGMetrics: &monitor.PostgresMetrics{
					ActiveConnections: 10,
					MaxConnections:    100,
					WaitingLocks:      12,
					CacheHitRatio:     95.0,
				},
			},
			wantBreach:           true,
			wantPrimaryComponent: "PostgreSQL Lock Contention",
			wantPrimarySeverity:  "CRITICAL",
		},
		{
			name: "API container CPU saturation",
			snapshot: StateSnapshot{
				LoadMetrics: load.StageMetrics{
					RPS:          1200,
					P95Ms:        120.0,
					ErrorPercent: 0.0,
				},
				APIMetrics: runtime.ContainerMetrics{
					CPUPercent: 91.5,
				},
			},
			wantBreach:           true,
			wantPrimaryComponent: "API Compute",
			wantPrimarySeverity:  "CRITICAL",
		},
		{
			name: "Memory OOM risk",
			snapshot: StateSnapshot{
				LoadMetrics: load.StageMetrics{
					RPS:          400,
					P95Ms:        50.0,
					ErrorPercent: 0.0,
				},
				APIMetrics: runtime.ContainerMetrics{
					CPUPercent:    45.0,
					MemoryPercent: 88.0,
					MemoryUsedMB:  450.0,
					MemoryLimitMB: 512.0,
				},
			},
			wantBreach:           true,
			wantPrimaryComponent: "Application Memory",
			wantPrimarySeverity:  "CRITICAL",
		},
		{
			name: "Redis evictions breach",
			snapshot: StateSnapshot{
				LoadMetrics: load.StageMetrics{
					RPS:          800,
					P95Ms:        60.0,
					ErrorPercent: 0.0,
				},
				APIMetrics: runtime.ContainerMetrics{
					CPUPercent: 40.0,
				},
				RedisMetrics: &monitor.RedisMetrics{
					UsedMemoryMB: 250.0,
					EvictedKeys:  142,
				},
			},
			wantBreach:           true,
			wantPrimaryComponent: "Redis Cache Evictions",
			wantPrimarySeverity:  "CRITICAL",
		},
		{
			name: "Downstream I/O latency breach without host saturation",
			snapshot: StateSnapshot{
				LoadMetrics: load.StageMetrics{
					RPS:          600,
					P95Ms:        310.0, // > 250ms
					ErrorPercent: 0.0,
				},
				APIMetrics: runtime.ContainerMetrics{
					CPUPercent:    35.0,
					MemoryPercent: 20.0,
				},
			},
			wantBreach:           true,
			wantPrimaryComponent: "Downstream I/O or Event Loop Delay",
			wantPrimarySeverity:  "CRITICAL",
		},
		{
			name: "Application 5xx error rate breach",
			snapshot: StateSnapshot{
				LoadMetrics: load.StageMetrics{
					RPS:          500,
					P95Ms:        40.0,
					ErrorPercent: 3.5, // > 1.0%
				},
				APIMetrics: runtime.ContainerMetrics{
					CPUPercent:    25.0,
					MemoryPercent: 20.0,
				},
			},
			wantBreach:           true,
			wantPrimaryComponent: "Application Server Error Rate",
			wantPrimarySeverity:  "CRITICAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := c.Diagnose(tt.snapshot)

			if report.HasBreach != tt.wantBreach {
				t.Errorf("Diagnose() HasBreach = %v, want %v", report.HasBreach, tt.wantBreach)
			}
			if report.Primary.Component != tt.wantPrimaryComponent {
				t.Errorf("Diagnose() Primary.Component = %q, want %q", report.Primary.Component, tt.wantPrimaryComponent)
			}
			if report.Primary.Severity != tt.wantPrimarySeverity {
				t.Errorf("Diagnose() Primary.Severity = %q, want %q", report.Primary.Severity, tt.wantPrimarySeverity)
			}
			if report.Primary.Remediation == "" {
				t.Errorf("Diagnose() Primary.Remediation should not be empty")
			}
		})
	}
}
