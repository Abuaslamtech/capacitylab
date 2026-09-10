package runtime

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestComposeManager(t *testing.T) {
	t.Run("Non-existent compose file returns error on start", func(t *testing.T) {
		tempDir := t.TempDir()
		missingFile := filepath.Join(tempDir, "missing-docker-compose.yml")

		cm := NewComposeManager(missingFile, "http://localhost:9999", 2*time.Second)
		err := cm.Start(context.Background())
		if err == nil {
			t.Errorf("Expected error starting missing compose file, got nil")
		}
	})

	t.Run("Empty compose file is no-op", func(t *testing.T) {
		cm := NewComposeManager("", "", 0)
		if err := cm.Start(context.Background()); err != nil {
			t.Errorf("Expected nil error for empty compose file, got %v", err)
		}
		if err := cm.Stop(context.Background()); err != nil {
			t.Errorf("Expected nil error for empty compose file, got %v", err)
		}
	})
}
