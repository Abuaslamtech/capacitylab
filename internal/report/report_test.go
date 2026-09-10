package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateHTML_OfflineZeroDependency(t *testing.T) {
	tempDir := t.TempDir()
	outHTML := filepath.Join(tempDir, "test-report.html")

	sizing := SizingData{
		TierName: "Small Tier",
		APIVCPU:  "1.0 vCPU",
		APIRAM:   "1 GB RAM",
	}

	stages := []StageData{
		{StageNum: 1, VUs: 10, RPS: 150, P50Ms: 12, P95Ms: 25, CPUPercent: 30, MemoryMB: 200},
	}

	err := GenerateHTML(
		outHTML,
		"TestApp",
		"http://localhost:3000",
		"test-container",
		"None",
		"Healthy",
		nil,
		nil,
		100,
		100,
		70,
		stages,
		DatabaseTelemetry{},
		nil,
		sizing,
	)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	content, err := os.ReadFile(outHTML)
	if err != nil {
		t.Fatalf("Failed to read generated HTML: %v", err)
	}

	htmlStr := string(content)

	// 1. Verify NO external CDN script tag exists
	if strings.Contains(htmlStr, "https://cdn.jsdelivr.net") {
		t.Errorf("Report should NOT contain external CDN links, but found jsdelivr")
	}

	// 2. Verify embedded Chart.js is present
	if !strings.Contains(htmlStr, "Chart.js") && !strings.Contains(htmlStr, "chart.umd") && !strings.Contains(htmlStr, "Chart") {
		t.Errorf("Report should embed Chart.js directly in the script tag")
	}

	// 3. Verify dynamic sizing recommendation is rendered
	if !strings.Contains(htmlStr, "Small Tier") {
		t.Errorf("Expected 'Small Tier' in rendered HTML, not found")
	}
	if !strings.Contains(htmlStr, "1.0 vCPU") {
		t.Errorf("Expected '1.0 vCPU' in rendered HTML, not found")
	}
}

func TestGenerateDashboardHTML_OfflineZeroDependency(t *testing.T) {
	tempDir := t.TempDir()
	outHTML := filepath.Join(tempDir, "test-dashboard.html")

	summaries := []DashboardRunSummary{
		{ID: "run-1", Date: "Sep 10 12:00", AppName: "TestApp", SustainableVUs: 70, PeakRPS: 200},
	}

	err := GenerateDashboardHTML(outHTML, summaries)
	if err != nil {
		t.Fatalf("GenerateDashboardHTML failed: %v", err)
	}

	content, err := os.ReadFile(outHTML)
	if err != nil {
		t.Fatalf("Failed to read generated dashboard HTML: %v", err)
	}

	htmlStr := string(content)

	if strings.Contains(htmlStr, "https://cdn.jsdelivr.net") {
		t.Errorf("Dashboard should NOT contain external CDN links, but found jsdelivr")
	}
}

func TestGenerateHTML_RemoteTargetAdaptive(t *testing.T) {
	tempDir := t.TempDir()
	outHTML := filepath.Join(tempDir, "remote-report.html")

	sizing := SizingData{
		TierName:        "Small Tier",
		APIVCPU:         "1.0 vCPU",
		APIRAM:          "1 GB RAM",
		IsRemoteTarget:  true,
		SizingRationale: "Estimated from traffic throughput capacity ceiling",
		CostBudgetVPS:   "$4 - $6 / mo",
		CostPaaS:        "$7 - $15 / mo",
		CostHyperscaler: "$20 - $45 / mo",
	}

	stages := []StageData{
		{StageNum: 1, VUs: 10, RPS: 150, P50Ms: 12, P95Ms: 25, CPUPercent: 0, MemoryMB: 0},
	}

	err := GenerateHTML(
		outHTML,
		"RemoteAPI",
		"https://api.example.com",
		"",
		"None",
		"Healthy",
		nil,
		nil,
		100,
		100,
		70,
		stages,
		DatabaseTelemetry{},
		nil,
		sizing,
	)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	content, err := os.ReadFile(outHTML)
	if err != nil {
		t.Fatalf("Failed to read generated HTML: %v", err)
	}

	htmlStr := string(content)

	if !strings.Contains(htmlStr, "Target: Hosted Cloud API") {
		t.Errorf("Expected 'Target: Hosted Cloud API' badge in report")
	}
	if !strings.Contains(htmlStr, "p95 Latency Progression (ms)") {
		t.Errorf("Expected 'p95 Latency Progression (ms)' chart title without CPU")
	}
	if !strings.Contains(htmlStr, "N/A (Remote)") {
		t.Errorf("Expected 'N/A (Remote)' in telemetry table for remote target")
	}
	if !strings.Contains(htmlStr, "$4 - $6 / mo") || !strings.Contains(htmlStr, "$7 - $15 / mo") || !strings.Contains(htmlStr, "$20 - $45 / mo") {
		t.Errorf("Expected multi-cloud cost breakdown values in report")
	}
}
