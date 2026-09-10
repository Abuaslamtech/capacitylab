package load

import (
	"strings"
	"testing"
	"time"

	"github.com/Abuaslamtech/capacitylab/internal/config"
)

func TestK6Adapter_GenerateScript(t *testing.T) {
	cfg := &config.Config{
		Application: config.Application{
			URL: "http://localhost:3000",
		},
		Workload: config.Workload{
			Pacing: config.PacingConfig{
				ThinkTimeMin: 250 * time.Millisecond,
			},
		},
		Scenarios: []config.Scenario{
			{
				Name:   "login_and_browse",
				Weight: 100,
				Flow: []config.ScenarioStep{
					{"get": "/api/v1/health"},
					{
						"post": map[string]any{
							"path": "/api/v1/login",
							"body": map[string]any{
								"user": "admin",
							},
						},
					},
				},
			},
		},
	}

	adapter := NewK6Adapter(cfg)
	script := adapter.GenerateScript(50, 10*time.Second)

	if !strings.Contains(script, "vus: 50") {
		t.Errorf("Expected script to configure 50 vus, got:\n%s", script)
	}
	if !strings.Contains(script, "http.get('http://localhost:3000/api/v1/health')") {
		t.Errorf("Expected script to include health GET request, got:\n%s", script)
	}
	if !strings.Contains(script, "http.post('http://localhost:3000/api/v1/login'") {
		t.Errorf("Expected script to include login POST request, got:\n%s", script)
	}
	if !strings.Contains(script, "sleep(0.25)") {
		t.Errorf("Expected script to include 0.25s think time sleep, got:\n%s", script)
	}
}
