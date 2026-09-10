package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Abuaslamtech/capacitylab/internal/dashboard"
	"github.com/Abuaslamtech/capacitylab/internal/report"
	"github.com/spf13/cobra"
)

var (
	dashboardPort int
	dashboardOpen bool
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Start the local live web dashboard and trends explorer",
	Long:  `Spins up a lightweight local web server on localhost:4242 serving real-time SSE telemetry streams and historical benchmark comparisons.`,
	Run: func(cmd *cobra.Command, args []string) {
		srv := dashboard.NewServer(dashboardPort)
		url := fmt.Sprintf("http://localhost:%d", dashboardPort)

		fmt.Println("🚀 CapacityLab Live Dashboard Server")
		fmt.Println("═════════════════════════════════════════════════════════════")
		fmt.Printf("🌐 Serving on:                     %s\n", url)
		fmt.Println("📊 Real-time SSE telemetry stream: /api/stream")
		fmt.Println("📜 Historical runs API:           /api/runs")
		fmt.Println("Press Ctrl+C to terminate.")
		fmt.Println("═════════════════════════════════════════════════════════════")

		if dashboardOpen {
			_ = report.OpenInBrowser(url)
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		if err := srv.Start(ctx); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "❌ Server error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	dashboardCmd.Flags().IntVarP(&dashboardPort, "port", "p", 4242, "Port for the local web dashboard server")
	dashboardCmd.Flags().BoolVar(&dashboardOpen, "open", true, "Automatically open dashboard in your default browser")
	rootCmd.AddCommand(dashboardCmd)
}
