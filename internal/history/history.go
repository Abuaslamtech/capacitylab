package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Abuaslamtech/capacitylab/internal/report"
)

// RunRecord stores full benchmark results for regression tracking and trends
type RunRecord struct {
	ID                string             `json:"id"`
	Timestamp         time.Time          `json:"timestamp"`
	AppName           string             `json:"app_name"`
	TargetURL         string             `json:"target_url"`
	TargetVUs         int                `json:"target_vus"`
	MaxObservedVUs    int                `json:"max_observed_vus"`
	SustainableVUs    int                `json:"sustainable_vus"`
	PeakRPS           float64            `json:"peak_rps"`
	P95LatencyMs      float64            `json:"p95_latency_ms"`
	PrimaryBottleneck string             `json:"primary_bottleneck"`
	BottleneckFix     string             `json:"bottleneck_fix"`
	Stages            []report.StageData `json:"stages,omitempty"`
}

// ComparisonResult represents the performance and capacity delta between two runs
type ComparisonResult struct {
	BaseID         string
	CurrentID      string
	CapacityDelta  float64 // Percentage change in sustainable capacity
	RPSDelta       float64 // Percentage change in peak RPS
	LatencyDelta   float64 // Percentage change in p95 latency (negative is better)
	IsRegression   bool
	RegressionItem string
}

// GetHistoryDir returns the path to ~/.capacitylab/history/
func GetHistoryDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".capacitylab", "history")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// SaveRun writes a benchmark run record to disk
func SaveRun(rec RunRecord) (string, error) {
	dir, err := GetHistoryDir()
	if err != nil {
		return "", err
	}

	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now()
	}

	sanitizedApp := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '_'
	}, rec.AppName)

	if rec.ID == "" {
		rec.ID = fmt.Sprintf("%s_%s", rec.Timestamp.Format("20060102-150405"), sanitizedApp)
	}

	filename := filepath.Join(dir, rec.ID+".json")
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to encode run record: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return "", fmt.Errorf("failed to save run record: %w", err)
	}

	return filename, nil
}

// ListRuns loads all saved runs sorted by timestamp descending
func ListRuns() ([]RunRecord, error) {
	dir, err := GetHistoryDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var runs []RunRecord
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			var rec RunRecord
			if err := json.Unmarshal(data, &rec); err == nil {
				runs = append(runs, rec)
			}
		}
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].Timestamp.After(runs[j].Timestamp)
	})

	return runs, nil
}

// GetLatestTwoRuns finds the latest run and the previous run for a given application
func GetLatestTwoRuns(appName string) (*RunRecord, *RunRecord, error) {
	runs, err := ListRuns()
	if err != nil {
		return nil, nil, err
	}

	var appRuns []RunRecord
	for _, r := range runs {
		if appName == "" || r.AppName == appName {
			appRuns = append(appRuns, r)
		}
	}

	if len(appRuns) == 0 {
		return nil, nil, fmt.Errorf("no benchmark history found")
	}
	if len(appRuns) == 1 {
		return &appRuns[0], nil, nil
	}

	return &appRuns[0], &appRuns[1], nil
}

// Compare computes the delta between a baseline run and a current run
func Compare(base, current RunRecord) ComparisonResult {
	res := ComparisonResult{
		BaseID:    base.ID,
		CurrentID: current.ID,
	}

	if base.SustainableVUs > 0 {
		res.CapacityDelta = (float64(current.SustainableVUs-base.SustainableVUs) / float64(base.SustainableVUs)) * 100.0
	}
	if base.PeakRPS > 0 {
		res.RPSDelta = ((current.PeakRPS - base.PeakRPS) / base.PeakRPS) * 100.0
	}
	if base.P95LatencyMs > 0 {
		res.LatencyDelta = ((current.P95LatencyMs - base.P95LatencyMs) / base.P95LatencyMs) * 100.0
	}

	// Flag regressions: capacity dropped >5% or latency worsened >15%
	if res.CapacityDelta < -5.0 {
		res.IsRegression = true
		res.RegressionItem = fmt.Sprintf("Sustainable capacity dropped by %.1f%% (%d ➔ %d users)",
			-res.CapacityDelta, base.SustainableVUs, current.SustainableVUs)
	} else if res.LatencyDelta > 15.0 {
		res.IsRegression = true
		res.RegressionItem = fmt.Sprintf("p95 latency degraded by +%.1f%% (%.1fms ➔ %.1fms)",
			res.LatencyDelta, base.P95LatencyMs, current.P95LatencyMs)
	}

	return res
}
