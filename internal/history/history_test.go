package history

import (
	"testing"
	"time"
)

func TestHistory_Compare(t *testing.T) {
	base := RunRecord{
		ID:             "run-1",
		Timestamp:      time.Now().Add(-1 * time.Hour),
		AppName:        "test-app",
		SustainableVUs: 300,
		PeakRPS:        10000,
		P95LatencyMs:   2.0,
	}

	t.Run("Performance improvement", func(t *testing.T) {
		current := RunRecord{
			ID:             "run-2",
			Timestamp:      time.Now(),
			AppName:        "test-app",
			SustainableVUs: 450,
			PeakRPS:        15000,
			P95LatencyMs:   1.5,
		}

		res := Compare(base, current)
		if res.IsRegression {
			t.Errorf("Expected improvement, got regression: %s", res.RegressionItem)
		}
		if res.CapacityDelta != 50.0 {
			t.Errorf("Expected +50%% capacity delta, got %.1f%%", res.CapacityDelta)
		}
		if res.RPSDelta != 50.0 {
			t.Errorf("Expected +50%% RPS delta, got %.1f%%", res.RPSDelta)
		}
		if res.LatencyDelta != -25.0 {
			t.Errorf("Expected -25%% latency delta, got %.1f%%", res.LatencyDelta)
		}
	})

	t.Run("Capacity regression detected", func(t *testing.T) {
		current := RunRecord{
			ID:             "run-3",
			Timestamp:      time.Now(),
			AppName:        "test-app",
			SustainableVUs: 250, // -16.7%
			PeakRPS:        8000,
			P95LatencyMs:   2.2,
		}

		res := Compare(base, current)
		if !res.IsRegression {
			t.Errorf("Expected regression to be flagged for capacity drop")
		}
	})

	t.Run("Latency regression detected", func(t *testing.T) {
		current := RunRecord{
			ID:             "run-4",
			Timestamp:      time.Now(),
			AppName:        "test-app",
			SustainableVUs: 300,
			PeakRPS:        10000,
			P95LatencyMs:   2.6, // +30%
		}

		res := Compare(base, current)
		if !res.IsRegression {
			t.Errorf("Expected regression to be flagged for latency degradation")
		}
	})
}
