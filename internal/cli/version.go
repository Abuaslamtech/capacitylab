package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = "v0.1.0-alpha"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of CapacityLab",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("CapacityLab %s (linux/amd64)\n", version)
	},
}

func init() {
	// init() runs automatically when Go loads this file
	// This registers 'version' as a child of the root command
	rootCmd.AddCommand(versionCmd)
}
