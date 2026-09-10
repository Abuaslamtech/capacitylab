package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Abuaslamtech/capacitylab/internal/config"
	"github.com/Abuaslamtech/capacitylab/internal/engine"
	"github.com/Abuaslamtech/capacitylab/internal/load"
	"github.com/Abuaslamtech/capacitylab/internal/monitor"
	"github.com/Abuaslamtech/capacitylab/internal/report"
	"github.com/Abuaslamtech/capacitylab/internal/runtime"
	"github.com/spf13/cobra"
)

var (
	outputReport string
	openBrowser  bool
	runMatrix    bool
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

		ctx := context.Background()

		// Initialize Database Monitors if configured
		var (
			pgMonitor    *monitor.PostgresMonitor
			redisMonitor *monitor.RedisMonitor
			dbTelemetry  report.DatabaseTelemetry
		)

		if cfg.Services.Postgres.DSN != "" {
			pgMon, err := monitor.NewPostgresMonitor(cfg.Services.Postgres.DSN)
			if err == nil && pgMon != nil {
				pgMonitor = pgMon
				defer pgMonitor.Close(ctx)
				dbTelemetry.HasPostgres = true
				fmt.Println("🐘 PostgreSQL Monitor connected")
			}
		}

		if cfg.Services.Redis.Addr != "" {
			rMon, err := monitor.NewRedisMonitor(cfg.Services.Redis.Addr)
			if err == nil && rMon != nil {
				redisMonitor = rMon
				defer redisMonitor.Close()
				dbTelemetry.HasRedis = true
				fmt.Println("🔴 Redis Monitor connected")
			}
		}

		// ─────────────────────────────────────────────────────────────
		// MATRIX MODE: Run multi-tier resource sizing sweep
		// ─────────────────────────────────────────────────────────────
		if runMatrix {
			fmt.Println("🔬 Running Multi-Tier Resource Sizing Matrix...")
			matrixEngine := engine.NewMatrixEngine(cfg, dockerClient)
			results, err := matrixEngine.RunMatrix(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ Matrix evaluation error: %v\n", err)
				os.Exit(1)
			}

			fmt.Println("\n═════════════════════════════════════════════════════════════")
			fmt.Println("📊 HARDWARE SIZING COMPARISON MATRIX")
			fmt.Println("═════════════════════════════════════════════════════════════")
			fmt.Printf("%-20s %-20s %-20s\n", "Configuration", "Sustainable Load", "Recommendation")
			fmt.Println("─────────────────────────────────────────────────────────────")

			var matrixTiers []report.TierData
			for _, r := range results {
				configStr := fmt.Sprintf("%.1f vCPU / %d MB", r.CPUs, r.MemoryMB)
				loadStr := fmt.Sprintf("~%d users", r.SustainableCapacity)
				fmt.Printf("%-20s %-20s %-20s\n", configStr, loadStr, r.Status)

				matrixTiers = append(matrixTiers, report.TierData{
					Configuration:       configStr,
					SustainableCapacity: loadStr,
					Status:              r.Status,
					Bottleneck:          r.Bottleneck,
				})
			}
			fmt.Println("═════════════════════════════════════════════════════════════")

			// Generate HTML Report with Matrix Table
			var maxVUs, sustVUs int
			if len(results) > 0 {
				maxVUs = results[len(results)-1].MaxObservedUsers
				sustVUs = results[len(results)-1].SustainableCapacity
			}

			_ = report.GenerateHTML(
				outputReport,
				cfg.Application.Name,
				cfg.Application.URL,
				cfg.Services.API.Container,
				"Multi-Tier Sizing Completed",
				cfg.Workload.MaxUsers,
				maxVUs,
				sustVUs,
				nil,
				dbTelemetry,
				matrixTiers,
			)

			fmt.Printf("\n📄 Hardware sizing report generated: %s\n", outputReport)
			if openBrowser {
				_ = report.OpenInBrowser(outputReport)
			}
			return
		}

		// ─────────────────────────────────────────────────────────────
		// STANDARD MODE: Progressive Ramping Benchmark
		// ─────────────────────────────────────────────────────────────
		runner := load.NewRunner(cfg)
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
		stageStepDuration := 6 * time.Second

		var (
			maxObservedUsers = 0
			saturationReason = ""
			breached         = false
			recordedStages   []report.StageData
		)

		stageNum := 1
		for currentUsers <= cfg.Workload.MaxUsers {
			fmt.Printf("▶ [Stage %d] Testing %d Concurrent Users (%v)...\n", stageNum, currentUsers, stageStepDuration)

			var peakCPU float64
			var peakMem float64
			sampleDone := make(chan bool)

			go func() {
				ticker := time.NewTicker(400 * time.Millisecond)
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
						// Harvest DB metrics if available
						if pgMonitor != nil {
							pgMetrics, err := pgMonitor.Harvest(ctx)
							if err == nil && pgMetrics != nil {
								dbTelemetry.PostgresActive = pgMetrics.ActiveConnections
								dbTelemetry.PostgresMax = pgMetrics.MaxConnections
								dbTelemetry.PostgresCacheHit = pgMetrics.CacheHitRatio
								dbTelemetry.PostgresLocks = pgMetrics.WaitingLocks
							}
						}
						if redisMonitor != nil {
							rMetrics, err := redisMonitor.Harvest(ctx)
							if err == nil && rMetrics != nil {
								dbTelemetry.RedisMemoryMB = rMetrics.UsedMemoryMB
								dbTelemetry.RedisConnected = rMetrics.ConnectedClients
								dbTelemetry.RedisHitRatio = rMetrics.HitRatio
								dbTelemetry.RedisInstantaneous = rMetrics.InstantaneousOps
							}
						}
					}
				}
			}()

			metrics, err := runner.RunStage(ctx, currentUsers, stageStepDuration)
			close(sampleDone)

			if err != nil {
				fmt.Printf("   ❌ Stage error: %v\n", err)
				break
			}

			recordedStages = append(recordedStages, report.StageData{
				StageNum:     stageNum,
				VUs:          currentUsers,
				RPS:          metrics.RPS,
				P50Ms:        metrics.P50Ms,
				P95Ms:        metrics.P95Ms,
				P99Ms:        metrics.P99Ms,
				CPUPercent:   peakCPU,
				MemoryMB:     peakMem,
				ErrorPercent: metrics.ErrorPercent,
			})

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

		// Generate HTML Report
		err = report.GenerateHTML(
			outputReport,
			cfg.Application.Name,
			cfg.Application.URL,
			containerName,
			saturationReason,
			cfg.Workload.MaxUsers,
			maxObservedUsers,
			sustainableCapacity,
			recordedStages,
			dbTelemetry,
			nil,
		)
		if err != nil {
			fmt.Printf("⚠️  Failed to generate HTML report: %v\n", err)
		} else {
			fmt.Printf("\n📄 Interactive report generated: %s\n", outputReport)
			if openBrowser {
				fmt.Println("🌐 Opening report in your default browser...")
				_ = report.OpenInBrowser(outputReport)
			}
		}
	},
}

func init() {
	runCmd.Flags().StringVarP(&configPath, "config", "c", "capacitylab.yaml", "Path to configuration file")
	runCmd.Flags().StringVarP(&outputReport, "output", "o", "capacity-report.html", "Path to output HTML report")
	runCmd.Flags().BoolVar(&openBrowser, "open", false, "Automatically open report in browser when finished")
	runCmd.Flags().BoolVarP(&runMatrix, "matrix", "m", false, "Run progressive load across configured CPU/RAM matrix tiers")
	rootCmd.AddCommand(runCmd)
}
