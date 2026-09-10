package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Abuaslamtech/capacitylab/internal/history"
	"github.com/Abuaslamtech/capacitylab/internal/report"
	"github.com/spf13/cobra"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Open the interactive multi-run historical trends dashboard",
	Long:  `Generates and opens an interactive HTML dashboard tracking capacity growth, throughput trends, and bottlenecks across all past benchmark runs.`,
	Run: func(cmd *cobra.Command, args []string) {
		runs, err := history.ListRuns()
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to read historical runs: %v\n", err)
			os.Exit(1)
		}

		if len(runs) == 0 {
			fmt.Println("ℹ️  No historical benchmark runs found.")
			fmt.Println("   Execute 'capacitylab run' first to generate your baseline!")
			return
		}

		var summaries []report.DashboardRunSummary
		for _, r := range runs {
			summaries = append(summaries, report.DashboardRunSummary{
				ID:             r.ID,
				Date:           r.Timestamp.Format("Jan 02 15:04"),
				AppName:        r.AppName,
				TargetURL:      r.TargetURL,
				SustainableVUs: r.SustainableVUs,
				MaxObservedVUs: r.MaxObservedVUs,
				PeakRPS:        r.PeakRPS,
				P95Ms:          r.P95LatencyMs,
				Bottleneck:     r.PrimaryBottleneck,
				Fix:            r.BottleneckFix,
			})
		}

		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to locate user home directory: %v\n", err)
			os.Exit(1)
		}

		dashboardPath := filepath.Join(home, ".capacitylab", "dashboard.html")
		err = report.GenerateDashboardHTML(dashboardPath, summaries)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to generate dashboard: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("📊 CapacityLab Historical Dashboard Generated!")
		fmt.Printf("   File: %s\n", dashboardPath)
		fmt.Printf("   Recorded runs: %d\n", len(runs))
		fmt.Println("🌐 Opening dashboard in your default browser...")
		_ = report.OpenInBrowser(dashboardPath)
	},
}

func init() {
	rootCmd.AddCommand(dashboardCmd)
}
