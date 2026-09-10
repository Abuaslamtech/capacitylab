package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExpandEnvWithDefaults(t *testing.T) {
	os.Setenv("TEST_APP_NAME", "super-api")
	os.Setenv("TEST_PORT", "9090")
	defer os.Unsetenv("TEST_APP_NAME")
	defer os.Unsetenv("TEST_PORT")

	yamlInput := []byte(`
application:
  name: ${TEST_APP_NAME:-default-app}
  url: http://127.0.0.1:${TEST_PORT:-3000}
services:
  api:
    container: ${CONTAINER_NAME:-my-default-container}
`)

	expanded := string(ExpandEnvWithDefaults(yamlInput))

	expectedFragments := []string{
		"name: super-api",
		"url: http://127.0.0.1:9090",
		"container: my-default-container",
	}

	for _, frag := range expectedFragments {
		if !containsSubstring(expanded, frag) {
			t.Errorf("Expected expanded YAML to contain %q, but got:\n%s", frag, expanded)
		}
	}
}

func TestLoadDotEnv(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")

	envContent := `
# Comment line
TEST_SECRET_KEY=secret_12345
export TEST_QUOTED_VAR="quoted_value"
TEST_SINGLE_QUOTE='single_quoted'
`
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("Failed to write temp .env file: %v", err)
	}

	defer os.Unsetenv("TEST_SECRET_KEY")
	defer os.Unsetenv("TEST_QUOTED_VAR")
	defer os.Unsetenv("TEST_SINGLE_QUOTE")

	LoadDotEnv(tempDir)

	if val := os.Getenv("TEST_SECRET_KEY"); val != "secret_12345" {
		t.Errorf("Expected TEST_SECRET_KEY to be 'secret_12345', got %q", val)
	}
	if val := os.Getenv("TEST_QUOTED_VAR"); val != "quoted_value" {
		t.Errorf("Expected TEST_QUOTED_VAR to be 'quoted_value', got %q", val)
	}
	if val := os.Getenv("TEST_SINGLE_QUOTE"); val != "single_quoted" {
		t.Errorf("Expected TEST_SINGLE_QUOTE to be 'single_quoted', got %q", val)
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || filepath.Base(s) != "" && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestConfigValidationPacingAndThrottle(t *testing.T) {
	tempDir := t.TempDir()
	cfgFile := filepath.Join(tempDir, "capacitylab.yaml")

	yamlContent := `version: "1"
application:
  name: test-app
  url: http://localhost:8080
workload:
  type: step-ramp
  start_users: 10
  max_users: 100
  step: 10
  step_duration: 10s
  warmup_duration: 5s
  pacing:
    think_time_min: 150ms
    think_time_max: 500ms
thresholds:
  max_cpu_percent: 85
  max_memory_percent: 80
  max_p95_latency_ms: 250
  max_error_rate_percent: 1.0
  max_cpu_throttle_percent: 20
scenarios:
  - name: health
    weight: 100
    flow:
      - get: /health
`
	if err := os.WriteFile(cfgFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write yaml: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Workload.Pacing.ThinkTimeMin != 150*time.Millisecond {
		t.Errorf("Expected think_time_min 150ms, got %v", cfg.Workload.Pacing.ThinkTimeMin)
	}
	if cfg.Workload.Pacing.ThinkTimeMax != 500*time.Millisecond {
		t.Errorf("Expected think_time_max 500ms, got %v", cfg.Workload.Pacing.ThinkTimeMax)
	}
	if cfg.Thresholds.MaxCPUThrottlePercent != 20.0 {
		t.Errorf("Expected max_cpu_throttle_percent 20.0, got %v", cfg.Thresholds.MaxCPUThrottlePercent)
	}
}

func TestConfigAPIServiceAndEngine(t *testing.T) {
	tempDir := t.TempDir()
	cfgFile := filepath.Join(tempDir, "capacitylab.yaml")

	yamlContent := `version: "1"
application:
  name: test-app
  url: http://localhost:8080
services:
  api:
    container: my-api
    cpu: "1.5"
    memory: "1GB"
    cpuset: "0,1"
workload:
  type: step-ramp
  engine: k6
  start_users: 10
  max_users: 50
  step: 10
  step_duration: 5s
  warmup_duration: 2s
thresholds:
  max_cpu_percent: 85
  max_memory_percent: 80
  max_p95_latency_ms: 250
  max_error_rate_percent: 1.0
scenarios:
  - name: test
    weight: 100
    flow:
      - get: /ping
`
	if err := os.WriteFile(cfgFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write yaml: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Services.API.CPU != "1.5" || cfg.Services.API.Memory != "1GB" || cfg.Services.API.CPUSet != "0,1" {
		t.Errorf("Unexpected API resource config: %+v", cfg.Services.API)
	}
	if cfg.Workload.Engine != "k6" {
		t.Errorf("Expected workload.engine to be 'k6', got %q", cfg.Workload.Engine)
	}
}

func TestConfigWorkloadModes(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("Soak workload defaults max_users and duration", func(t *testing.T) {
		cfgFile := filepath.Join(tempDir, "soak.yaml")
		yamlContent := `version: "1"
application:
  name: soak-app
  url: http://localhost:8080
workload:
  type: soak
  start_users: 25
thresholds:
  max_p95_latency_ms: 500
scenarios:
  - name: health
    weight: 100
    flow:
      - get: /health
`
		if err := os.WriteFile(cfgFile, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to write yaml: %v", err)
		}

		cfg, err := Load(cfgFile)
		if err != nil {
			t.Fatalf("Failed to load soak config: %v", err)
		}

		if cfg.Workload.Type != "soak" {
			t.Errorf("Expected workload type 'soak', got %s", cfg.Workload.Type)
		}
		if cfg.Workload.MaxUsers != 25 {
			t.Errorf("Expected max_users to default to start_users (25), got %d", cfg.Workload.MaxUsers)
		}
		if cfg.Workload.Duration != 60*time.Second {
			t.Errorf("Expected default duration of 60s, got %v", cfg.Workload.Duration)
		}
	})

	t.Run("Spike workload validation", func(t *testing.T) {
		cfgFile := filepath.Join(tempDir, "spike.yaml")
		yamlContent := `version: "1"
application:
  name: spike-app
  url: http://localhost:8080
workload:
  type: spike
  start_users: 10
  max_users: 100
thresholds:
  max_p95_latency_ms: 500
scenarios:
  - name: health
    weight: 100
    flow:
      - get: /health
`
		if err := os.WriteFile(cfgFile, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to write yaml: %v", err)
		}

		cfg, err := Load(cfgFile)
		if err != nil {
			t.Fatalf("Failed to load spike config: %v", err)
		}

		if cfg.Workload.Type != "spike" {
			t.Errorf("Expected workload type 'spike', got %s", cfg.Workload.Type)
		}
	})
}
