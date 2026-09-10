package sentinel

import (
	"testing"
	"time"
)

func TestSentinel_DetectKneePoint(t *testing.T) {
	s := New()

	t.Run("Linear scaling without knee-point", func(t *testing.T) {
		vus := []int{10, 50, 100, 150, 200}
		p95s := []float64{1.0, 1.4, 1.9, 2.4, 2.9}

		res := s.DetectKneePoint(vus, p95s)
		if res.Detected {
			t.Errorf("Expected no knee point for linear scaling, got detected: %v", res)
		}
	})

	t.Run("Exponential inflection knee-point detected", func(t *testing.T) {
		vus := []int{10, 50, 100, 150}
		// baseline slope: (1.4 - 1.0)/40 = 0.01 ms/VU
		// stage 3 to 4: (40.0 - 2.0)/50 = 0.76 ms/VU (76x increase, delta > 5ms)
		p95s := []float64{1.0, 1.4, 2.0, 40.0}

		res := s.DetectKneePoint(vus, p95s)
		if !res.Detected {
			t.Fatalf("Expected knee point to be detected, but was not")
		}
		if res.InflectionVUs != 150 {
			t.Errorf("Expected InflectionVUs = 150, got %d", res.InflectionVUs)
		}
		if res.InflectionStage != 4 {
			t.Errorf("Expected InflectionStage = 4, got %d", res.InflectionStage)
		}
	})

	t.Run("Insufficient stages", func(t *testing.T) {
		vus := []int{10, 50}
		p95s := []float64{1.0, 2.0}

		res := s.DetectKneePoint(vus, p95s)
		if res.Detected {
			t.Errorf("Expected no detection with fewer than 3 stages")
		}
	})
}

func TestSentinel_StageGuard(t *testing.T) {
	s := New()
	guard := s.StartStage()

	time.Sleep(10 * time.Millisecond)
	report := guard.Finish()

	if report.Telemetry.RunnerMemMB <= 0 {
		t.Errorf("Expected RunnerMemMB > 0, got %f", report.Telemetry.RunnerMemMB)
	}
}
