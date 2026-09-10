package runtime

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// ComposeManager handles automated Docker Compose lifecycle for target applications
type ComposeManager struct {
	composeFile string
	healthURL   string
	timeout     time.Duration
}

// NewComposeManager creates a manager for compose startup and healthchecks
func NewComposeManager(composeFile string, healthURL string, timeout time.Duration) *ComposeManager {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &ComposeManager{
		composeFile: composeFile,
		healthURL:   healthURL,
		timeout:     timeout,
	}
}

// Start launches the containers via docker compose
func (c *ComposeManager) Start(ctx context.Context) error {
	if c.composeFile == "" {
		return nil
	}

	if _, err := os.Stat(c.composeFile); err != nil {
		return fmt.Errorf("compose file '%s' not found: %w", c.composeFile, err)
	}

	// Try 'docker compose', fallback to 'docker-compose'
	var cmd *exec.Cmd
	if _, err := exec.LookPath("docker"); err == nil {
		cmd = exec.CommandContext(ctx, "docker", "compose", "-f", c.composeFile, "up", "-d")
	} else if _, err := exec.LookPath("docker-compose"); err == nil {
		cmd = exec.CommandContext(ctx, "docker-compose", "-f", c.composeFile, "up", "-d")
	} else {
		return fmt.Errorf("neither 'docker compose' nor 'docker-compose' command found in PATH")
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up failed: %w (output: %s)", err, string(out))
	}

	return c.WaitForHealth(ctx)
}

// WaitForHealth polls the application URL until it returns a 2xx or 3xx status
func (c *ComposeManager) WaitForHealth(ctx context.Context) error {
	if c.healthURL == "" {
		return nil
	}

	deadline := time.Now().Add(c.timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.healthURL, nil)
		if err == nil {
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode < 500 {
					return nil
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("application at '%s' did not become healthy within %v", c.healthURL, c.timeout)
}

// Stop shuts down containers via docker compose down
func (c *ComposeManager) Stop(ctx context.Context) error {
	if c.composeFile == "" {
		return nil
	}

	var cmd *exec.Cmd
	if _, err := exec.LookPath("docker"); err == nil {
		cmd = exec.CommandContext(ctx, "docker", "compose", "-f", c.composeFile, "down")
	} else if _, err := exec.LookPath("docker-compose"); err == nil {
		cmd = exec.CommandContext(ctx, "docker-compose", "-f", c.composeFile, "down")
	} else {
		return nil
	}

	_ = cmd.Run()
	return nil
}
