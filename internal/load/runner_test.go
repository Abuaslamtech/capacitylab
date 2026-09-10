package load

import (
	"math/rand"
	"testing"

	"github.com/Abuaslamtech/capacitylab/internal/config"
)

func TestNewRunner_TLSConfig(t *testing.T) {
	t.Run("Default strict TLS verification", func(t *testing.T) {
		cfg := &config.Config{
			Application: config.Application{
				Name: "prod-api",
				URL:  "https://api.example.com",
			},
		}
		runner := NewRunner(cfg)
		if runner.transport.TLSClientConfig.InsecureSkipVerify {
			t.Errorf("Expected InsecureSkipVerify to be false by default for production security")
		}
	})

	t.Run("Insecure TLS verification when enabled", func(t *testing.T) {
		cfg := &config.Config{
			Application: config.Application{
				Name:     "local-dev",
				URL:      "https://localhost:8443",
				Insecure: true,
			},
		}
		runner := NewRunner(cfg)
		if !runner.transport.TLSClientConfig.InsecureSkipVerify {
			t.Errorf("Expected InsecureSkipVerify to be true when explicitly configured")
		}
	})
}

func TestLatencyCollector_ReservoirSamplingBoundsMemory(t *testing.T) {
	collector := latencyCollector{}

	// Simulate high throughput with 120,000 requests (exceeding maxLatencySamples of 50,000)
	batchSize := 500
	batches := 240 // 240 * 500 = 120,000 samples

	for i := 0; i < batches; i++ {
		batch := make([]float64, batchSize)
		for j := 0; j < batchSize; j++ {
			batch[j] = float64(rand.Intn(100) + 10)
		}
		collector.addBatch(batch)
	}

	if len(collector.samples) > maxLatencySamples {
		t.Errorf("Expected samples slice to be bounded at %d, but got %d", maxLatencySamples, len(collector.samples))
	}
	if collector.count != 120000 {
		t.Errorf("Expected collector count to be 120000, got %d", collector.count)
	}

	p50, p90, p95, p99 := calculatePercentiles(collector.samples)
	if p50 <= 0 || p90 <= 0 || p95 <= 0 || p99 <= 0 {
		t.Errorf("Percentiles should be positive numbers, got p50=%f, p95=%f", p50, p95)
	}
}
