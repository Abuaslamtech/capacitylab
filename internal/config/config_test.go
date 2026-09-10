package config

import (
	"os"
	"path/filepath"
	"testing"
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
