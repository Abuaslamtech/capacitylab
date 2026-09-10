package sentinel

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// MaxRunnerCPUThresholdPercent is the maximum CPU overhead allowed for the runner process
	// before client-side CPU contention risks skewing benchmark measurements (Trap 1).
	MaxRunnerCPUThresholdPercent = 35.0

	// MaxTimeWaitSocketsThreshold warns when loopback TIME_WAIT sockets reach hazardous levels (Trap 2).
	MaxTimeWaitSocketsThreshold = 10000
)

// HostTelemetry captures host and process health during a benchmark stage
type HostTelemetry struct {
	RunnerCPUPercent float64
	RunnerMemMB      float64
	TimeWaitSockets  int
	InUseSockets     int
}

// SentinelReport summarizes any host or environment contamination detected
type SentinelReport struct {
	Telemetry  HostTelemetry
	Warnings   []string
	HasWarning bool
}

// StageGuard monitors resource usage during a specific stage
type StageGuard struct {
	startWall time.Time
	startCPU  time.Duration
}

// Sentinel oversees local benchmarking execution to guarantee clean measurement conditions
type Sentinel struct{}

// New initializes a new Local Benchmarking Sentinel
func New() *Sentinel {
	return &Sentinel{}
}

// StartStage begins tracking host and process metrics for an individual stage
func (s *Sentinel) StartStage() *StageGuard {
	return &StageGuard{
		startWall: time.Now(),
		startCPU:  getProcessCPUTime(),
	}
}

// Finish completes stage tracking and inspects for host contamination
func (g *StageGuard) Finish() SentinelReport {
	wallElapsed := time.Since(g.startWall)
	cpuElapsed := getProcessCPUTime() - g.startCPU

	var runnerCPU float64
	numCores := float64(runtime.NumCPU())
	if wallElapsed > 0 && numCores > 0 {
		runnerCPU = (float64(cpuElapsed) / float64(wallElapsed)) / numCores * 100.0
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	runnerMem := float64(memStats.Alloc) / (1024 * 1024)

	twSockets, inUseSockets := readSocketStats()

	report := SentinelReport{
		Telemetry: HostTelemetry{
			RunnerCPUPercent: runnerCPU,
			RunnerMemMB:      runnerMem,
			TimeWaitSockets:  twSockets,
			InUseSockets:     inUseSockets,
		},
	}

	// Rule 1: Check runner CPU overhead (Trap 1)
	if runnerCPU > MaxRunnerCPUThresholdPercent {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("Runner CPU overhead reached %.1f%% (>%.0f%% limit). Client-side scheduling delays may skew latency measurements.",
				runnerCPU, MaxRunnerCPUThresholdPercent))
	}

	// Rule 2: Check TIME_WAIT sockets (Trap 2)
	if twSockets > MaxTimeWaitSocketsThreshold {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("Elevated TIME_WAIT sockets detected (%d > %d). Imminent ephemeral port exhaustion risk on loopback.",
				twSockets, MaxTimeWaitSocketsThreshold))
	}

	report.HasWarning = len(report.Warnings) > 0
	return report
}

// KneePointInfo details early latency queueing inflection before hard saturation
type KneePointInfo struct {
	Detected        bool
	InflectionStage int
	InflectionVUs   int
	BaselineSlope   float64
	CurrentSlope    float64
	Message         string
}

// DetectKneePoint identifies the mathematical inflection point where latency begins exponential rise
func (s *Sentinel) DetectKneePoint(vus []int, p95s []float64) KneePointInfo {
	n := len(vus)
	if n < 3 || len(p95s) != n {
		return KneePointInfo{Detected: false}
	}

	// Calculate baseline slope from stage 1 to stage 2
	deltaVU1 := float64(vus[1] - vus[0])
	if deltaVU1 <= 0 {
		return KneePointInfo{Detected: false}
	}
	baselineSlope := (p95s[1] - p95s[0]) / deltaVU1
	if baselineSlope < 0.001 {
		baselineSlope = 0.001 // minimum baseline to prevent division by zero
	}

	// Check subsequent stages for slope acceleration
	for i := 2; i < n; i++ {
		deltaVU := float64(vus[i] - vus[i-1])
		if deltaVU <= 0 {
			continue
		}
		currentSlope := (p95s[i] - p95s[i-1]) / deltaVU
		deltaP95 := p95s[i] - p95s[i-1]

		// Inflection criteria: slope surges by 3.0x AND latency delta exceeds 5.0ms
		if currentSlope >= (baselineSlope * 3.0) && deltaP95 >= 5.0 {
			return KneePointInfo{
				Detected:        true,
				InflectionStage: i + 1,
				InflectionVUs:   vus[i],
				BaselineSlope:   baselineSlope,
				CurrentSlope:    currentSlope,
				Message: fmt.Sprintf("Latency slope increased by %.1fx (%.2f ms/VU vs baseline %.2f ms/VU) at %d VUs.",
					currentSlope/baselineSlope, currentSlope, baselineSlope, vus[i]),
			}
		}
	}

	return KneePointInfo{Detected: false}
}

// getProcessCPUTime returns total user + system CPU duration consumed by this process
func getProcessCPUTime() time.Duration {
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		return 0
	}
	user := time.Duration(rusage.Utime.Sec)*time.Second + time.Duration(rusage.Utime.Usec)*time.Microsecond
	sys := time.Duration(rusage.Stime.Sec)*time.Second + time.Duration(rusage.Stime.Usec)*time.Microsecond
	return user + sys
}

// readSocketStats extracts active and TIME_WAIT socket counts from /proc/net/sockstat
func readSocketStats() (timeWait int, inUse int) {
	file, err := os.Open("/proc/net/sockstat")
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "TCP:") {
			fields := strings.Fields(line)
			for i := 0; i < len(fields)-1; i++ {
				if fields[i] == "inuse" {
					inUse, _ = strconv.Atoi(fields[i+1])
				}
				if fields[i] == "tw" {
					timeWait, _ = strconv.Atoi(fields[i+1])
				}
			}
			break
		}
	}
	return timeWait, inUse
}
