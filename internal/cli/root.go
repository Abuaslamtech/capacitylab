package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "capacitylab",
	Short: "CapacityLab is a developer infrastructure & capacity planning tool",
	Long: `CapacityLab runs your backend locally under controlled Docker cgroup resources,
    simulates realistic multi-step user workloads, detects saturation points, 
    and estimates the exact production infrastructure required for your application.`,
	Run: func(cmd *cobra.Command, args []string) {
		// If user types just 'capacitylab', show the help menu
		cmd.Help()
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
