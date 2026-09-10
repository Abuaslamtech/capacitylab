package report

import (
	"fmt"
	"os"
	"strings"
)

// GitHubSummaryParams holds data required to render the GitHub Actions step summary.
type GitHubSummaryParams struct {
	AppName           string
	TargetURL         string
	TargetVUs         int
	MaxObservedVUs    int
	SustainableVUs    int
	PeakRPS           float64
	P95LatencyMs      float64
	PrimaryBottleneck string
	BottleneckFix     string
	Sizing            SizingData
	Stages            []StageData
	IsRegression      bool
	RegressionItem    string
	CapacityDelta     float64
}

// AppendGitHubStepSummary writes a rich GitHub Actions Markdown summary if summaryFile or GITHUB_STEP_SUMMARY is set.
func AppendGitHubStepSummary(summaryFile string, params GitHubSummaryParams) error {
	if summaryFile == "" {
		summaryFile = os.Getenv("GITHUB_STEP_SUMMARY")
	}
	if summaryFile == "" {
		return nil
	}

	var md strings.Builder
	md.WriteString(fmt.Sprintf("## 🚀 CapacityLab Benchmark: %s\n\n", params.AppName))

	if params.IsRegression {
		md.WriteString(fmt.Sprintf("> [!CAUTION]\n> **Capacity Regression Detected:** %s\n\n", params.RegressionItem))
	} else if params.CapacityDelta > 0 {
		md.WriteString(fmt.Sprintf("> [!TIP]\n> **Capacity Expanded:** Sustainable load increased by %+.1f%%\n\n", params.CapacityDelta))
	}

	md.WriteString("| Metric | Value | Status |\n")
	md.WriteString("| :--- | :--- | :--- |\n")
	md.WriteString(fmt.Sprintf("| **Target Workload** | `%d users` | Target |\n", params.TargetVUs))
	md.WriteString(fmt.Sprintf("| **Max Observed Load** | `%d users` | Peak sustained before breach |\n", params.MaxObservedVUs))
	md.WriteString(fmt.Sprintf("| **Recommended Sustainable Capacity** | **`~%d users`** | 30%% Safety Headroom |\n", params.SustainableVUs))
	md.WriteString(fmt.Sprintf("| **Peak Throughput** | `%.0f RPS` | - |\n", params.PeakRPS))
	md.WriteString(fmt.Sprintf("| **p95 Latency at Peak** | `%.1f ms` | - |\n", params.P95LatencyMs))
	if params.PrimaryBottleneck != "" && params.PrimaryBottleneck != "None" {
		md.WriteString(fmt.Sprintf("| **Primary Bottleneck** | `%s` | ⚠️ Constraint |\n", params.PrimaryBottleneck))
	} else {
		md.WriteString("| **Primary Bottleneck** | `None detected` | ✅ Healthy |\n")
	}
	md.WriteString("\n")

	if params.BottleneckFix != "" {
		md.WriteString(fmt.Sprintf("> **Actionable Fix:** %s\n\n", params.BottleneckFix))
	}

	md.WriteString(fmt.Sprintf("### 💡 Recommended Production Sizing (%s)\n", params.Sizing.TierName))
	md.WriteString(fmt.Sprintf("* **API Service:** `%s`, `%s` *(%s)*\n", params.Sizing.APIVCPU, params.Sizing.APIRAM, params.Sizing.APICostEst))
	if params.Sizing.PostgresVCPU != "" {
		md.WriteString(fmt.Sprintf("* **PostgreSQL:** `%s`, `%s` *(%s - %s)*\n", params.Sizing.PostgresVCPU, params.Sizing.PostgresRAM, params.Sizing.PostgresCostEst, params.Sizing.PostgresAdvice))
	}
	if params.Sizing.RedisVCPU != "" {
		md.WriteString(fmt.Sprintf("* **Redis:** `%s`, `%s` *(%s)*\n", params.Sizing.RedisVCPU, params.Sizing.RedisRAM, params.Sizing.RedisCostEst))
	}
	if params.Sizing.TotalCostEst != "" {
		md.WriteString(fmt.Sprintf("* **Estimated Total Cloud Spend:** **`%s`**\n", params.Sizing.TotalCostEst))
	}
	md.WriteString("\n")

	if len(params.Stages) > 0 {
		md.WriteString("<details><summary><b>Stage Telemetry Breakdown</b> (Click to expand)</summary>\n\n")
		md.WriteString("| Stage | VUs | RPS | p50 Latency | p95 Latency | API CPU | RAM | Error % |\n")
		md.WriteString("| :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |\n")
		for _, s := range params.Stages {
			md.WriteString(fmt.Sprintf("| #%d | %d | %.0f | %.1fms | %.1fms | %.1f%% | %.1fMB | %.1f%% |\n",
				s.StageNum, s.VUs, s.RPS, s.P50Ms, s.P95Ms, s.CPUPercent, s.MemoryMB, s.ErrorPercent))
		}
		md.WriteString("\n</details>\n\n")
	}

	f, err := os.OpenFile(summaryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open summary file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(md.String()); err != nil {
		return fmt.Errorf("failed to write summary: %w", err)
	}

	return nil
}
