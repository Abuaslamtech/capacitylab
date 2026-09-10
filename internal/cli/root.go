package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "capacitylab",
	Short: "CapacityLab: discover your backend's real capacity boundary and right-sized infrastructure",
	Long: BrandBanner("Autonomous Infrastructure & Capacity Sizing Engine") + `
  Simulates realistic multi-step user workloads, discovers capacity boundaries,
  pinpoints bottlenecks, and estimates candidate starting cloud infrastructure.`,
	Run: func(cmd *cobra.Command, args []string) {
		// If user types just 'capacitylab', show the help menu
		_ = cmd.Help()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main().
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
