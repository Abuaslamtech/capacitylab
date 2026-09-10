package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Default starter YAML configuration
const defaultStarterConfig = `# CapacityLab Configuration
version: "1"

# 1. Target Application Definition
application:
  name: my-backend
  url: http://localhost:3000
  startup:
    compose_file: ./docker-compose.yml
    wait_for_health: true
    timeout: 60s

# 2. Services to profile under controlled resource limits
services:
  api:
    container: my-api-container
    test_matrix:
      cpu: ["0.5", "1.0", "2.0"]
      memory: ["512MB", "1GB", "2GB"]

    postgres:
      container: my-postgres-container
      dsn: "postgres://postgres:postgres@localhost:5432/my_db?sslmode=disable"
      max_connections: 100

    redis:
      container: my-redis-container
      addr: "localhost:6379"

# 3. Workload Step Ramping
workload:
  type: step-ramp
  start_users: 10
  max_users: 1000
  step: 50
  step_duration: 30s
  warmup_duration: 15s
  pacing:
    think_time_min: 200ms
    think_time_max: 800ms

# 4. Saturation & SLA Thresholds
thresholds:
  max_cpu_percent: 85
  max_memory_percent: 80
  max_p95_latency_ms: 250
  max_error_rate_percent: 1.0

# 5. Realistic User Journeys
scenarios:
  - name: browse_dashboard
    weight: 50
    flow:
      - get: /api/v1/health
      - get: /api/v1/dashboard

  - name: create_record
    weight: 50
    flow:
      - post:
          path: /api/v1/records
          body:
            name: "test-item"
`

var forceOverwrite bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a starter capacitylab.yaml configuration file",
	Long:  `Creates a new, annotated capacitylab.yaml configuration file in the current directory.`,
	Run: func(cmd *cobra.Command, args []string) {
		configFile := "capacitylab.yaml"

		// Check if file already exists
		if _, err := os.Stat(configFile); err == nil && !forceOverwrite {
			fmt.Printf("⚠️  '%s' already exists. Use --force to overwrite.\n", configFile)
			return
		}

		// Write the default configuration file
		err := os.WriteFile(configFile, []byte(defaultStarterConfig), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to create configuration file: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ Created starter configuration: %s\n\n", configFile)
		fmt.Println("Next steps:")
		fmt.Println("  1. Edit capacitylab.yaml to match your local service endpoints.")
		fmt.Println("  2. Run 'capacitylab validate' to check your configuration.")
	},
}

func init() {
	// Add the --force (or -f) flag to initCmd
	initCmd.Flags().BoolVarP(&forceOverwrite, "force", "f", false, "Overwrite existing capacitylab.yaml")

	// Register with root
	rootCmd.AddCommand(initCmd)
}
