package load

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Abuaslamtech/capacitylab/internal/config"
)

// StageMetrics records performance telemetry for one load stage
type StageMetrics struct {
	VUs           int
	Duration      time.Duration
	TotalRequests int64
	SuccessCount  int64
	ErrorCount    int64
	RPS           float64
	P50Ms         float64
	P90Ms         float64
	P95Ms         float64
	P99Ms         float64
	ErrorPercent  float64
}

// Runner drives concurrent virtual user traffic
type Runner struct {
	cfg        *config.Config
	transport  *http.Transport
	httpClient *http.Client
	selector   *ScenarioSelector
	baseURL    string
}

// NewRunner creates a load runner with connection pooling to prevent socket exhaustion (Trap 4)
func NewRunner(cfg *config.Config) *Runner {
	transport := &http.Transport{
		MaxIdleConns:        2000,
		MaxIdleConnsPerHost: 1000,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression: false,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	selector, _ := NewScenarioSelector(cfg.Scenarios)
	baseURL := strings.TrimRight(cfg.Application.URL, "/")

	return &Runner{
		cfg:        cfg,
		transport:  transport,
		httpClient: client,
		selector:   selector,
		baseURL:    baseURL,
	}
}

// Reset clears idle connections from the pool
func (r *Runner) Reset() {
	if r.transport != nil {
		r.transport.CloseIdleConnections()
	}
}

// RunStage executes a load stage with N virtual users for a given duration
func (r *Runner) RunStage(ctx context.Context, vus int, duration time.Duration) (*StageMetrics, error) {
	var (
		totalReqs        int64
		successes        int64
		errorCount       int64
		mu               sync.Mutex
		latencies        []float64
		stageCtx, cancel = context.WithTimeout(ctx, duration)
	)
	defer cancel()

	var wg sync.WaitGroup

	// Launch N virtual users as lightweight goroutines
	for i := 0; i < vus; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case <-stageCtx.Done():
					return
				default:
					// Execute a random weighted scenario
					start := time.Now()
					err := r.executeScenario(stageCtx)
					latency := float64(time.Since(start).Microseconds()) / 1000.0 // ms

					atomic.AddInt64(&totalReqs, 1)

					mu.Lock()
					latencies = append(latencies, latency)
					mu.Unlock()

					if err != nil {
						// Only count real network or server errors, not the stage timer expiring
						if stageCtx.Err() == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
							atomic.AddInt64(&errorCount, 1)
						}
					} else {
						atomic.AddInt64(&successes, 1)
					}

					// Pacing / think time (simulate real human delays)
					time.Sleep(time.Duration(20+rand.Intn(30)) * time.Millisecond)
				}
			}
		}()
	}

	// Wait for the duration to elapse and all virtual users to spin down
	wg.Wait()

	// Compute statistics
	seconds := duration.Seconds()
	rps := float64(totalReqs) / seconds

	var errPercent float64
	if totalReqs > 0 {
		errPercent = (float64(errorCount) / float64(totalReqs)) * 100.0
	}

	p50, p90, p95, p99 := calculatePercentiles(latencies)

	return &StageMetrics{
		VUs:           vus,
		Duration:      duration,
		TotalRequests: totalReqs,
		SuccessCount:  successes,
		ErrorCount:    errorCount,
		RPS:           rps,
		P50Ms:         p50,
		P90Ms:         p90,
		P95Ms:         p95,
		P99Ms:         p99,
		ErrorPercent:  errPercent,
	}, nil
}

func (r *Runner) executeScenario(ctx context.Context) error {
	if r.selector == nil || len(r.selector.scenarios) == 0 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL, nil)
		if err != nil {
			return err
		}
		resp, err := r.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode >= 500 {
			return fmt.Errorf("server error: %d", resp.StatusCode)
		}
		return nil
	}

	// Pick scenario based on weight distribution
	scenario := r.selector.Pick()

	// Execute scenario flow steps sequentially
	for _, step := range scenario.Steps {
		fullURL := r.baseURL
		if step.Path != "" {
			if strings.HasPrefix(step.Path, "/") {
				fullURL += step.Path
			} else {
				fullURL += "/" + step.Path
			}
		}

		var bodyReader io.Reader
		if len(step.Body) > 0 {
			bodyReader = bytes.NewReader(step.Body)
		}

		req, err := http.NewRequestWithContext(ctx, step.Method, fullURL, bodyReader)
		if err != nil {
			return err
		}

		for k, v := range step.Headers {
			req.Header.Set(k, v)
		}

		resp, err := r.httpClient.Do(req)
		if err != nil {
			return err
		}

		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 500 {
			return fmt.Errorf("step %s %s error: %d", step.Method, step.Path, resp.StatusCode)
		}
	}

	return nil
}

func calculatePercentiles(latencies []float64) (p50, p90, p95, p99 float64) {
	if len(latencies) == 0 {
		return 0, 0, 0, 0
	}

	sort.Float64s(latencies)
	n := len(latencies)

	p50 = latencies[int(float64(n)*0.50)]
	p90 = latencies[int(float64(n)*0.90)]
	p95 = latencies[int(float64(n)*0.95)]
	p99 = latencies[int(float64(n)*0.99)]
	return
}
