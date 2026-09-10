package cli

import (
	"fmt"
	"os"

	"github.com/Abuaslamtech/capacitylab/internal/history"
	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View historical benchmark runs and capacity trends",
	Long:  `Displays a chronological history of past capacity benchmarks, highlighting sustainable load and bottlenecks.`,
	Run: func(cmd *cobra.Command, args []string) {
		runs, err := history.ListRuns()
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to read history: %v\n", err)
			os.Exit(1)
		}

		if len(runs) == 0 {
			fmt.Println("ℹ️  No historical benchmark runs found.")
			fmt.Println("   Execute 'capacitylab run' to capture your first baseline!")
			return
		}

		fmt.Println("📜 CapacityLab Benchmark Run History")
		fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════")
		fmt.Printf("%-24s %-16s %-16s %-12s %-10s %-18s\n",
			"RUN ID", "DATE", "APP", "SUSTAINABLE", "PEAK RPS", "BOTTLENECK")
		fmt.Println("───────────────────────────────────────────────────────────────────────────────────────────────")

		for _, r := range runs {
			dateStr := r.Timestamp.Format("Jan 02 15:04")
			sustStr := fmt.Sprintf("~%d users", r.SustainableVUs)
			rpsStr := fmt.Sprintf("%.0f", r.PeakRPS)
			bottleneck := r.PrimaryBottleneck
			if len(bottleneck) > 20 {
				bottleneck = bottleneck[:17] + "..."
			}
			if bottleneck == "" {
				bottleneck = "None"
			}

			fmt.Printf("%-24s %-16s %-16s %-12s %-10s %-18s\n",
				r.ID, dateStr, r.AppName, sustStr, rpsStr, bottleneck)
		}

		fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════════════")
		fmt.Printf("💡 Total: %d runs recorded in ~/.capacitylab/history/\n", len(runs))
		fmt.Println("   Compare runs with: capacitylab compare <run-id-1> <run-id-2>")
	},
}

func init() {
	rootCmd.AddCommand(historyCmd)
}
