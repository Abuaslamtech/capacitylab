package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/Abuaslamtech/capacitylab/internal/config"
	"github.com/Abuaslamtech/capacitylab/internal/runtime"
	"github.com/spf13/cobra"
)

var configPath string

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate your capacitylab.yaml configuration and check Docker connectivity",
	Long:  `Loads and validates the syntax of capacitylab.yaml, then checks Docker daemon connectivity and container health.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprint(os.Stdout, BrandBanner("Configuration Validator"))
		fmt.Printf("  %s %s\n\n", Dim("Config File:"), Cyan(configPath))

		cfg, err := config.Load(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s %s\n     %v\n\n", IconError(), Bold(Red("Validation Failed")), err)
			os.Exit(1)
		}

		// Print clean summary of validated settings
		fmt.Printf("  %s %-20s %s\n", IconSuccess(), Dim("Version"), Bold(cfg.Version))
		fmt.Printf("  %s %-20s %s %s\n", IconSuccess(), Dim("Application"), Bold(cfg.Application.Name), Cyan("("+cfg.Application.URL+")"))
		fmt.Printf("  %s %-20s %d %s %d users %s\n", IconSuccess(), Dim("Workload Ramping"),
			cfg.Workload.StartUsers, IconArrow(), cfg.Workload.MaxUsers, Dim(fmt.Sprintf("(step: +%d)", cfg.Workload.Step)))
		fmt.Printf("  %s %-20s warmup: %s, step: %s\n", IconSuccess(), Dim("Stage Durations"),
			Cyan(cfg.Workload.WarmupDuration.String()), Cyan(cfg.Workload.StepDuration.String()))
		fmt.Printf("  %s %-20s CPU ≤ %.0f%%, Mem ≤ %.0f%%, p95 ≤ %.0fms, Errors ≤ %.1f%%\n",
			IconSuccess(), Dim("SLA Thresholds"),
			cfg.Thresholds.MaxCPUPercent, cfg.Thresholds.MaxMemoryPercent,
			cfg.Thresholds.MaxP95LatencyMs, cfg.Thresholds.MaxErrorRatePercent)
		fmt.Printf("  %s %-20s %d defined %s\n\n", IconSuccess(), Dim("Scenarios"),
			len(cfg.Scenarios), Dim("(total weight: 100%)"))

		// Check Docker daemon connection (optional for hosted APIs)
		isLocalDocker := cfg.Application.Startup.ComposeFile != "" || cfg.Services.API.Container != ""
		dockerClient, err := runtime.NewClient()
		if err != nil {
			if isLocalDocker {
				fmt.Printf("  %s %s: %v\n", IconWarning(), Bold(Yellow("Docker Daemon")), err)
				fmt.Println("\n  Configuration syntax is valid, but Docker is required for local container profiling.")
			} else {
				fmt.Printf("  %s %s %s\n", IconBullet(), Dim("Docker Engine"), Dim("(skipped — testing hosted cloud target)"))
			}
		} else {
			ctx := context.Background()
			if envInfo, err := dockerClient.DetectEnvironment(ctx); err == nil {
				hostType := "Native Linux"
				if envInfo.IsDockerDesktop {
					hostType = "Docker Desktop VM"
				} else if envInfo.IsWSL2 {
					hostType = "WSL2 Virtualized"
				}
				fmt.Printf("  %s %-20s %s %s\n", IconSuccess(), Dim("Host Topology"),
					Bold(hostType), Dim(fmt.Sprintf("(Daemon RAM: %.1f GB, Cores: %d)", envInfo.TotalMemMB/1024, envInfo.CPUs)))
				if envInfo.Warning != "" {
					fmt.Printf("  %s %s\n", IconWarning(), Yellow(envInfo.Warning))
				}
			}

			if cfg.Application.Startup.ComposeFile != "" {
				if _, err := os.Stat(cfg.Application.Startup.ComposeFile); err == nil {
					fmt.Printf("  %s %-20s %s\n", IconSuccess(), Dim("Compose File"), Green(cfg.Application.Startup.ComposeFile))
				} else {
					fmt.Printf("  %s %-20s %s %s\n", IconWarning(), Dim("Compose File"), Yellow(cfg.Application.Startup.ComposeFile), Dim("(not found)"))
				}
			}

			if cfg.Services.API.Container != "" {
				running, status, err := dockerClient.ContainerStatus(ctx, cfg.Services.API.Container)
				if err != nil {
					fmt.Printf("  %s %-20s %s %s\n", IconWarning(), Dim("API Container"), Yellow(cfg.Services.API.Container), Dim("(not found on local host)"))
				} else {
					fmt.Printf("  %s %-20s %s %s\n", IconSuccess(), Dim("API Container"), Bold(cfg.Services.API.Container), Green(fmt.Sprintf("[%s]", status)))
					_ = running
				}
			}
		}

		fmt.Printf("\n  %s %s\n", IconSuccess(), Bold(Green("Configuration is valid and ready to run!")))
		fmt.Printf("     Execute: %s\n\n", Bold(BrightCyan("capacitylab run --open")))
	},
}

func init() {
	validateCmd.Flags().StringVarP(&configPath, "config", "c", "capacitylab.yaml", "Path to configuration file")
	rootCmd.AddCommand(validateCmd)
}
