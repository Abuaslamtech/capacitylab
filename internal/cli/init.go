package cli

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	forceOverwrite bool
	initType       string
	initURL        string
	initJourney    bool
	nonInteractive bool
)

// GenerateConfig produces tailored YAML configuration based on target architecture and scenarios
func GenerateConfig(targetType, targetURL, appName string, includeJourney bool) string {
	if appName == "" {
		appName = deriveAppName(targetURL)
	}

	targetType = strings.ToLower(strings.TrimSpace(targetType))
	if targetType == "docker" || targetType == "compose" || targetType == "local" {
		return generateDockerConfig(appName, targetURL)
	}

	return generateAPIConfig(appName, targetURL, includeJourney)
}

func deriveAppName(rawURL string) string {
	if rawURL == "" {
		return "my-api"
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "my-api"
	}
	host := u.Hostname()
	parts := strings.Split(host, ".")
	if len(parts) > 0 && parts[0] != "localhost" && parts[0] != "127" {
		prefix := strings.Split(parts[0], "-")[0]
		if len(prefix) > 0 {
			return prefix + "-api"
		}
	}
	return "my-api"
}

func generateAPIConfig(appName, targetURL string, includeJourney bool) string {
	if targetURL == "" {
		targetURL = "http://localhost:3000"
	}

	if includeJourney {
		return fmt.Sprintf(`# CapacityLab Configuration (Hosted / Live API)
version: "1"

# 1. Target Application Definition
application:
  name: %s
  url: %s

# 2. Workload Step Ramping
workload:
  type: step-ramp
  engine: native
  start_users: 2
  max_users: 30
  step: 5
  step_duration: 15s
  warmup_duration: 5s
  pacing:
    think_time_min: 200ms
    think_time_max: 600ms

# 3. Saturation & SLA Thresholds
thresholds:
  max_cpu_percent: 85
  max_memory_percent: 80
  max_p95_latency_ms: 1500
  max_error_rate_percent: 5.0

# 4. Multi-Step Stateful User Journey
scenarios:
  - name: user_lifecycle_journey
    weight: 100
    flow:
      # Step 1: Register unique user (dynamic UUID prevents collision errors)
      - post:
          path: /auth/register
          headers:
            Content-Type: application/json
          body:
            name: "Test User"
            email: "user_{{$uuid}}@example.com"
            password: "Password123!"

      # Step 2: Login with credentials (extracts response.body.token into session)
      - post:
          path: /auth/login
          headers:
            Content-Type: application/json
          body:
            email: "user_{{$uuid}}@example.com"
            password: "Password123!"

      # Step 3: Access Protected Endpoint using extracted JWT
      - get:
          path: /users/me
          headers:
            Authorization: "Bearer {{response.body.token}}"
`, appName, targetURL)
	}

	return fmt.Sprintf(`# CapacityLab Configuration (Hosted / Live API)
version: "1"

# 1. Target Application Definition
application:
  name: %s
  url: %s

# 2. Workload Step Ramping
workload:
  type: step-ramp
  engine: native
  start_users: 5
  max_users: 50
  step: 10
  step_duration: 15s
  warmup_duration: 5s
  pacing:
    think_time_min: 100ms
    think_time_max: 400ms

# 3. Saturation & SLA Thresholds
thresholds:
  max_cpu_percent: 85
  max_memory_percent: 80
  max_p95_latency_ms: 1000
  max_error_rate_percent: 2.0

# 4. Endpoints to Benchmark
scenarios:
  - name: browse_endpoints
    weight: 100
    flow:
      - get: /health
      - get: /api/v1/items?limit=20
`, appName, targetURL)
}

func generateDockerConfig(appName, targetURL string) string {
	if targetURL == "" {
		targetURL = "http://localhost:3000"
	}

	return fmt.Sprintf(`# CapacityLab Configuration (Local Docker Stack)
version: "1"

# 1. Target Application Definition
application:
  name: %s
  url: %s
  startup:
    compose_file: ./docker-compose.yml
    wait_for_health: true
    timeout: 60s

# 2. Local Container & Resource Profiling (measures CPU/RAM throttling)
services:
  api:
    container: my-api-container
    test_matrix:
      cpu: ["0.5", "1.0", "2.0"]
      memory: ["512MB", "1GB", "2GB"]

# 3. Workload Step Ramping
workload:
  type: step-ramp
  engine: native
  start_users: 10
  max_users: 500
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
  max_p95_latency_ms: 500
  max_error_rate_percent: 1.0

# 5. Scenarios
scenarios:
  - name: api_flow
    weight: 100
    flow:
      - get: /health
      - get: /api/v1/records
`, appName, targetURL)
}

// RunInitWizard executes the interactive prompt or uses provided flags to create capacitylab.yaml
func RunInitWizard(in io.Reader, out io.Writer, configFile string) error {
	// Check if file already exists
	if _, err := os.Stat(configFile); err == nil && !forceOverwrite {
		fmt.Fprintf(out, "⚠️  '%s' already exists. Use --force to overwrite.\n", configFile)
		return nil
	}

	chosenType := initType
	chosenURL := initURL
	chosenJourney := initJourney

	scanner := bufio.NewScanner(in)

	if !nonInteractive && chosenType == "" {
		fmt.Fprint(out, BrandBanner("Interactive Configuration Wizard"))
		fmt.Fprintln(out, "  "+Dim("Configure a tailored capacity benchmark for your application."))
		fmt.Fprintln(out)

		fmt.Fprintln(out, "  "+BrightCyan("? ")+Bold("What type of application are you testing?"))
		fmt.Fprintln(out, "    "+IconRadioActive()+" "+Bold("1) Hosted / Live API")+"    "+Dim("› Render, AWS, Fly.io, staging, or local server"))
		fmt.Fprintln(out, "    "+IconRadioInactive()+" "+Bold("2) Local Docker Stack")+"   "+Dim("› Measure container CPU/RAM limits with Docker Compose"))
		fmt.Fprint(out, "  "+IconPointer()+" Selection "+Dim("[1]")+": ")

		if scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			if text == "2" {
				chosenType = "docker"
			} else {
				chosenType = "api"
			}
		}

		defaultURL := "http://localhost:3000"
		if chosenType == "api" {
			defaultURL = "https://api.example.com"
		}

		if chosenURL == "" {
			fmt.Fprintln(out, "\n  "+BrightCyan("? ")+Bold("Target Base URL")+" "+Dim(fmt.Sprintf("[%s]", defaultURL)))
			fmt.Fprint(out, "  "+IconPointer()+" URL: ")
			if scanner.Scan() {
				urlInput := strings.TrimSpace(scanner.Text())
				if urlInput != "" {
					chosenURL = urlInput
				} else {
					chosenURL = defaultURL
				}
			}
		}

		if chosenType == "api" && !cmdFlagsChanged("auth") {
			fmt.Fprintln(out, "\n  "+BrightCyan("? ")+Bold("What scenario structure would you like?"))
			fmt.Fprintln(out, "    "+IconRadioActive()+" "+Bold("1) Multi-step User Journey")+"  "+Dim("› Register ➔ Login ➔ Token Extract ➔ Protected API"))
			fmt.Fprintln(out, "    "+IconRadioInactive()+" "+Bold("2) Simple Endpoint Test")+"     "+Dim("› Quick smoke test on GET endpoints"))
			fmt.Fprint(out, "  "+IconPointer()+" Selection "+Dim("[1]")+": ")

			if scanner.Scan() {
				text := strings.TrimSpace(scanner.Text())
				if text == "2" {
					chosenJourney = false
				} else {
					chosenJourney = true
				}
			}
		}
	}

	if chosenType == "" {
		chosenType = "api"
	}
	if chosenURL == "" {
		chosenURL = "http://localhost:3000"
	}

	appName := deriveAppName(chosenURL)
	content := GenerateConfig(chosenType, chosenURL, appName, chosenJourney)

	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to create configuration file: %w", err)
	}

	cardRows := [][2]string{
		{"File", Cyan(configFile)},
		{"Target", BrightCyan(chosenURL)},
		{"Mode", BadgeCyan(strings.ToUpper(chosenType))},
		{"Engine", "Native Pooled Go Runner"},
	}
	fmt.Fprint(out, Card(IconSuccess()+"  Configuration Created", cardRows))

	fmt.Fprintln(out, "\n  "+Bold("Next steps:"))
	fmt.Fprintf(out, "    %s Review and customize endpoints in %s\n", IconArrow(), Cyan(configFile))
	fmt.Fprintf(out, "    %s Run %s to check connectivity\n", IconArrow(), Bold("capacitylab validate"))
	fmt.Fprintf(out, "    %s Run %s to launch the capacity test\n\n", IconArrow(), Bold("capacitylab run --open"))

	return nil
}

func cmdFlagsChanged(name string) bool {
	f := initCmd.Flags().Lookup(name)
	return f != nil && f.Changed
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a tailored capacitylab.yaml configuration file",
	Long: `Creates a new, tailored capacitylab.yaml configuration file in the current directory.
Can be run interactively as a guided wizard, or non-interactively using flags.`,
}

func init() {
	initCmd.Run = func(cmd *cobra.Command, args []string) {
		configFile := "capacitylab.yaml"
		if err := RunInitWizard(os.Stdin, os.Stdout, configFile); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to initialize configuration: %v\n", err)
			os.Exit(1)
		}
	}

	initCmd.Flags().BoolVarP(&forceOverwrite, "force", "f", false, "Overwrite existing capacitylab.yaml")
	initCmd.Flags().StringVarP(&initType, "type", "t", "", "Application type: 'api' (hosted/remote) or 'docker' (local compose)")
	initCmd.Flags().StringVarP(&initURL, "url", "u", "", "Base URL of the target application")
	initCmd.Flags().BoolVar(&initJourney, "auth", false, "Include multi-step authentication journey scenario")
	initCmd.Flags().BoolVarP(&nonInteractive, "yes", "y", false, "Skip interactive prompts and use defaults/flags")

	rootCmd.AddCommand(initCmd)
}
