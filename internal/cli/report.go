package cli

import (
	"fmt"
	"os"

	"github.com/Abuaslamtech/capacitylab/internal/analyzer"
	"github.com/Abuaslamtech/capacitylab/internal/history"
	"github.com/Abuaslamtech/capacitylab/internal/report"
	"github.com/spf13/cobra"
)

var (
	reportOutput string
	reportOpen   bool
)

var reportCmd = &cobra.Command{
	Use:   "report [run-id]",
	Short: "Regenerate standalone interactive HTML report from historical benchmark data",
	Long: `Reads a raw benchmark run snapshot from ~/.capacitylab/history/ and regenerates
a zero-dependency interactive HTML capacity report file.
If no run-id is specified, the most recent benchmark run is used.`,
	Run: func(cmd *cobra.Command, args []string) {
		var selectedRun *history.RunRecord

		if len(args) > 0 {
			runID := args[0]
			runs, err := history.ListRuns()
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ Failed to read run history: %v\n", err)
				os.Exit(1)
			}
			for i := range runs {
				if runs[i].ID == runID {
					selectedRun = &runs[i]
					break
				}
			}
			if selectedRun == nil {
				fmt.Fprintf(os.Stderr, "❌ Run '%s' not found in ~/.capacitylab/history/\n", runID)
				os.Exit(1)
			}
		} else {
			latest, _, err := history.GetLatestTwoRuns("")
			if err != nil || latest == nil {
				fmt.Fprintf(os.Stderr, "❌ No recorded benchmark runs found. Execute 'capacitylab run' first.\n")
				os.Exit(1)
			}
			selectedRun = latest
		}

		fmt.Printf("📄 Regenerating HTML report for run: %s (%s)\n", selectedRun.ID, selectedRun.AppName)

		var peakCPU float64
		var peakRAM float64
		for _, s := range selectedRun.Stages {
			if s.CPUPercent > peakCPU {
				peakCPU = s.CPUPercent
			}
			if s.MemoryMB > peakRAM {
				peakRAM = s.MemoryMB
			}
		}

		hwRec := analyzer.ComputeRecommendation(
			selectedRun.TargetVUs,
			selectedRun.MaxObservedVUs,
			peakCPU,
			peakRAM,
			0,
			0,
			0,
		)

		sizing := report.SizingData{
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

		if reportOutput == "" {
			reportOutput = fmt.Sprintf("capacity-report-%s.html", selectedRun.ID)
		}

		containerName := ""
		if peakCPU > 0 || peakRAM > 0 {
			containerName = selectedRun.AppName
		}

		err := report.GenerateHTML(
			reportOutput,
			selectedRun.AppName,
			selectedRun.TargetURL,
			containerName,
			selectedRun.PrimaryBottleneck,
			selectedRun.BottleneckFix,
			nil,
			[]string{"Historical Run Snapshot"},
			selectedRun.TargetVUs,
			selectedRun.MaxObservedVUs,
			selectedRun.SustainableVUs,
			selectedRun.Stages,
			report.DatabaseTelemetry{},
			nil,
			sizing,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to generate report: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ Interactive HTML report generated: %s\n", reportOutput)
		if reportOpen {
			_ = report.OpenInBrowser(reportOutput)
		}
	},
}

func init() {
	reportCmd.Flags().StringVarP(&reportOutput, "output", "o", "", "Path to output HTML report (defaults to capacity-report-<run-id>.html)")
	reportCmd.Flags().BoolVar(&reportOpen, "open", true, "Automatically open report in browser")
	rootCmd.AddCommand(reportCmd)
}
