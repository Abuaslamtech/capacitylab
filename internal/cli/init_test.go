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

func TestRunInitWizardForceOverwrite(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "capacitylab.yaml")

	originalContent := "existing_config: true\n"
	if err := os.WriteFile(configFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to write initial config: %v", err)
	}

	// 1. Without --force: should NOT overwrite
	var out bytes.Buffer
	err := RunInitWizard(strings.NewReader(""), &out, configFile)
	if err != nil {
		t.Fatalf("RunInitWizard failed without force: %v", err)
	}
	if !strings.Contains(out.String(), "already exists") {
		t.Errorf("Expected 'already exists' warning, got: %s", out.String())
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(data) != originalContent {
		t.Errorf("File was modified without --force: %s", string(data))
	}

	// 2. With --force: should atomically replace
	forceOverwrite = true
	nonInteractive = true
	initType = "api"
	initURL = "https://example.com"
	defer func() {
		forceOverwrite = false
		nonInteractive = false
		initType = ""
		initURL = ""
	}()

	out.Reset()
	err = RunInitWizard(strings.NewReader(""), &out, configFile)
	if err != nil {
		t.Fatalf("RunInitWizard failed with force: %v", err)
	}

	data, err = os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read overwritten file: %v", err)
	}
	if strings.Contains(string(data), "existing_config: true") {
		t.Errorf("File was not overwritten: %s", string(data))
	}
	if !strings.Contains(string(data), "https://example.com") {
		t.Errorf("Expected new config in file, got: %s", string(data))
	}
}

func TestWriteFileAtomic(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "test.yaml")

	// Initial write
	initialData := []byte("version: 1\n")
	if err := writeFileAtomic(targetPath, initialData, 0600); err != nil {
		t.Fatalf("writeFileAtomic failed on initial write: %v", err)
	}

	fi, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("Failed to stat target file: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("Expected perm 0600, got %o", fi.Mode().Perm())
	}

	// Atomic overwrite should preserve existing permissions (0600)
	newData := []byte("version: 2\n")
	if err := writeFileAtomic(targetPath, newData, 0644); err != nil {
		t.Fatalf("writeFileAtomic failed on overwrite: %v", err)
	}

	readData, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read overwritten file: %v", err)
	}
	if string(readData) != string(newData) {
		t.Errorf("Expected %q, got %q", string(newData), string(readData))
	}

	fi, err = os.Stat(targetPath)
	if err != nil {
		t.Fatalf("Failed to stat target file after overwrite: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("Expected preserved perm 0600, got %o", fi.Mode().Perm())
	}

	// Verify no temporary files were left behind
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read tempDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "test.yaml" {
		t.Errorf("Expected only test.yaml in directory, found: %v", entries)
	}
}
