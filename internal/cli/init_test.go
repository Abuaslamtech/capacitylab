package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateConfig(t *testing.T) {
	t.Run("API mode without auth journey", func(t *testing.T) {
		out := GenerateConfig("api", "https://api.example.com", "test-api", false)
		if !strings.Contains(out, "url: https://api.example.com") {
			t.Errorf("Expected URL in output: %s", out)
		}
		if !strings.Contains(out, "browse_endpoints") {
			t.Errorf("Expected simple endpoint scenario in output: %s", out)
		}
		if strings.Contains(out, "services:") || strings.Contains(out, "compose_file") {
			t.Errorf("Hosted API config should not contain Docker services or compose file")
		}
	})

	t.Run("API mode with auth journey", func(t *testing.T) {
		out := GenerateConfig("api", "https://sample-api.onrender.com/api/v1", "", true)
		if !strings.Contains(out, "name: sample-api") {
			t.Errorf("Expected derived name 'sample-api' in output: %s", out)
		}
		if !strings.Contains(out, "user_lifecycle_journey") {
			t.Errorf("Expected auth lifecycle journey in output: %s", out)
		}
		if !strings.Contains(out, "{{response.body.token}}") {
			t.Errorf("Expected session token chaining in output: %s", out)
		}
		if strings.Contains(out, "services:") || strings.Contains(out, "compose_file") {
			t.Errorf("Hosted API config should not contain Docker services or compose file")
		}
	})

	t.Run("Docker mode", func(t *testing.T) {
		out := GenerateConfig("docker", "http://localhost:8080", "docker-app", false)
		if !strings.Contains(out, "compose_file: ./docker-compose.yml") {
			t.Errorf("Expected compose_file in Docker config: %s", out)
		}
		if !strings.Contains(out, "services:") {
			t.Errorf("Expected services section in Docker config: %s", out)
		}
	})
}

func TestRunInitWizardInteractive(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "capacitylab.yaml")

	// Simulate user typing: "1" (API) -> Enter, "https://my-service.com" -> Enter, "1" (Journey) -> Enter
	input := "1\nhttps://my-service.com\n1\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := RunInitWizard(in, &out, configFile)
	if err != nil {
		t.Fatalf("RunInitWizard failed: %v", err)
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read generated config: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "url: https://my-service.com") {
		t.Errorf("Expected configured URL in output file, got:\n%s", content)
	}
	if !strings.Contains(content, "user_lifecycle_journey") {
		t.Errorf("Expected journey scenario in output file, got:\n%s", content)
	}
}
