package analyzer

import (
	"testing"
)

func TestComputeRecommendation(t *testing.T) {
	t.Run("Micro tier recommendation", func(t *testing.T) {
		rec := ComputeRecommendation(100, 150, 25.0, 200.0, 5, 50, 50.0, 0.70)
		if rec.SustainableVUs != 105 {
			t.Errorf("Expected sustainable capacity 105, got %d", rec.SustainableVUs)
		}
		if rec.SafetyHeadroomPct != 30.0 {
			t.Errorf("Expected 30%% headroom, got %.1f%%", rec.SafetyHeadroomPct)
		}
		if rec.APITierName != "Micro Tier" {
			t.Errorf("Expected Micro Tier, got %s", rec.APITierName)
		}
		if rec.APIVCPU != "0.5 vCPU" {
			t.Errorf("Expected 0.5 vCPU, got %s", rec.APIVCPU)
		}
		if rec.TotalCostEst == "" || rec.APICostEst == "" {
			t.Errorf("Expected non-empty cost estimations, got total=%q, api=%q", rec.TotalCostEst, rec.APICostEst)
		}
	})

	t.Run("Custom safety factor", func(t *testing.T) {
		rec := ComputeRecommendation(100, 200, 25.0, 200.0, 5, 50, 50.0, 0.80)
		if rec.SustainableVUs != 160 {
			t.Errorf("Expected sustainable capacity 160 (0.80 * 200), got %d", rec.SustainableVUs)
		}
		if rec.SafetyHeadroomPct != 20.0 {
			t.Errorf("Expected 20%% headroom, got %.1f%%", rec.SafetyHeadroomPct)
		}
	})

	t.Run("PostgreSQL pool expansion recommendation", func(t *testing.T) {
		rec := ComputeRecommendation(500, 700, 85.0, 800.0, 45, 100, 50.0, 0.70)
		if rec.APITierName != "Compute-Optimized Tier" {
			t.Errorf("Expected Compute-Optimized Tier, got %s", rec.APITierName)
		}
		if rec.PostgresRAM != "2 GB RAM" {
			t.Errorf("Expected 2 GB RAM, got %s", rec.PostgresRAM)
		}
		if rec.PostgresAdvice == "" {
			t.Errorf("Expected PgBouncer advice, got empty string")
		}
	})

	t.Run("Overprovisioning detection", func(t *testing.T) {
		rec := ComputeRecommendation(200, 1000, 50.0, 400.0, 10, 50, 50.0, 0.70)
		if !rec.Overprovisioned {
			t.Errorf("Expected Overprovisioned to be true when capacity is > 2x target")
		}
	})

	t.Run("Remote target detection and multi-cloud pricing", func(t *testing.T) {
		rec := ComputeRecommendation(50, 30, 0.0, 0.0, 0, 0, 0.0, 0.70)
		if !rec.IsRemoteTarget {
			t.Errorf("Expected IsRemoteTarget to be true when CPU and RAM are 0")
		}
		if rec.SizingRationale == "" {
			t.Errorf("Expected SizingRationale to be populated for remote target")
		}
		if rec.Costs.BudgetVPS == "" || rec.Costs.PaaS == "" || rec.Costs.Hyperscaler == "" {
			t.Errorf("Expected multi-cloud costs populated: %+v", rec.Costs)
		}
	})
}
