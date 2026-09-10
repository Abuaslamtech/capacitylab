package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Abuaslamtech/capacitylab/internal/config"
	"github.com/Abuaslamtech/capacitylab/internal/load"
	"github.com/Abuaslamtech/capacitylab/internal/runtime"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the capacity benchmark against your backend",
	Long: `Executes progressive step-ramping load against the target application,
    simultaneously scraping container CPU and memory metrics to detect the saturation point.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("🚀 Starting CapacityLab Benchmark Engine")
		fmt.Println("─────────────────────────────────────────────────────────────")

		cfg, err := config.Load(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Configuration error: %v\n", err)
			os.Exit(1)
		}

		dockerClient, err := runtime.NewClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Docker error: %v\n", err)
			os.Exit(1)
		}

		runner := load.NewRunner(cfg)
		ctx := context.Background()

		containerName := cfg.Services.API.Container
		fmt.Printf("🎯 Target:      %s (%s)\n", cfg.Application.Name, cfg.Application.URL)
		fmt.Printf("🐳 Container:   %s\n", containerName)
		fmt.Printf("📈 Load Range:  %d ➔ %d users (step: %d)\n\n",
			cfg.Workload.StartUsers, cfg.Workload.MaxUsers, cfg.Workload.Step)

		// 1. Warm-up Phase
		warmupDuration := 5 * time.Second
		fmt.Printf("⏳ [Stage 0] Warming up caches (%v with %d VUs)...\n", warmupDuration, cfg.Workload.StartUsers)
		_, err = runner.RunStage(ctx, cfg.Workload.StartUsers, warmupDuration)
		if err != nil {
			fmt.Printf("⚠️  Warmup warning: %v\n", err)
		}
		fmt.Println("   ✓ Warmup complete! Caches and sockets primed.\n")

		// 2. Progressive Ramping Stages
		currentUsers := cfg.Workload.StartUsers
		stageStepDuration := 6 * time.Second // Fast 6-second stages for testing

		var (
			maxObservedUsers = 0
			saturationReason = ""
			breached         = false
		)

		stageNum := 1
		for currentUsers <= cfg.Workload.MaxUsers {
			fmt.Printf("▶ [Stage %d] Testing %d Concurrent Users (%v)...\n", stageNum, currentUsers, stageStepDuration)

			// Start background container metrics sampling
			var peakCPU float64
			var peakMem float64
			sampleDone := make(chan bool)

			go func() {
				ticker := time.NewTicker(500 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-sampleDone:
						return
					case <-ticker.C:
						metrics, err := dockerClient.GetMetrics(ctx, containerName)
						if err == nil {
							if metrics.CPUPercent > peakCPU {
								peakCPU = metrics.CPUPercent
							}
							if metrics.MemoryUsedMB > peakMem {
								peakMem = metrics.MemoryUsedMB
							}
						}
					}
				}
			}()

			// Run traffic for this stage
			metrics, err := runner.RunStage(ctx, currentUsers, stageStepDuration)
			close(sampleDone) // Stop sampling

			if err != nil {
				fmt.Printf("   ❌ Stage error: %v\n", err)
				break
			}

			// Print stage telemetry line
			fmt.Printf("   📊 RPS: %-6.0f | p95: %-5.1fms | API CPU: %-4.1f%% | RAM: %-5.1fMB | Errors: %.1f%%\n",
				metrics.RPS, metrics.P95Ms, peakCPU, peakMem, metrics.ErrorPercent)

			// 3. Evaluate SLA thresholds
			if peakCPU > cfg.Thresholds.MaxCPUPercent {
				saturationReason = fmt.Sprintf("API CPU exceeded threshold (%.1f%% > %.0f%%)", peakCPU, cfg.Thresholds.MaxCPUPercent)
				breached = true
			} else if metrics.P95Ms > cfg.Thresholds.MaxP95LatencyMs {
				saturationReason = fmt.Sprintf("p95 latency breached SLA (%.1fms > %.0fms)", metrics.P95Ms, cfg.Thresholds.MaxP95LatencyMs)
				breached = true
			} else if metrics.ErrorPercent > cfg.Thresholds.MaxErrorRatePercent {
				saturationReason = fmt.Sprintf("Error rate exceeded threshold (%.1f%% > %.1f%%)", metrics.ErrorPercent, cfg.Thresholds.MaxErrorRatePercent)
				breached = true
			}

			if breached {
				fmt.Printf("\n🛑 SATURATION BREACH DETECTED at %d users!\n", currentUsers)
				fmt.Printf("   Cause: %s\n\n", saturationReason)
				break
			}

			maxObservedUsers = currentUsers
			currentUsers += cfg.Workload.Step
			stageNum++
		}

		// 4. Print Capacity Plan Summary
		fmt.Println("═════════════════════════════════════════════════════════════")
		fmt.Println("🏆 CAPACITY PLANNING REPORT")
		fmt.Println("═════════════════════════════════════════════════════════════")

		if maxObservedUsers == 0 {
			fmt.Println("❌ Application could not sustain baseline traffic.")
			return
		}

		// Apply 30% safety headroom
		sustainableCapacity := int(float64(maxObservedUsers) * 0.70)

		fmt.Printf("Target Workload:                 %d concurrent users\n", cfg.Workload.MaxUsers)
		fmt.Printf("Maximum Observed Load:           %d concurrent users\n", maxObservedUsers)
		fmt.Printf("Recommended Sustainable Load:    ~%d concurrent users (30%% safety buffer)\n\n", sustainableCapacity)

		if breached {
			fmt.Printf("Primary Bottleneck:              %s\n", saturationReason)
		} else {
			fmt.Println("Primary Bottleneck:              None detected within tested range!")
		}

		fmt.Println("\nRecommended Sizing for Current Load:")
		fmt.Printf("  • %s Container: 1.0 vCPU, 512 MB RAM\n", containerName)
		fmt.Println("═════════════════════════════════════════════════════════════")
	},
}

func init() {
	runCmd.Flags().StringVarP(&configPath, "config", "c", "capacitylab.yaml", "Path to configuration file")
	rootCmd.AddCommand(runCmd)
}
