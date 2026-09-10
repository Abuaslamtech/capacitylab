package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendGitHubStepSummary(t *testing.T) {
	tempDir := t.TempDir()
	summaryFile := filepath.Join(tempDir, "step_summary.md")

	params := GitHubSummaryParams{
		AppName:           "test-service",
		TargetURL:         "http://localhost:3000",
		TargetVUs:         500,
		MaxObservedVUs:    400,
		SustainableVUs:    280,
		PeakRPS:           850,
		P95LatencyMs:      45.5,
		PrimaryBottleneck: "API CPU Saturation",
		BottleneckFix:     "Increase container CPU",
		Sizing: SizingData{
			TierName:     "Small Tier",
			APIVCPU:      "1.0 vCPU",
			APIRAM:       "1 GB RAM",
			APICostEst:   "$6/mo",
			TotalCostEst: "$26/mo",
		},
		Stages: []StageData{
			{
				StageNum:     1,
				VUs:          50,
				RPS:          100,
				P50Ms:        10,
				P95Ms:        20,
				CPUPercent:   30,
				MemoryMB:     200,
				ErrorPercent: 0,
			},
		},
		IsRegression:  false,
		CapacityDelta: 15.0,
	}

	err := AppendGitHubStepSummary(summaryFile, params)
	if err != nil {
		t.Fatalf("AppendGitHubStepSummary failed: %v", err)
	}

	content, err := os.ReadFile(summaryFile)
	if err != nil {
		t.Fatalf("Failed to read summary file: %v", err)
	}

	contentStr := string(content)
	expectedSubstrings := []string{
		"CapacityLab Benchmark: test-service",
		"Capacity Expanded:",
		"~280 users",
		"Small Tier",
		"$26/mo",
		"API CPU Saturation",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(contentStr, sub) {
			t.Errorf("Expected summary to contain %q, but got:\n%s", sub, contentStr)
		}
	}
}
