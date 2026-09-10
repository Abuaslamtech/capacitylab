package load

import (
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

// StageRunner defines the standard contract for workload engines (Native Go worker pool or Headless k6).
type StageRunner interface {
	RunStage(ctx context.Context, vus int, duration time.Duration) (*StageMetrics, error)
}

var _ StageRunner = (*Runner)(nil)

// Runner drives concurrent virtual user traffic.
type Runner struct {
	cfg        *config.Config
	transport  *http.Transport
	httpClient *http.Client
	selector   *ScenarioSelector
	baseURL    string
}

const maxLatencySamples = 50000

type latencyCollector struct {
	mu      sync.Mutex
	samples []float64
	count   int64
}

func (c *latencyCollector) addBatch(batch []float64) {
	if len(batch) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, lat := range batch {
		c.count++
		if len(c.samples) < maxLatencySamples {
			c.samples = append(c.samples, lat)
		} else {
			// Algorithm R reservoir sampling to bound memory during long endurance/soak runs
			idx := rand.Int63n(c.count)
			if idx < int64(maxLatencySamples) {
				c.samples[idx] = lat
			}
		}
	}
}

// NewRunner creates a load runner with connection pooling to prevent socket exhaustion (Trap 4)
func NewRunner(cfg *config.Config) *Runner {
	insecure := false
	if cfg != nil && cfg.Application.Insecure {
		insecure = true
	}

	transport := &http.Transport{
		MaxIdleConns:        2000,
		MaxIdleConnsPerHost: 1000,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: insecure},
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

// RunStage executes a load stage with N virtual users for a given duration.
func (r *Runner) RunStage(ctx context.Context, vus int, duration time.Duration) (*StageMetrics, error) {
	var (
		totalReqs        atomic.Int64
		successes        atomic.Int64
		errorCount       atomic.Int64
		collector        latencyCollector
		stageCtx, cancel = context.WithTimeout(ctx, duration)
	)
	defer cancel()

	var wg sync.WaitGroup

	// Launch N virtual users as lightweight goroutines
	for i := 0; i < vus; i++ {
		wg.Add(1)
		go func() {
			var localLatencies []float64
			flush := func() {
				if len(localLatencies) > 0 {
					collector.addBatch(localLatencies)
					localLatencies = localLatencies[:0]
				}
			}
			defer func() {
				flush()
				wg.Done()
			}()

			for {
				select {
				case <-stageCtx.Done():
					return
				default:
					// Execute a random weighted scenario
					start := time.Now()
					err := r.executeScenario(stageCtx)
					latency := float64(time.Since(start).Microseconds()) / 1000.0 // ms

					totalReqs.Add(1)
					localLatencies = append(localLatencies, latency)
					if len(localLatencies) >= 500 {
						flush()
					}

					if err != nil {
						// Only count real network or server errors, not the stage timer expiring
						if stageCtx.Err() == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
							errorCount.Add(1)
						}
					} else {
						successes.Add(1)
					}

					// Pacing / think time (simulate real human delays using configured thresholds)
					minThink := r.cfg.Workload.Pacing.ThinkTimeMin
					maxThink := r.cfg.Workload.Pacing.ThinkTimeMax
					if minThink <= 0 {
						minThink = 20 * time.Millisecond
					}
					if maxThink <= minThink {
						time.Sleep(minThink)
					} else {
						delta := maxThink - minThink
						time.Sleep(minThink + time.Duration(rand.Int63n(int64(delta))))
					}
				}
			}
		}()
	}

	// Wait for the duration to elapse and all virtual users to spin down
	wg.Wait()

	// Compute statistics
	seconds := duration.Seconds()
	tot := totalReqs.Load()
	errs := errorCount.Load()
	succ := successes.Load()

	var rps float64
	if seconds > 0 {
		rps = float64(tot) / seconds
	}

	var errPercent float64
	if tot > 0 {
		errPercent = (float64(errs) / float64(tot)) * 100.0
	}

	p50, p90, p95, p99 := calculatePercentiles(collector.samples)

	return &StageMetrics{
		VUs:           vus,
		Duration:      duration,
		TotalRequests: tot,
		SuccessCount:  succ,
		ErrorCount:    errs,
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
	session := make(map[string]string)

	// Execute scenario flow steps sequentially with dynamic session context
	for _, step := range scenario.Steps {
		stepErr := func() error {
			method, path, headers, bodyReader := step.ResolveExecution(session)

			fullURL := r.baseURL
			if path != "" {
				if strings.HasPrefix(path, "/") {
					fullURL += path
				} else {
					fullURL += "/" + path
				}
			}

			req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
			if err != nil {
				return err
			}

			for k, v := range headers {
				req.Header.Set(k, v)
			}

			resp, err := r.httpClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			// Read response body up to 1MB to prevent OOM while allowing JSON token extraction
			bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))

			if resp.StatusCode >= 500 {
				return fmt.Errorf("step %s %s error: %d", step.Method, step.Path, resp.StatusCode)
			}

			// Extract response variables for next steps
			ExtractResponseContext(resp.StatusCode, resp.Header, bodyBytes, session)
			return nil
		}()

		if stepErr != nil {
			return stepErr
		}
	}

	return nil
}

func calculatePercentiles(latencies []float64) (float64, float64, float64, float64) {
	n := len(latencies)
	if n == 0 {
		return 0, 0, 0, 0
	}

	sort.Float64s(latencies)

	pct := func(p float64) float64 {
		idx := int(float64(n) * p)
		if idx >= n {
			idx = n - 1
		}
		return latencies[idx]
	}

	return pct(0.50), pct(0.90), pct(0.95), pct(0.99)
}
