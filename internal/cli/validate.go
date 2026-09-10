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
		fmt.Printf("🔍 Validating configuration: %s\n\n", configPath)

		cfg, err := config.Load(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Validation failed:\n   %v\n\n", err)
			os.Exit(1)
		}

		// Print clean summary of validated settings
		fmt.Printf("  ✓ Version:               %s\n", cfg.Version)
		fmt.Printf("  ✓ Application:           %s (%s)\n", cfg.Application.Name, cfg.Application.URL)
		fmt.Printf("  ✓ Workload Ramping:      %d ➔ %d users (step: %d)\n",
			cfg.Workload.StartUsers, cfg.Workload.MaxUsers, cfg.Workload.Step)
		fmt.Printf("  ✓ Stage Durations:       warmup: %v, step: %v\n",
			cfg.Workload.WarmupDuration, cfg.Workload.StepDuration)
		fmt.Printf("  ✓ SLA Thresholds:        CPU ≤ %.0f%%, Mem ≤ %.0f%%, p95 ≤ %.0fms, Errors ≤ %.1f%%\n",
			cfg.Thresholds.MaxCPUPercent, cfg.Thresholds.MaxMemoryPercent,
			cfg.Thresholds.MaxP95LatencyMs, cfg.Thresholds.MaxErrorRatePercent)
		fmt.Printf("  ✓ Scenarios:             %d defined (total weight: 100%%)\n\n", len(cfg.Scenarios))

		// Check Docker daemon connection
		fmt.Println("🐳 Checking Docker Environment...")
		dockerClient, err := runtime.NewClient()
		if err != nil {
			fmt.Printf("  ⚠️  Docker Daemon: %v\n", err)
			fmt.Println("\nConfiguration syntax is valid, but Docker is not currently reachable.")
			return
		}
		fmt.Println("  ✓ Docker Daemon is active and connected")
		ctx := context.Background()
		if cfg.Services.API.Container != "" {
			running, status, err := dockerClient.ContainerStatus(ctx, cfg.Services.API.Container)
			if err != nil {
				fmt.Printf("  ⚠️  API Container ('%s'): not found on local host\n", cfg.Services.API.Container)
			} else {
				fmt.Printf("  ✓ API Container ('%s'): %s (running: %t)\n", cfg.Services.API.Container, status, running)
			}
		}

		// Check if configured containers exist
		fmt.Println("\n🎉 Ready for benchmark runs!")
	},
}

func init() {
	validateCmd.Flags().StringVarP(&configPath, "config", "c", "capacitylab.yaml", "Path to configuration file")
	rootCmd.AddCommand(validateCmd)
}
