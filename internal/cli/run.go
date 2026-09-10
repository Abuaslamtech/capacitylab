package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Abuaslamtech/capacitylab/internal/analyzer"
	"github.com/Abuaslamtech/capacitylab/internal/config"
	"github.com/Abuaslamtech/capacitylab/internal/dashboard"
	"github.com/Abuaslamtech/capacitylab/internal/engine"
	"github.com/Abuaslamtech/capacitylab/internal/history"
	"github.com/Abuaslamtech/capacitylab/internal/load"
	"github.com/Abuaslamtech/capacitylab/internal/monitor"
	"github.com/Abuaslamtech/capacitylab/internal/report"
	"github.com/Abuaslamtech/capacitylab/internal/runtime"
	"github.com/Abuaslamtech/capacitylab/internal/sentinel"
	"github.com/spf13/cobra"
)

var (
	outputReport       string
	openBrowser        bool
	runMatrix          bool
	overrideURL        string
	overrideMaxUsers   int
	overrideStartUsers int
	overrideStep       int
	overrideContainer  string
	failOnRegression   bool
	overrideEngine     string
	overrideCPU        string
	overrideMemory     string
	overrideCPUSet     string
	cleanupCompose     bool
	jsonStdout         bool
	outputJSONFile     string
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the capacity benchmark against your backend",
	Long: `Executes progressive step-ramping load against the target application,
simultaneously scraping container CPU and memory metrics to detect the saturation point.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprint(os.Stdout, BrandBanner("Autonomous Benchmark Engine"))

		cfg, err := config.Load(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s %s\n     %v\n\n", IconError(), Bold(Red("Configuration Error")), err)
			os.Exit(1)
		}

		if overrideURL != "" {
			cfg.Application.URL = overrideURL
		}
		if overrideMaxUsers > 0 {
			cfg.Workload.MaxUsers = overrideMaxUsers
		}
		if overrideStartUsers > 0 {
			cfg.Workload.StartUsers = overrideStartUsers
		}
		if overrideStep > 0 {
			cfg.Workload.Step = overrideStep
		}
		if overrideContainer != "" {
			cfg.Services.API.Container = overrideContainer
		}
		if err := cfg.Validate(); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Configuration error after flag overrides: %v\n", err)
			os.Exit(1)
		}

		dockerClient, err := runtime.NewClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Docker error: %v\n", err)
			os.Exit(1)
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		// 1. Detect Docker / Host Virtualization Topology (Trap 2)
		if envInfo, err := dockerClient.DetectEnvironment(ctx); err == nil {
			hostType := "Native Linux"
			if envInfo.IsDockerDesktop {
				hostType = "Docker Desktop VM"
			} else if envInfo.IsWSL2 {
				hostType = "WSL2 Virtualized"
			}
			fmt.Printf("🐳 Host Engine: %s (Daemon RAM: %.1f GB, Cores: %d)\n", hostType, envInfo.TotalMemMB/1024, envInfo.CPUs)
			if envInfo.Warning != "" {
				fmt.Printf("   ⚠️  TOPOLOGY: %s\n", envInfo.Warning)
			}
		}

		// 2. Automated Compose Orchestration if configured
		var composeMgr *runtime.ComposeManager
		if cfg.Application.Startup.ComposeFile != "" {
			fmt.Printf("📦 Orchestrating application startup via '%s'...\n", cfg.Application.Startup.ComposeFile)
			composeMgr = runtime.NewComposeManager(
				cfg.Application.Startup.ComposeFile,
				cfg.Application.URL,
				cfg.Application.Startup.Timeout,
			)
			if err := composeMgr.Start(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "❌ Startup failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("   ✓ Application containers initialized and healthy!")
			if cleanupCompose {
				defer func() {
					fmt.Println("\n🧹 Cleaning up Docker Compose services...")
					cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cleanCancel()
					if err := composeMgr.Stop(cleanCtx); err != nil {
						fmt.Printf("⚠️  Compose cleanup warning: %v\n", err)
					} else {
						fmt.Println("   ✓ Compose services stopped.")
					}
				}()
			}
		}

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
				defer redisMonitor.Close(ctx)
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

			matrixSizing := report.SizingData{
				TierName: "Multi-Tier Hardware Sweep",
				APIVCPU:  "Evaluated via matrix",
				APIRAM:   "Evaluated via matrix",
			}

			_ = report.GenerateHTML(
				outputReport,
				cfg.Application.Name,
				cfg.Application.URL,
				cfg.Services.API.Container,
				"Multi-Tier Sizing Completed",
				"Review the matrix table below to select optimal tier.",
				nil,
				nil,
				cfg.Workload.MaxUsers,
				maxVUs,
				sustVUs,
				nil,
				dbTelemetry,
				matrixTiers,
				matrixSizing,
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
		containerName := cfg.Services.API.Container

		// Apply Container Constraints (CPU, Memory, CPUSet Pinning)
		targetCPU := cfg.Services.API.CPU
		if overrideCPU != "" {
			targetCPU = overrideCPU
		}
		targetMem := cfg.Services.API.Memory
		if overrideMemory != "" {
			targetMem = overrideMemory
		}
		targetCPUSet := cfg.Services.API.CPUSet
		if overrideCPUSet != "" {
			targetCPUSet = overrideCPUSet
		}

		if (targetCPU != "" || targetMem != "" || targetCPUSet != "") && containerName != "" {
			var cpuVal float64
			var memMB int64
			if targetCPU != "" {
				if parsedCPU, err := strconv.ParseFloat(targetCPU, 64); err == nil {
					cpuVal = parsedCPU
				}
			}
			if targetMem != "" {
				memMB = engine.ParseMemoryMB(targetMem)
			}
			fmt.Printf("⚙️  Applying Container Constraints: CPU=%.1f, RAM=%d MB, CPUSet=%s\n", cpuVal, memMB, targetCPUSet)
			if err := dockerClient.UpdateResources(ctx, containerName, cpuVal, memMB, targetCPUSet); err != nil {
				fmt.Printf("⚠️  Warning: Failed to update container cgroups: %v\n", err)
			} else {
				fmt.Printf("   ✓ Applied resource constraints to '%s'\n", containerName)
			}
		}

		// Select Traffic Generator Engine
		engineChoice := "native"
		if cfg.Workload.Engine != "" {
			engineChoice = strings.ToLower(cfg.Workload.Engine)
		}
		if overrideEngine != "" {
			engineChoice = strings.ToLower(overrideEngine)
		}

		var runner load.StageRunner
		if engineChoice == "k6" {
			k6Adapter := load.NewK6Adapter(cfg)
			if !k6Adapter.IsAvailable() {
				fmt.Fprintf(os.Stderr, "❌ k6 engine selected, but 'k6' binary was not found in PATH.\nInstall k6 (https://k6.io/docs/get-started/installation/) or run with '--engine native'.\n")
				os.Exit(1)
			}
			runner = k6Adapter
			fmt.Println("⚡ Traffic Engine: k6 Headless Subprocess")
		} else {
			runner = load.NewRunner(cfg)
			fmt.Println("⚡ Traffic Engine: Native Go Connection-Pooled Pool")
		}

		fmt.Printf("🎯 Target:      %s (%s)\n", cfg.Application.Name, cfg.Application.URL)
		fmt.Printf("🐳 Container:   %s\n", containerName)
		fmt.Printf("📈 Load Range:  %d ➔ %d users (step: %d)\n\n",
			cfg.Workload.StartUsers, cfg.Workload.MaxUsers, cfg.Workload.Step)

		// 1. Warm-up Phase with Cache Hit Ratio Stabilization (Trap 3)
		warmupDuration := cfg.Workload.WarmupDuration
		if warmupDuration <= 0 {
			warmupDuration = 5 * time.Second
		}
		fmt.Printf("  %s %s %s\n", BadgeCyan("WARMUP"), Bold("Warming up target caches & sockets"), Dim(fmt.Sprintf("(%v with %d VUs)", warmupDuration, cfg.Workload.StartUsers)))
		_, err = runner.RunStage(ctx, cfg.Workload.StartUsers, warmupDuration)
		if err != nil {
			fmt.Printf("  %s %s: %v\n", IconWarning(), Dim("Warmup warning"), err)
		}
		if pgMonitor != nil {
			if pgMetrics, _ := pgMonitor.Harvest(ctx); pgMetrics != nil && pgMetrics.CacheHitRatio > 0 {
				fmt.Printf("    %s PostgreSQL cache warmed (Hit Ratio: %.1f%%)\n", IconSuccess(), pgMetrics.CacheHitRatio)
			}
		}
		if redisMonitor != nil {
			if rMetrics, _ := redisMonitor.Harvest(ctx); rMetrics != nil {
				fmt.Printf("    %s Redis cache primed (Memory: %.1f MB)\n", IconSuccess(), rMetrics.UsedMemoryMB)
			}
		}
		fmt.Printf("  %s %s\n\n", IconSuccess(), Green("Warmup complete. Initializing workload step ramping."))

		// 2. Progressive Ramping Stages
		classifier := analyzer.NewClassifier(cfg.Thresholds)
		sent := sentinel.New()
		var finalReport analyzer.BottleneckReport

		stageStepDuration := 6 * time.Second

		type stagePlan struct {
			num      int
			vus      int
			duration time.Duration
			label    string
		}

		var plan []stagePlan
		switch cfg.Workload.Type {
		case "soak", "constant":
			totalDur := cfg.Workload.Duration
			if totalDur <= 0 {
				totalDur = 60 * time.Second
			}
			sliceDur := 10 * time.Second
			if sliceDur > totalDur {
				sliceDur = totalDur
			}
			slices := int(totalDur / sliceDur)
			if slices < 1 {
				slices = 1
			}
			for i := 1; i <= slices; i++ {
				plan = append(plan, stagePlan{
					num:      i,
					vus:      cfg.Workload.StartUsers,
					duration: sliceDur,
					label:    fmt.Sprintf("Soak Interval %d/%d (%d VUs)", i, slices, cfg.Workload.StartUsers),
				})
			}

		case "spike":
			plan = append(plan, stagePlan{
				num:      1,
				vus:      cfg.Workload.StartUsers,
				duration: stageStepDuration,
				label:    fmt.Sprintf("Baseline Traffic (%d VUs)", cfg.Workload.StartUsers),
			})
			plan = append(plan, stagePlan{
				num:      2,
				vus:      cfg.Workload.MaxUsers,
				duration: stageStepDuration,
				label:    fmt.Sprintf("Surge Spike (%d VUs)", cfg.Workload.MaxUsers),
			})
			plan = append(plan, stagePlan{
				num:      3,
				vus:      cfg.Workload.StartUsers,
				duration: stageStepDuration,
				label:    fmt.Sprintf("Recovery Phase (%d VUs)", cfg.Workload.StartUsers),
			})

		default: // "step-ramp"
			curr := cfg.Workload.StartUsers
			stg := 1
			for curr <= cfg.Workload.MaxUsers {
				plan = append(plan, stagePlan{
					num:      stg,
					vus:      curr,
					duration: stageStepDuration,
					label:    fmt.Sprintf("%d Virtual Users", curr),
				})
				curr += cfg.Workload.Step
				stg++
			}
		}

		var (
			maxObservedUsers       = 0
			breached               = false
			recordedStages         []report.StageData
			vusHistory             []int
			p95History             []float64
			allSentinelWarnings    []string
			peakRunnerCPU          float64
			peakRunnerMem          float64
			peakTimeWaitSockets    int
			latestContainerMetrics runtime.ContainerMetrics
			latestPGMetrics        *monitor.PostgresMetrics
			latestRedisMetrics     *monitor.RedisMetrics
		)

		for _, sp := range plan {
			if ctx.Err() != nil {
				fmt.Println("\n  " + IconWarning() + " " + Yellow("Benchmark interrupted. Summarizing completed stages..."))
				break
			}
			fmt.Printf("  %s %s %s\n", BadgeCyan(fmt.Sprintf("STAGE %d", sp.num)), Bold(sp.label), Dim(fmt.Sprintf("(duration: %v)", sp.duration)))

			var peakCPU float64
			var peakMem float64

			baselineMetrics, _ := dockerClient.GetMetrics(ctx, containerName)
			var baselineThrottled uint64
			if baselineMetrics != nil {
				baselineThrottled = baselineMetrics.ThrottledPeriods
			}

			guard := sent.StartStage()
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
						if err == nil && metrics != nil {
							if metrics.CPUPercent > peakCPU {
								peakCPU = metrics.CPUPercent
							}
							if metrics.MemoryUsedMB > peakMem {
								peakMem = metrics.MemoryUsedMB
							}
							latestContainerMetrics = *metrics
						}
						// Harvest DB metrics if available
						if pgMonitor != nil {
							pgMetrics, err := pgMonitor.Harvest(ctx)
							if err == nil && pgMetrics != nil {
								dbTelemetry.PostgresActive = pgMetrics.ActiveConnections
								dbTelemetry.PostgresMax = pgMetrics.MaxConnections
								dbTelemetry.PostgresCacheHit = pgMetrics.CacheHitRatio
								dbTelemetry.PostgresLocks = pgMetrics.WaitingLocks
								latestPGMetrics = pgMetrics
							}
						}
						if redisMonitor != nil {
							rMetrics, err := redisMonitor.Harvest(ctx)
							if err == nil && rMetrics != nil {
								dbTelemetry.RedisMemoryMB = rMetrics.UsedMemoryMB
								dbTelemetry.RedisConnected = rMetrics.ConnectedClients
								dbTelemetry.RedisHitRatio = rMetrics.HitRatio
								dbTelemetry.RedisInstantaneous = rMetrics.InstantaneousOps
								latestRedisMetrics = rMetrics
							}
						}
					}
				}
			}()

			metrics, err := runner.RunStage(ctx, sp.vus, sp.duration)
			close(sampleDone)
			sentReport := guard.Finish()

			if sentReport.Telemetry.RunnerCPUPercent > peakRunnerCPU {
				peakRunnerCPU = sentReport.Telemetry.RunnerCPUPercent
			}
			if sentReport.Telemetry.RunnerMemMB > peakRunnerMem {
				peakRunnerMem = sentReport.Telemetry.RunnerMemMB
			}
			if sentReport.Telemetry.TimeWaitSockets > peakTimeWaitSockets {
				peakTimeWaitSockets = sentReport.Telemetry.TimeWaitSockets
			}
			if sentReport.HasWarning {
				allSentinelWarnings = append(allSentinelWarnings, sentReport.Warnings...)
				for _, w := range sentReport.Warnings {
					fmt.Printf("    %s %s: %s\n", IconWarning(), Dim("Host Sentinel"), Yellow(w))
				}
			}

			if err != nil {
				fmt.Printf("    %s Stage error: %v\n", IconError(), err)
				break
			}

			if latestContainerMetrics.ThrottledPeriods >= baselineThrottled {
				latestContainerMetrics.ThrottledPeriods -= baselineThrottled
			} else {
				latestContainerMetrics.ThrottledPeriods = 0
			}

			latestContainerMetrics.CPUPercent = peakCPU
			latestContainerMetrics.MemoryUsedMB = peakMem
			if latestContainerMetrics.MemoryLimitMB > 0 {
				latestContainerMetrics.MemoryPercent = (peakMem / latestContainerMetrics.MemoryLimitMB) * 100
			}

			recordedStages = append(recordedStages, report.StageData{
				StageNum:     sp.num,
				VUs:          sp.vus,
				RPS:          metrics.RPS,
				P50Ms:        metrics.P50Ms,
				P95Ms:        metrics.P95Ms,
				P99Ms:        metrics.P99Ms,
				CPUPercent:   peakCPU,
				MemoryMB:     peakMem,
				ErrorPercent: metrics.ErrorPercent,
			})

			dashboard.PublishLiveEvent("stage_complete", map[string]any{
				"stage":     sp.num,
				"vus":       sp.vus,
				"rps":       metrics.RPS,
				"p95":       metrics.P95Ms,
				"cpu":       peakCPU,
				"memory_mb": peakMem,
				"error_pct": metrics.ErrorPercent,
				"app_name":  cfg.Application.Name,
			})

			p95Formatted := fmt.Sprintf("%.1fms", metrics.P95Ms)
			if metrics.P95Ms > cfg.Thresholds.MaxP95LatencyMs {
				p95Formatted = Bold(Red(p95Formatted))
			} else if metrics.P95Ms > cfg.Thresholds.MaxP95LatencyMs*0.75 {
				p95Formatted = Bold(Yellow(p95Formatted))
			} else {
				p95Formatted = Bold(Green(p95Formatted))
			}

			errFormatted := fmt.Sprintf("%.1f%%", metrics.ErrorPercent)
			if metrics.ErrorPercent > cfg.Thresholds.MaxErrorRatePercent {
				errFormatted = Bold(Red(errFormatted))
			} else if metrics.ErrorPercent > 0 {
				errFormatted = Bold(Yellow(errFormatted))
			} else {
				errFormatted = Green(errFormatted)
			}

			fmt.Printf("    %s %s %-6s  %s %-16s  %s %-14s",
				IconPointer(), Dim("RPS:"), Bold(Cyan(fmt.Sprintf("%.0f", metrics.RPS))),
				Dim("p95:"), p95Formatted,
				Dim("Errors:"), errFormatted)
			if peakCPU > 0 || peakMem > 0 {
				fmt.Printf("  %s %-6s  %s %-7s",
					Dim("CPU:"), Dim(fmt.Sprintf("%.1f%%", peakCPU)),
					Dim("RAM:"), Dim(fmt.Sprintf("%.1fMB", peakMem)))
			}
			fmt.Println()

			vusHistory = append(vusHistory, sp.vus)
			p95History = append(p95History, metrics.P95Ms)
			knee := sent.DetectKneePoint(vusHistory, p95History)
			if knee.Detected && knee.InflectionVUs == sp.vus {
				fmt.Printf("    %s %s: Early latency queueing detected (%.1fx surge at %d VUs)\n",
					IconWarning(), Bold(Yellow("KNEE DETECTED")), knee.CurrentSlope/knee.BaselineSlope, knee.InflectionVUs)
			}

			// 3. Evaluate Cross-Service Root-Cause Classifier
			snap := analyzer.StateSnapshot{
				LoadMetrics:  *metrics,
				APIMetrics:   latestContainerMetrics,
				PGMetrics:    latestPGMetrics,
				RedisMetrics: latestRedisMetrics,
			}

			diag := classifier.Diagnose(snap)
			finalReport = diag

			if diag.HasBreach {
				breached = true
				fmt.Printf("\n  %s %s\n", BadgeRed("SATURATION LIMIT DETECTED"), Bold(fmt.Sprintf("Traffic saturated at %d concurrent users", sp.vus)))
				fmt.Printf("    %s %-16s [%s] %s\n", IconPointer(), Dim("Bottleneck:"), Bold(Red(diag.Primary.Severity)), Bold(diag.Primary.Component))
				fmt.Printf("    %s %-16s %s\n", IconPointer(), Dim("Diagnosis:"), diag.Primary.Summary)
				fmt.Printf("    %s %-16s %s\n", IconPointer(), Dim("Actionable Fix:"), BrightCyan(diag.Primary.Remediation))
				if len(diag.Secondary) > 0 {
					fmt.Println("\n    " + Dim("Secondary Warnings:"))
					for _, sec := range diag.Secondary {
						fmt.Printf("      %s [%s] %s: %s\n", IconWarning(), sec.Severity, sec.Component, sec.Summary)
					}
				}
				if len(diag.NonLimiting) > 0 {
					fmt.Println("\n    " + Dim("Healthy Systems:"))
					for _, nl := range diag.NonLimiting {
						fmt.Printf("      %s %s\n", IconSuccess(), nl)
					}
				}
				fmt.Println()
				break
			}

			maxObservedUsers = sp.vus
		}

		// 4. Print Capacity Plan Summary
		fmt.Fprint(os.Stdout, BrandBanner("Capacity Planning Summary"))

		if maxObservedUsers == 0 {
			fmt.Printf("  %s %s\n\n", IconError(), Bold(Red("Target could not sustain baseline traffic.")))
			return
		}

		sustainableCapacity := int(float64(maxObservedUsers) * 0.70)

		fmt.Printf("  %-26s %s\n", Dim("Target Workload:"), Bold(fmt.Sprintf("%d concurrent users", cfg.Workload.MaxUsers)))
		fmt.Printf("  %-26s %s\n", Dim("Max Observed Capacity:"), Bold(fmt.Sprintf("%d concurrent users", maxObservedUsers)))
		fmt.Printf("  %-26s %s\n\n", Dim("Safe Production Load:"), Bold(BrightGreen(fmt.Sprintf("~%d concurrent users (30%% safety buffer)", sustainableCapacity))))

		if breached {
			fmt.Printf("  %-26s [%s] %s\n", Dim("Primary Bottleneck:"), Bold(Red(finalReport.Primary.Severity)), Bold(finalReport.Primary.Component))
			fmt.Printf("  %-26s %s\n", Dim("Diagnosis:"), finalReport.Primary.Summary)
			fmt.Printf("  %-26s %s\n", Dim("Actionable Fix:"), BrightCyan(finalReport.Primary.Remediation))
			if len(finalReport.Secondary) > 0 {
				fmt.Println("\n  " + Dim("Secondary Warnings:"))
				for _, sec := range finalReport.Secondary {
					fmt.Printf("    %s [%s] %s: %s\n", IconWarning(), sec.Severity, sec.Component, sec.Summary)
				}
			}
			if len(finalReport.NonLimiting) > 0 {
				fmt.Println("\n  " + Dim("Healthy & Non-Limiting Systems:"))
				for _, nl := range finalReport.NonLimiting {
					fmt.Printf("    %s %s\n", IconSuccess(), nl)
				}
			}
		} else {
			fmt.Printf("  %-26s %s\n", Dim("Primary Bottleneck:"), Green("None detected within tested range!"))
			if len(finalReport.NonLimiting) > 0 {
				fmt.Println("\n  " + Dim("Healthy & Non-Limiting Systems:"))
				for _, nl := range finalReport.NonLimiting {
					fmt.Printf("    %s %s\n", IconSuccess(), nl)
				}
			}
		}

		fmt.Println("\n  " + Dim("Local Host Sentinel Integrity:"))
		if len(allSentinelWarnings) == 0 {
			fmt.Printf("    %s Zero host starvation (Peak Runner CPU: %.1f%%, RAM: %.1f MB, TIME_WAIT Sockets: %d)\n",
				IconSuccess(), peakRunnerCPU, peakRunnerMem, peakTimeWaitSockets)
		} else {
			for _, w := range allSentinelWarnings {
				fmt.Printf("    %s %s\n", IconWarning(), w)
			}
		}

		// Compute Dynamic Production Hardware Recommendations (Section 8)
		hwRec := analyzer.ComputeRecommendation(
			cfg.Workload.MaxUsers,
			maxObservedUsers,
			latestContainerMetrics.CPUPercent,
			latestContainerMetrics.MemoryUsedMB,
			dbTelemetry.PostgresActive,
			dbTelemetry.PostgresMax,
			dbTelemetry.RedisMemoryMB,
		)

		fmt.Printf("\n  %s %s\n", Bold("Recommended Production Sizing"), Dim(fmt.Sprintf("(%s - with 30%% safety buffer)", hwRec.APITierName)))
		if hwRec.IsRemoteTarget {
			fmt.Printf("    %s %-16s %s, %s %s\n", IconBullet(), Bold("Target Host:"), hwRec.APIVCPU, hwRec.APIRAM, Dim("(Estimated from traffic ceiling)"))
		} else {
			fmt.Printf("    %s %-16s %s, %s %s\n", IconBullet(), Bold("API Container:"), hwRec.APIVCPU, hwRec.APIRAM, Dim(fmt.Sprintf("(Peak CPU: %.1f%%, RAM: %.1f MB)", latestContainerMetrics.CPUPercent, latestContainerMetrics.MemoryUsedMB)))
		}
		if dbTelemetry.HasPostgres {
			fmt.Printf("    %s %-16s %s, %s %s\n", IconBullet(), Bold("PostgreSQL:"), hwRec.PostgresVCPU, hwRec.PostgresRAM, Dim("("+hwRec.PostgresAdvice+")"))
		}
		if dbTelemetry.HasRedis {
			fmt.Printf("    %s %-16s %s, %s\n", IconBullet(), Bold("Redis:"), hwRec.RedisVCPU, hwRec.RedisRAM)
		}

		fmt.Printf("\n  %s\n", Bold("Multi-Cloud Cost Estimates:"))
		fmt.Printf("    %s %-16s %s\n", IconArrow(), Dim("Budget VPS:"), hwRec.Costs.BudgetVPS)
		fmt.Printf("    %s %-16s %s\n", IconArrow(), Dim("Managed PaaS:"), BrightCyan(hwRec.Costs.PaaS))
		fmt.Printf("    %s %-16s %s\n", IconArrow(), Dim("Hyperscaler:"), Yellow(hwRec.Costs.Hyperscaler))

		if hwRec.Overprovisioned {
			fmt.Printf("\n  %s %s\n", IconWarning(), Yellow(hwRec.OverprovisionWarning))
		}
		fmt.Println("  " + Gray("─────────────────────────────────────────────────────────────"))

		// Prepare Report Data
		var secondaryList []string
		for _, sec := range finalReport.Secondary {
			secondaryList = append(secondaryList, fmt.Sprintf("%s: %s (Fix: %s)", sec.Component, sec.Summary, sec.Remediation))
		}
		for _, w := range allSentinelWarnings {
			secondaryList = append(secondaryList, "Sentinel: "+w)
		}

		bottleneckSummary := finalReport.Primary.Summary
		bottleneckFix := finalReport.Primary.Remediation
		if !breached {
			bottleneckSummary = "None detected within tested range"
			bottleneckFix = "System is operating comfortably within current hardware parameters."
		}

		nonLimitingList := append([]string{}, finalReport.NonLimiting...)
		if len(allSentinelWarnings) == 0 && peakTimeWaitSockets >= 0 {
			nonLimitingList = append(nonLimitingList, fmt.Sprintf("Sentinel Verified (Runner CPU: %.1f%%, TIME_WAIT: %d)", peakRunnerCPU, peakTimeWaitSockets))
		}

		sizingData := report.SizingData{
			TierName:             hwRec.APITierName,
			APIVCPU:              hwRec.APIVCPU,
			APIRAM:               hwRec.APIRAM,
			APICostEst:           hwRec.APICostEst,
			PostgresVCPU:         hwRec.PostgresVCPU,
			PostgresRAM:          hwRec.PostgresRAM,
			PostgresAdvice:       hwRec.PostgresAdvice,
			PostgresCostEst:      hwRec.PostgresCostEst,
			RedisVCPU:            hwRec.RedisVCPU,
			RedisRAM:             hwRec.RedisRAM,
			RedisCostEst:         hwRec.RedisCostEst,
			TotalCostEst:         hwRec.TotalCostEst,
			OverprovisionWarning: hwRec.OverprovisionWarning,
			IsRemoteTarget:       hwRec.IsRemoteTarget,
			SizingRationale:      hwRec.SizingRationale,
			CostBudgetVPS:        hwRec.Costs.BudgetVPS,
			CostPaaS:             hwRec.Costs.PaaS,
			CostHyperscaler:      hwRec.Costs.Hyperscaler,
		}

		// Generate HTML Report
		err = report.GenerateHTML(
			outputReport,
			cfg.Application.Name,
			cfg.Application.URL,
			containerName,
			bottleneckSummary,
			bottleneckFix,
			secondaryList,
			nonLimitingList,
			cfg.Workload.MaxUsers,
			maxObservedUsers,
			sustainableCapacity,
			recordedStages,
			dbTelemetry,
			nil,
			sizingData,
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

		// 5. Persist run snapshot to ~/.capacitylab/history/
		var peakRPS float64
		var p95AtPeak float64
		for _, s := range recordedStages {
			if s.RPS > peakRPS {
				peakRPS = s.RPS
				p95AtPeak = s.P95Ms
			}
		}

		latestPrev, _, _ := history.GetLatestTwoRuns(cfg.Application.Name)

		rec := history.RunRecord{
			Timestamp:         time.Now(),
			AppName:           cfg.Application.Name,
			TargetURL:         cfg.Application.URL,
			TargetVUs:         cfg.Workload.MaxUsers,
			MaxObservedVUs:    maxObservedUsers,
			SustainableVUs:    sustainableCapacity,
			PeakRPS:           peakRPS,
			P95LatencyMs:      p95AtPeak,
			PrimaryBottleneck: finalReport.Primary.Component,
			BottleneckFix:     finalReport.Primary.Remediation,
			Stages:            recordedStages,
		}

		savedFile, err := history.SaveRun(rec)
		var isReg bool
		var regItem string
		var capDelta float64

		if err == nil {
			fmt.Printf("💾 Run snapshot saved: %s\n", savedFile)
			if latestPrev != nil && latestPrev.SustainableVUs > 0 {
				cmp := history.Compare(*latestPrev, rec)
				isReg = cmp.IsRegression
				regItem = cmp.RegressionItem
				capDelta = cmp.CapacityDelta
				if cmp.IsRegression {
					fmt.Printf("⚠️  Historical Regression: %s\n", cmp.RegressionItem)
					if failOnRegression {
						fmt.Fprintf(os.Stderr, "❌ Failing CI/CD build due to --fail-on-regression flag.\n")
						os.Exit(1)
					}
				} else if cmp.CapacityDelta != 0 {
					fmt.Printf("📈 Comparison to previous run: Sustainable capacity %+.1f%% (%d ➔ %d users)\n",
						cmp.CapacityDelta, latestPrev.SustainableVUs, rec.SustainableVUs)
				}
			}
		}

		// 6. Output to GitHub Actions Step Summary if running in CI
		if os.Getenv("GITHUB_STEP_SUMMARY") != "" {
			summaryErr := report.AppendGitHubStepSummary(os.Getenv("GITHUB_STEP_SUMMARY"), report.GitHubSummaryParams{
				AppName:           cfg.Application.Name,
				TargetURL:         cfg.Application.URL,
				TargetVUs:         cfg.Workload.MaxUsers,
				MaxObservedVUs:    maxObservedUsers,
				SustainableVUs:    sustainableCapacity,
				PeakRPS:           peakRPS,
				P95LatencyMs:      p95AtPeak,
				PrimaryBottleneck: finalReport.Primary.Component,
				BottleneckFix:     finalReport.Primary.Remediation,
				Sizing:            sizingData,
				Stages:            recordedStages,
				IsRegression:      isReg,
				RegressionItem:    regItem,
				CapacityDelta:     capDelta,
			})
			if summaryErr == nil {
				fmt.Println("📋 GitHub Actions Step Summary updated: " + os.Getenv("GITHUB_STEP_SUMMARY"))
			}
		}

		// 7. Structured Machine-Readable JSON Export
		type BenchmarkJSONOutput struct {
			AppName           string                   `json:"app_name"`
			TargetURL         string                   `json:"target_url"`
			Timestamp         string                   `json:"timestamp"`
			TargetVUs         int                      `json:"target_vus"`
			MaxObservedVUs    int                      `json:"max_observed_vus"`
			SustainableVUs    int                      `json:"sustainable_vus"`
			PeakRPS           float64                  `json:"peak_rps"`
			P95LatencyMs      float64                  `json:"p95_latency_ms"`
			PrimaryBottleneck string                   `json:"primary_bottleneck"`
			BottleneckFix     string                   `json:"bottleneck_fix"`
			SecondaryWarnings []string                 `json:"secondary_warnings,omitempty"`
			NonLimiting       []string                 `json:"non_limiting,omitempty"`
			Sizing            report.SizingData        `json:"sizing"`
			DatabaseTelemetry report.DatabaseTelemetry `json:"database_telemetry"`
			Stages            []report.StageData       `json:"stages"`
		}

		jsonResult := BenchmarkJSONOutput{
			AppName:           cfg.Application.Name,
			TargetURL:         cfg.Application.URL,
			Timestamp:         rec.Timestamp.Format(time.RFC3339),
			TargetVUs:         cfg.Workload.MaxUsers,
			MaxObservedVUs:    maxObservedUsers,
			SustainableVUs:    sustainableCapacity,
			PeakRPS:           peakRPS,
			P95LatencyMs:      p95AtPeak,
			PrimaryBottleneck: finalReport.Primary.Component,
			BottleneckFix:     finalReport.Primary.Remediation,
			SecondaryWarnings: secondaryList,
			NonLimiting:       nonLimitingList,
			Sizing:            sizingData,
			DatabaseTelemetry: dbTelemetry,
			Stages:            recordedStages,
		}

		if outputJSONFile != "" {
			if jsonBytes, err := json.MarshalIndent(jsonResult, "", "  "); err == nil {
				if err := os.WriteFile(outputJSONFile, jsonBytes, 0644); err == nil {
					fmt.Printf("📦 Machine-readable JSON exported: %s\n", outputJSONFile)
				}
			}
		}

		if jsonStdout {
			if jsonBytes, err := json.MarshalIndent(jsonResult, "", "  "); err == nil {
				fmt.Println(string(jsonBytes))
			}
		}
	},
}

func init() {
	runCmd.Flags().StringVarP(&configPath, "config", "c", "capacitylab.yaml", "Path to configuration file")
	runCmd.Flags().StringVarP(&outputReport, "output", "o", "capacity-report.html", "Path to output HTML report")
	runCmd.Flags().BoolVar(&openBrowser, "open", false, "Automatically open report in browser when finished")
	runCmd.Flags().BoolVarP(&runMatrix, "matrix", "m", false, "Run progressive load across configured CPU/RAM matrix tiers")
	runCmd.Flags().StringVar(&overrideURL, "url", "", "Override application target URL")
	runCmd.Flags().IntVar(&overrideMaxUsers, "max-users", 0, "Override maximum concurrent users")
	runCmd.Flags().IntVar(&overrideStartUsers, "start-users", 0, "Override start concurrent users")
	runCmd.Flags().IntVar(&overrideStep, "step", 0, "Override step increment for concurrent users")
	runCmd.Flags().StringVar(&overrideContainer, "container", "", "Override API container name")
	runCmd.Flags().StringVar(&overrideEngine, "engine", "", "Load generator engine: 'native' (default) or 'k6'")
	runCmd.Flags().StringVar(&overrideCPU, "cpu", "", "Constrain API container CPU allocation (e.g. '0.5', '1.0')")
	runCmd.Flags().StringVar(&overrideMemory, "memory", "", "Constrain API container RAM allocation (e.g. '512MB', '1GB')")
	runCmd.Flags().StringVar(&overrideCPUSet, "cpuset", "", "Pin API container to specific CPU cores (e.g. '0,1')")
	runCmd.Flags().BoolVar(&cleanupCompose, "cleanup", false, "Automatically tear down Docker Compose services when test finishes")
	runCmd.Flags().BoolVar(&failOnRegression, "fail-on-regression", false, "Exit with code 1 if a capacity regression is detected")
	runCmd.Flags().BoolVar(&jsonStdout, "json", false, "Output benchmark results as JSON to stdout")
	runCmd.Flags().StringVar(&outputJSONFile, "output-json", "", "Path to write machine-readable JSON results file")
	runCmd.Flags().StringVar(&outputJSONFile, "json-output", "", "Alias for --output-json")
	rootCmd.AddCommand(runCmd)
}
