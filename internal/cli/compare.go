package cli

import (
	"fmt"
	"os"

	"github.com/Abuaslamtech/capacitylab/internal/history"
	"github.com/spf13/cobra"
)

var compareCmd = &cobra.Command{
	Use:   "compare [base-run-id] [target-run-id]",
	Short: "Compare capacity and latency metrics between two benchmark runs",
	Long: `Compares two benchmark runs to identify capacity regressions or throughput improvements.
If no arguments are provided, compares the most recent run against the immediately preceding run.`,
	Run: func(cmd *cobra.Command, args []string) {
		var base, target *history.RunRecord

		if len(args) >= 2 {
			runs, err := history.ListRuns()
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ Failed to read runs: %v\n", err)
				os.Exit(1)
			}
			for i := range runs {
				if runs[i].ID == args[0] {
					b := runs[i]
					base = &b
				}
				if runs[i].ID == args[1] {
					t := runs[i]
					target = &t
				}
			}
			if base == nil {
				fmt.Fprintf(os.Stderr, "❌ Base run '%s' not found.\n", args[0])
				os.Exit(1)
			}
			if target == nil {
				fmt.Fprintf(os.Stderr, "❌ Target run '%s' not found.\n", args[1])
				os.Exit(1)
			}
		} else {
			latest, prev, err := history.GetLatestTwoRuns("")
			if err != nil || latest == nil {
				fmt.Println("❌ Insufficient benchmark runs to compare. At least 2 runs are required.")
				return
			}
			if prev == nil {
				fmt.Println("ℹ️  Only 1 run recorded. Run another benchmark to enable comparison.")
				return
			}
			base = prev
			target = latest
		}

		res := history.Compare(*base, *target)

		fmt.Println("⚖️  CapacityLab Benchmark Comparison")
		fmt.Println("═════════════════════════════════════════════════════════════════════════")
		fmt.Printf("Baseline:  %s (%s • %s)\n", base.ID, base.AppName, base.Timestamp.Format("Jan 02 15:04"))
		fmt.Printf("Current:   %s (%s • %s)\n", target.ID, target.AppName, target.Timestamp.Format("Jan 02 15:04"))
		fmt.Println("─────────────────────────────────────────────────────────────────────────")
		fmt.Printf("%-24s %-14s %-14s %-12s\n", "METRIC", "BASELINE", "CURRENT", "DELTA")
		fmt.Println("─────────────────────────────────────────────────────────────────────────")

		// 1. Sustainable Capacity
		capDeltaStr := fmt.Sprintf("%+.1f%%", res.CapacityDelta)
		fmt.Printf("%-24s %-14s %-14s %-12s\n",
			"Sustainable Capacity",
			fmt.Sprintf("%d users", base.SustainableVUs),
			fmt.Sprintf("%d users", target.SustainableVUs),
			capDeltaStr)

		// 2. Peak Throughput (RPS)
		rpsDeltaStr := fmt.Sprintf("%+.1f%%", res.RPSDelta)
		fmt.Printf("%-24s %-14s %-14s %-12s\n",
			"Peak Throughput (RPS)",
			fmt.Sprintf("%.0f rps", base.PeakRPS),
			fmt.Sprintf("%.0f rps", target.PeakRPS),
			rpsDeltaStr)

		// 3. p95 Latency
		latDeltaStr := fmt.Sprintf("%+.1f%%", res.LatencyDelta)
		fmt.Printf("%-24s %-14s %-14s %-12s\n",
			"p95 Latency",
			fmt.Sprintf("%.1f ms", base.P95LatencyMs),
			fmt.Sprintf("%.1f ms", target.P95LatencyMs),
			latDeltaStr)

		// 4. Primary Bottlenecks
		fmt.Println("─────────────────────────────────────────────────────────────────────────")
		fmt.Printf("Baseline Bottleneck:   %s\n", base.PrimaryBottleneck)
		fmt.Printf("Current Bottleneck:    %s\n", target.PrimaryBottleneck)
		fmt.Println("═════════════════════════════════════════════════════════════════════════")

		if res.IsRegression {
			fmt.Printf("🛑 REGRESSION DETECTED: %s\n", res.RegressionItem)
		} else if res.CapacityDelta > 0 || res.RPSDelta > 0 {
			fmt.Printf("🎉 CAPACITY EXPANDED: Sustainable capacity grew by +%.1f%%!\n", res.CapacityDelta)
		} else {
			fmt.Println("✓ Performance remains stable across runs.")
		}
	},
}

func init() {
	rootCmd.AddCommand(compareCmd)
}
