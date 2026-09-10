package report

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

//go:embed assets/chart.min.js
var embeddedChartJS string

// StageData combines load metrics with container resource telemetry
type StageData struct {
	StageNum     int
	VUs          int
	RPS          float64
	P50Ms        float64
	P95Ms        float64
	P99Ms        float64
	CPUPercent   float64
	MemoryMB     float64
	ErrorPercent float64
}

// TierData represents one row in the Hardware Sizing Comparison
type TierData struct {
	Configuration       string
	SustainableCapacity string
	Status              string
	Bottleneck          string
}

// SizingData represents computed dynamic hardware recommendations
type SizingData struct {
	TierName             string
	APIVCPU              string
	APIRAM               string
	APICostEst           string
	PostgresVCPU         string
	PostgresRAM          string
	PostgresAdvice       string
	PostgresCostEst      string
	RedisVCPU            string
	RedisRAM             string
	RedisCostEst         string
	TotalCostEst         string
	OverprovisionWarning string

	IsRemoteTarget    bool
	SafetyHeadroomPct float64
	SizingRationale   string
	CostBudgetVPS     string
	CostPaaS          string
	CostHyperscaler   string
}

// DatabaseTelemetry holds database statistics harvested during the test
type DatabaseTelemetry struct {
	HasPostgres        bool
	PostgresActive     int
	PostgresMax        int
	PostgresCacheHit   float64
	PostgresLocks      int
	HasRedis           bool
	RedisMemoryMB      float64
	RedisConnected     int
	RedisHitRatio      float64
	RedisInstantaneous int
}

// ReportData represents everything needed to render the interactive dashboard
type ReportData struct {
	AppName              string
	TargetURL            string
	ContainerName        string
	GeneratedAt          string
	TargetVUs            int
	MaxObservedVUs       int
	SustainableVUs       int
	PrimaryBottleneck    string
	BottleneckAction     string
	IsBottleneckHealthy  bool
	SecondaryBottlenecks []string
	NonLimitingServices  []string
	Stages               []StageData
	DB                   DatabaseTelemetry
	MatrixTiers          []TierData
	HasMatrix            bool
	HasContainerMetrics  bool
	Sizing               SizingData
	EmbeddedChartJS      template.JS
	ChartLabelsJSON      template.JS
	ChartVUsJSON         template.JS
	ChartRPSJSON         template.JS
	ChartP95JSON         template.JS
	ChartCPUJSON         template.JS
}

// GenerateHTML bakes the telemetry into a self-contained, interactive HTML file
func GenerateHTML(
	filepath string,
	appName, targetURL, containerName, bottleneck, bottleneckAction string,
	secondary []string, nonLimiting []string,
	targetVUs, maxVUs, sustainableVUs int,
	stages []StageData,
	dbTele DatabaseTelemetry,
	matrixTiers []TierData,
	sizing SizingData,
) error {
	var labels []string
	var vus []int
	var rps []float64
	var p95 []float64
	var cpu []float64

	hasContainer := false
	for _, s := range stages {
		labels = append(labels, fmt.Sprintf("%d VUs", s.VUs))
		vus = append(vus, s.VUs)
		rps = append(rps, s.RPS)
		p95 = append(p95, s.P95Ms)
		cpu = append(cpu, s.CPUPercent)
		if s.CPUPercent > 0 || s.MemoryMB > 0 {
			hasContainer = true
		}
	}
	if containerName != "" {
		hasContainer = true
	}

	labelsJSON, _ := json.Marshal(labels)
	vusJSON, _ := json.Marshal(vus)
	rpsJSON, _ := json.Marshal(rps)
	p95JSON, _ := json.Marshal(p95)
	cpuJSON, _ := json.Marshal(cpu)

	isHealthy := strings.HasPrefix(strings.ToLower(bottleneck), "none") || bottleneck == ""

	data := ReportData{
		AppName:              appName,
		TargetURL:            targetURL,
		ContainerName:        containerName,
		GeneratedAt:          time.Now().Format("Jan 02, 2006 • 15:04:05 MST"),
		TargetVUs:            targetVUs,
		MaxObservedVUs:       maxVUs,
		SustainableVUs:       sustainableVUs,
		PrimaryBottleneck:    bottleneck,
		BottleneckAction:     bottleneckAction,
		IsBottleneckHealthy:  isHealthy,
		SecondaryBottlenecks: secondary,
		NonLimitingServices:  nonLimiting,
		Stages:               stages,
		DB:                   dbTele,
		MatrixTiers:          matrixTiers,
		HasMatrix:            len(matrixTiers) > 0,
		HasContainerMetrics:  hasContainer,
		Sizing:               sizing,
		EmbeddedChartJS:      template.JS(embeddedChartJS),
		ChartLabelsJSON:      template.JS(labelsJSON),
		ChartVUsJSON:         template.JS(vusJSON),
		ChartRPSJSON:         template.JS(rpsJSON),
		ChartP95JSON:         template.JS(p95JSON),
		ChartCPUJSON:         template.JS(cpuJSON),
	}

	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create report file: %w", err)
	}
	defer file.Close()

	return tmpl.Execute(file, data)
}

// OpenInBrowser automatically opens the generated report in the system default browser
func OpenInBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // Linux
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}

// Single-file embedded HTML dashboard (Linear / Vercel dark theme with Canvas charts)
const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>CapacityLab Report • {{.AppName}}</title>
  <script>{{.EmbeddedChartJS}}</script>
  <style>
    :root {
      --bg: #07090e;
      --card-bg: #0f131f;
      --card-hover: #141a29;
      --border: #1a2030;
      --border-subtle: rgba(255, 255, 255, 0.05);
      --text: #f8fafc;
      --muted: #94a3b8;
      --dim: #64748b;
      --accent: #38bdf8;
      --accent-glow: rgba(56, 189, 248, 0.15);
      --success: #10b981;
      --warning: #f59e0b;
      --danger: #ef4444;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Inter", "Helvetica Neue", Arial, sans-serif; }
    body { background-color: var(--bg); color: var(--text); padding: 36px 20px; line-height: 1.5; -webkit-font-smoothing: antialiased; }
    .container { max-width: 1200px; margin: 0 auto; }
    
    /* Header */
    .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 28px; border-bottom: 1px solid var(--border); padding-bottom: 20px; flex-wrap: wrap; gap: 16px; }
    .title-group h1 { font-size: 26px; font-weight: 700; letter-spacing: -0.5px; margin-bottom: 4px; display: flex; align-items: center; gap: 8px; }
    .title-group h1 span { color: var(--accent); }
    .meta { font-size: 13px; color: var(--muted); display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
    .meta a { color: var(--accent); text-decoration: none; }
    .meta a:hover { text-decoration: underline; }
    .badge { background: #151b2b; color: var(--accent); padding: 6px 14px; border-radius: 6px; font-size: 12px; font-weight: 600; border: 1px solid var(--border); display: inline-flex; align-items: center; gap: 6px; }
    .badge-cloud { background: rgba(56, 189, 248, 0.1); color: var(--accent); border-color: rgba(56, 189, 248, 0.3); }
    .badge-docker { background: rgba(16, 185, 129, 0.1); color: var(--success); border-color: rgba(16, 185, 129, 0.3); }

    /* Summary Grid */
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 16px; margin-bottom: 24px; }
    .card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 12px; padding: 20px; transition: border-color 0.2s ease, transform 0.2s ease; }
    .card:hover { border-color: rgba(56, 189, 248, 0.3); }
    .card-label { font-size: 11px; text-transform: uppercase; letter-spacing: 0.8px; color: var(--dim); margin-bottom: 8px; font-weight: 700; }
    .card-value { font-size: 28px; font-weight: 700; letter-spacing: -0.5px; }
    .card-sub { font-size: 13px; color: var(--muted); margin-top: 6px; line-height: 1.4; }
    .val-green { color: var(--success); }
    .val-red { color: var(--danger); }
    .val-blue { color: var(--accent); }

    /* Diagnostic Card */
    .diag-card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 12px; padding: 20px; margin-bottom: 24px; }

    /* Chart Grid & Cards */
    .chart-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-bottom: 24px; }
    @media (max-width: 900px) { .chart-grid { grid-template-columns: 1fr; } }
    .chart-card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 12px; padding: 20px; }
    .chart-title { font-size: 15px; font-weight: 600; margin-bottom: 16px; color: var(--text); }
    .chart-box { position: relative; height: 300px; width: 100%; }

    /* Table */
    .table-card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 12px; padding: 20px; overflow-x: auto; margin-bottom: 28px; }
    table { width: 100%; border-collapse: collapse; text-align: left; font-size: 13px; }
    th { color: var(--muted); font-weight: 600; border-bottom: 1px solid var(--border); padding: 12px 14px; font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px; white-space: nowrap; }
    td { padding: 12px 14px; border-bottom: 1px solid var(--border-subtle); font-variant-numeric: tabular-nums; white-space: nowrap; }
    tr:hover td { background-color: rgba(255, 255, 255, 0.02); }
    tr:last-child td { border-bottom: none; }
    .tag-fail { background: rgba(239, 68, 68, 0.15); color: var(--danger); padding: 3px 8px; border-radius: 4px; font-size: 11px; font-weight: 600; border: 1px solid rgba(239, 68, 68, 0.25); }
    .tag-pass { background: rgba(16, 185, 129, 0.15); color: var(--success); padding: 3px 8px; border-radius: 4px; font-size: 11px; font-weight: 600; border: 1px solid rgba(16, 185, 129, 0.25); }
    .tag-warn { background: rgba(245, 158, 11, 0.15); color: var(--warning); padding: 3px 8px; border-radius: 4px; font-size: 11px; font-weight: 600; border: 1px solid rgba(245, 158, 11, 0.25); }

    /* ═══════════════════════════════════════════════════════════════════════════════
       PRODUCTION SIZING & MULTI-CLOUD ARCHITECTURE (RE-ENGINEERED UX)
       ═══════════════════════════════════════════════════════════════════════════════ */
    .sizing-section {
      background: linear-gradient(180deg, #0e1322 0%, #070a12 100%);
      border: 1px solid rgba(56, 189, 248, 0.25);
      border-top: 3px solid #38bdf8;
      border-radius: 16px;
      padding: 28px;
      margin-bottom: 36px;
      box-shadow: 0 16px 40px -12px rgba(0, 0, 0, 0.6), 0 0 28px -6px rgba(56, 189, 248, 0.08);
    }
    .sizing-header {
      display: flex;
      justify-content: space-between;
      align-items: flex-start;
      margin-bottom: 16px;
      flex-wrap: wrap;
      gap: 12px;
    }
    .sizing-kicker {
      font-size: 10px;
      font-weight: 800;
      letter-spacing: 1.2px;
      color: var(--accent);
      margin-bottom: 4px;
    }
    .sizing-heading {
      font-size: 20px;
      font-weight: 700;
      color: #fff;
      letter-spacing: -0.4px;
    }
    .sizing-badges {
      display: flex;
      gap: 8px;
      align-items: center;
      flex-wrap: wrap;
    }
    .pill-tier {
      background: rgba(99, 102, 241, 0.15);
      border: 1px solid rgba(99, 102, 241, 0.4);
      color: #a5b4fc;
      padding: 5px 14px;
      border-radius: 9999px;
      font-size: 12px;
      font-weight: 700;
      letter-spacing: 0.2px;
    }
    .pill-headroom {
      background: rgba(16, 185, 129, 0.12);
      border: 1px solid rgba(16, 185, 129, 0.35);
      color: #34d399;
      padding: 5px 14px;
      border-radius: 9999px;
      font-size: 12px;
      font-weight: 600;
    }
    .sizing-rationale-bar {
      background: rgba(255, 255, 255, 0.025);
      border: 1px solid rgba(255, 255, 255, 0.06);
      border-left: 3px solid var(--accent);
      border-radius: 8px;
      padding: 12px 16px;
      display: flex;
      align-items: flex-start;
      gap: 10px;
      margin-bottom: 22px;
    }
    .rationale-bulb { font-size: 16px; line-height: 1.4; }
    .rationale-text { font-size: 13px; color: #cbd5e1; line-height: 1.5; }
    .rationale-text strong { color: #f8fafc; }
    .sizing-warning-bar {
      background: rgba(245, 158, 11, 0.08);
      border: 1px solid rgba(245, 158, 11, 0.3);
      border-left: 3px solid var(--warning);
      border-radius: 8px;
      padding: 12px 16px;
      display: flex;
      align-items: flex-start;
      gap: 10px;
      margin-bottom: 22px;
    }
    .warning-icon { font-size: 16px; line-height: 1.4; }
    .warning-text { font-size: 13px; color: #fde68a; line-height: 1.5; }

    /* Specs Bento Grid */
    .specs-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
      gap: 16px;
      margin-bottom: 28px;
    }
    .spec-card {
      background: #080c16;
      border: 1px solid rgba(255, 255, 255, 0.08);
      border-radius: 12px;
      padding: 18px;
      display: flex;
      flex-direction: column;
      justify-content: space-between;
      transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
    }
    .spec-card:hover {
      border-color: rgba(56, 189, 248, 0.35);
      transform: translateY(-2px);
    }
    .spec-card-top {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 14px;
    }
    .spec-type {
      font-size: 13px;
      font-weight: 700;
      color: #f1f5f9;
    }
    .spec-badge {
      font-size: 10px;
      text-transform: uppercase;
      letter-spacing: 0.6px;
      font-weight: 700;
      padding: 3px 8px;
      border-radius: 4px;
      background: rgba(56, 189, 248, 0.1);
      color: var(--accent);
      border: 1px solid rgba(56, 189, 248, 0.25);
    }
    .spec-badge-db {
      background: rgba(16, 185, 129, 0.1);
      color: var(--success);
      border-color: rgba(16, 185, 129, 0.25);
    }
    .spec-badge-redis {
      background: rgba(245, 158, 11, 0.1);
      color: var(--warning);
      border-color: rgba(245, 158, 11, 0.25);
    }
    .spec-main-val {
      display: flex;
      align-items: center;
      gap: 8px;
      margin-bottom: 12px;
      flex-wrap: wrap;
    }
    .spec-chip {
      background: rgba(255, 255, 255, 0.06);
      border: 1px solid rgba(255, 255, 255, 0.12);
      padding: 6px 12px;
      border-radius: 6px;
      font-size: 14px;
      font-weight: 700;
      color: #fff;
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    }
    .spec-divider { color: #64748b; font-weight: 700; }
    .spec-footer {
      font-size: 12px;
      color: var(--muted);
      border-top: 1px solid rgba(255, 255, 255, 0.05);
      padding-top: 10px;
      line-height: 1.4;
    }

    /* Cloud Pricing Cards */
    .cloud-comparison-area {
      border-top: 1px solid rgba(255, 255, 255, 0.08);
      padding-top: 24px;
    }
    .cloud-header { margin-bottom: 18px; }
    .cloud-title {
      font-size: 16px;
      font-weight: 700;
      color: #f8fafc;
      letter-spacing: -0.3px;
    }
    .cloud-subtitle {
      font-size: 13px;
      color: var(--muted);
      margin-top: 2px;
    }
    .cloud-pricing-cards {
      display: grid;
      grid-template-columns: repeat(3, 1fr);
      gap: 16px;
    }
    @media (max-width: 960px) {
      .cloud-pricing-cards { grid-template-columns: 1fr; }
    }
    .pricing-card {
      background: #080c16;
      border: 1px solid rgba(255, 255, 255, 0.08);
      border-radius: 12px;
      padding: 22px 20px;
      display: flex;
      flex-direction: column;
      justify-content: space-between;
      position: relative;
      transition: all 0.25s cubic-bezier(0.16, 1, 0.3, 1);
    }
    .pricing-card:hover {
      border-color: rgba(255, 255, 255, 0.22);
      transform: translateY(-3px);
      box-shadow: 0 12px 28px -8px rgba(0, 0, 0, 0.6);
    }
    .pricing-card-hero {
      border-color: rgba(56, 189, 248, 0.5);
      background: linear-gradient(180deg, rgba(56, 189, 248, 0.08) 0%, #080c16 100%);
      box-shadow: 0 0 24px rgba(56, 189, 248, 0.12);
    }
    .pricing-card-hero:hover {
      border-color: rgba(56, 189, 248, 0.8);
      box-shadow: 0 14px 32px -8px rgba(56, 189, 248, 0.25);
    }
    .hero-top-badge {
      position: absolute;
      top: -11px;
      left: 50%;
      transform: translateX(-50%);
      background: linear-gradient(90deg, #0284c7, #38bdf8);
      color: #04101e;
      font-size: 10px;
      font-weight: 800;
      letter-spacing: 0.8px;
      padding: 3px 12px;
      border-radius: 9999px;
      white-space: nowrap;
    }
    .pricing-card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 8px;
    }
    .pricing-pill {
      font-size: 11px;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 0.6px;
      color: var(--muted);
    }
    .pill-hero { color: var(--accent); }
    .pricing-tier-tag {
      font-size: 11px;
      color: var(--dim);
      font-weight: 600;
    }
    .tag-hero { color: #7dd3fc; font-weight: 600; }
    .provider-title {
      font-size: 15px;
      font-weight: 700;
      color: #f8fafc;
      margin-bottom: 14px;
    }
    .pricing-figure-box {
      margin-bottom: 16px;
      padding: 12px 14px;
      background: rgba(255, 255, 255, 0.03);
      border-radius: 8px;
      border: 1px solid rgba(255, 255, 255, 0.05);
    }
    .price-amount {
      font-size: 15px;
      font-weight: 800;
      color: #f8fafc;
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      line-height: 1.35;
      word-break: break-word;
    }
    .price-hero {
      color: var(--accent);
      text-shadow: 0 0 16px rgba(56, 189, 248, 0.25);
    }
    .price-enterprise {
      color: #fcd34d;
    }
    .pricing-features {
      list-style: none;
      display: flex;
      flex-direction: column;
      gap: 8px;
    }
    .pricing-features li {
      font-size: 12px;
      color: #94a3b8;
      display: flex;
      align-items: flex-start;
      gap: 8px;
      line-height: 1.4;
    }
    .check-mark {
      color: var(--success);
      font-weight: 700;
      flex-shrink: 0;
    }
    .check-hero { color: var(--accent); }
    .total-cost-card {
      background: #080c16;
      border: 1px solid var(--border);
      border-radius: 12px;
      padding: 20px;
      text-align: center;
    }
    .total-cost-label { font-size: 12px; text-transform: uppercase; color: var(--muted); letter-spacing: 0.6px; font-weight: 700; }
    .total-cost-figure { font-size: 26px; font-weight: 800; color: var(--accent); margin: 8px 0 4px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
    .total-cost-sub { font-size: 13px; color: var(--dim); }
  </style>
</head>
<body>
  <div class="container">
    <!-- Header -->
    <header class="header">
      <div class="title-group">
        <h1>⚡ CapacityLab <span>Report</span></h1>
        <div class="meta">
          <strong>{{.AppName}}</strong>
          <span>•</span>
          <a href="{{.TargetURL}}" target="_blank" rel="noopener noreferrer">{{.TargetURL}}</a>
          <span>•</span>
          <span>{{.GeneratedAt}}</span>
        </div>
      </div>
      <div>
        {{if .ContainerName}}
        <div class="badge badge-docker">🐳 Container: {{.ContainerName}}</div>
        {{else}}
        <div class="badge badge-cloud">☁️ Target: Hosted Cloud API</div>
        {{end}}
      </div>
    </header>

    <!-- Metrics Cards -->
    <div class="grid">
      <div class="card">
        <div class="card-label">Recommended Operating Load</div>
        <div class="card-value val-green">~{{.SustainableVUs}} users</div>
        <div class="card-sub">With {{printf "%.0f" .Sizing.SafetyHeadroomPct}}% operational safety headroom</div>
      </div>
      <div class="card">
        <div class="card-label">Measured Capacity Boundary</div>
        <div class="card-value val-blue">{{.MaxObservedVUs}} users</div>
        <div class="card-sub">Target workload: {{.TargetVUs}} users</div>
      </div>
      <div class="card">
        <div class="card-label">Primary Bottleneck</div>
        {{if .IsBottleneckHealthy}}
        <div class="card-value val-green" style="font-size: 20px; line-height: 1.3; margin-top: 4px;">✔ Healthy</div>
        <div class="card-sub">{{.PrimaryBottleneck}}</div>
        {{else}}
        <div class="card-value val-red" style="font-size: 17px; line-height: 1.3; margin-top: 4px;">{{.PrimaryBottleneck}}</div>
        {{if .BottleneckAction}}
        <div class="card-sub" style="color: #cbd5e1; margin-top: 6px;"><strong>Fix:</strong> {{.BottleneckAction}}</div>
        {{end}}
        {{end}}
      </div>
    </div>

    <!-- Full Diagnostic Breakdown (If secondary or non-limiting items exist) -->
    {{if or .SecondaryBottlenecks .NonLimitingServices}}
    <div class="diag-card">
      <div class="card-label" style="margin-bottom: 12px;">Full System Diagnostic Breakdown</div>
      {{if .SecondaryBottlenecks}}
      <div style="margin-bottom: 14px;">
        <span style="font-size: 13px; color: var(--warning); font-weight: 600;">Secondary Constraints:</span>
        <ul style="margin-left: 20px; font-size: 13px; color: var(--muted); margin-top: 4px;">
          {{range .SecondaryBottlenecks}}
          <li style="margin-bottom: 2px;">{{.}}</li>
          {{end}}
        </ul>
      </div>
      {{end}}
      {{if .NonLimitingServices}}
      <div>
        <span style="font-size: 13px; color: var(--success); font-weight: 600;">Healthy &amp; Non-Limiting Components:</span>
        <div style="display: flex; gap: 8px; flex-wrap: wrap; margin-top: 6px;">
          {{range .NonLimitingServices}}
          <span class="badge" style="background: rgba(16, 185, 129, 0.1); color: var(--success); border-color: rgba(16, 185, 129, 0.2);">✓ {{.}}</span>
          {{end}}
        </div>
      </div>
      {{end}}
    </div>
    {{end}}

    <!-- Database Telemetry Section (If present) -->
    {{if or .DB.HasPostgres .DB.HasRedis}}
    <div class="grid" style="margin-bottom: 24px;">
      {{if .DB.HasPostgres}}
      <div class="card">
        <div class="card-label">PostgreSQL Health</div>
        <div class="card-value val-blue">{{.DB.PostgresActive}} / {{.DB.PostgresMax}}</div>
        <div class="card-sub">Active Connections • Cache Hit: {{printf "%.1f" .DB.PostgresCacheHit}}%</div>
      </div>
      {{end}}
      {{if .DB.HasRedis}}
      <div class="card">
        <div class="card-label">Redis Health</div>
        <div class="card-value val-blue">{{printf "%.1f" .DB.RedisMemoryMB}} MB</div>
        <div class="card-sub">Hit Ratio: {{printf "%.1f" .DB.RedisHitRatio}}% • Ops: {{.DB.RedisInstantaneous}}/s</div>
      </div>
      {{end}}
    </div>
    {{end}}

    <!-- Dual Time-Series Charts -->
    <div class="chart-grid">
      <div class="chart-card">
        <div class="chart-title">Throughput (RPS) vs Concurrent Users</div>
        <div class="chart-box">
          <canvas id="rpsChart"></canvas>
        </div>
      </div>
      <div class="chart-card">
        <div class="chart-title">{{if .HasContainerMetrics}}p95 Latency (ms) &amp; Container CPU %{{else}}p95 Latency Progression (ms){{end}}</div>
        <div class="chart-box">
          <canvas id="latencyCpuChart"></canvas>
        </div>
      </div>
    </div>

    <!-- Hardware Matrix Comparison Table (If --matrix was run) -->
    {{if .HasMatrix}}
    <div class="table-card">
      <div class="chart-title">Hardware Sizing Comparison Matrix</div>
      <table>
        <thead>
          <tr>
            <th>Hardware Configuration</th>
            <th>Sustainable Capacity</th>
            <th>Status</th>
            <th>Primary Bottleneck</th>
          </tr>
        </thead>
        <tbody>
          {{range .MatrixTiers}}
          <tr>
            <td><strong>{{.Configuration}}</strong></td>
            <td>{{.SustainableCapacity}}</td>
            <td>
              {{if eq .Status "Recommended"}}
                <span class="tag-pass">{{.Status}}</span>
              {{else if eq .Status "Insufficient"}}
                <span class="tag-fail">{{.Status}}</span>
              {{else}}
                <span class="tag-warn">{{.Status}}</span>
              {{end}}
            </td>
            <td style="color: var(--muted);">{{.Bottleneck}}</td>
          </tr>
          {{end}}
        </tbody>
      </table>
    </div>
    {{end}}

    <!-- Stage Table -->
    <div class="table-card">
      <div class="chart-title">Benchmark Stage Telemetry</div>
      <table>
        <thead>
          <tr>
            <th>Stage</th>
            <th>VUs</th>
            <th>Throughput (RPS)</th>
            <th>p50 Latency</th>
            <th>p95 Latency</th>
            <th>API CPU %</th>
            <th>RAM Used</th>
            <th>Errors</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {{range .Stages}}
          <tr>
            <td><strong>#{{.StageNum}}</strong></td>
            <td>{{.VUs}} users</td>
            <td>{{printf "%.0f" .RPS}} req/s</td>
            <td>{{printf "%.1f" .P50Ms}} ms</td>
            <td>{{printf "%.1f" .P95Ms}} ms</td>
            <td>{{if $.HasContainerMetrics}}{{printf "%.1f" .CPUPercent}}%{{else}}<span style="color: var(--muted); font-size: 12px;">N/A (Remote)</span>{{end}}</td>
            <td>{{if $.HasContainerMetrics}}{{printf "%.1f" .MemoryMB}} MB{{else}}<span style="color: var(--muted); font-size: 12px;">N/A (Remote)</span>{{end}}</td>
            <td>{{printf "%.1f" .ErrorPercent}}%</td>
            <td>
              {{if gt .ErrorPercent 1.0}}
                <span class="tag-fail">Saturated</span>
              {{else}}
                <span class="tag-pass">Passed</span>
              {{end}}
            </td>
          </tr>
          {{end}}
        </tbody>
      </table>
    </div>

    <!-- Infrastructure Sizing Recommendation (Redesigned World-Class Architecture) -->
    <section class="sizing-section">
      <div class="sizing-header">
        <div class="sizing-title-group">
          <div class="sizing-kicker">INFRASTRUCTURE BLUEPRINT &amp; SIZING</div>
          <h2 class="sizing-heading">Candidate Infrastructure Sizing ({{.Sizing.TierName}})</h2>
        </div>
        <div class="sizing-badges">
          <span class="pill-tier">{{.Sizing.TierName}}</span>
          <span class="pill-headroom">🛡️ {{printf "%.0f" .Sizing.SafetyHeadroomPct}}% Headroom Included</span>
        </div>
      </div>

      <!-- Sizing Rationale -->
      <div class="sizing-rationale-bar">
        <span class="rationale-bulb">💡</span>
        <div class="rationale-text">
          <strong>Sizing Rationale:</strong> {{if .Sizing.SizingRationale}}{{.Sizing.SizingRationale}}{{else}}Based on observed container resource consumption, this configuration ensures sustained throughput without cascading queueing.{{end}}
        </div>
      </div>

      {{if .Sizing.OverprovisionWarning}}
      <div class="sizing-warning-bar">
        <span class="warning-icon">⚠️</span>
        <div class="warning-text">
          <strong>Cost Optimization Notice:</strong> {{.Sizing.OverprovisionWarning}}
        </div>
      </div>
      {{end}}

      <!-- Compute Specs Bento -->
      <div class="specs-grid">
        <div class="spec-card">
          <div class="spec-card-top">
            <span class="spec-type">API / Application Compute</span>
            <span class="spec-badge">Primary Target</span>
          </div>
          <div class="spec-main-val">
            <span class="spec-chip">⚡ {{.Sizing.APIVCPU}}</span>
            <span class="spec-divider">+</span>
            <span class="spec-chip">💾 {{.Sizing.APIRAM}}</span>
          </div>
          <div class="spec-footer">
            {{if .Sizing.APICostEst}}
            <span>Est. Standalone Compute: <strong>{{.Sizing.APICostEst}}</strong></span>
            {{else}}
            <span>Sized for ~{{.SustainableVUs}} sustainable concurrent users</span>
            {{end}}
          </div>
        </div>

        {{if .DB.HasPostgres}}
        <div class="spec-card">
          <div class="spec-card-top">
            <span class="spec-type">PostgreSQL Database</span>
            <span class="spec-badge spec-badge-db">Storage &amp; Pool</span>
          </div>
          <div class="spec-main-val">
            <span class="spec-chip">🐘 {{.Sizing.PostgresVCPU}}</span>
            <span class="spec-divider">+</span>
            <span class="spec-chip">💾 {{.Sizing.PostgresRAM}}</span>
          </div>
          <div class="spec-footer">
            <span>{{if .Sizing.PostgresAdvice}}{{.Sizing.PostgresAdvice}}{{else}}Standard connection pool adequate{{end}}</span>
          </div>
        </div>
        {{end}}

        {{if .DB.HasRedis}}
        <div class="spec-card">
          <div class="spec-card-top">
            <span class="spec-type">Redis In-Memory Cache</span>
            <span class="spec-badge spec-badge-redis">Cache Tier</span>
          </div>
          <div class="spec-main-val">
            <span class="spec-chip">⚡ {{.Sizing.RedisVCPU}}</span>
            <span class="spec-divider">+</span>
            <span class="spec-chip">💾 {{.Sizing.RedisRAM}}</span>
          </div>
          <div class="spec-footer">
            <span>{{if .Sizing.RedisCostEst}}{{.Sizing.RedisCostEst}}{{else}}Sized for working memory set{{end}}</span>
          </div>
        </div>
        {{end}}
      </div>

      {{if .Sizing.CostBudgetVPS}}
      <!-- Multi-Cloud Pricing Comparison (3 Modern Cards) -->
      <div class="cloud-comparison-area">
        <div class="cloud-header">
          <h3 class="cloud-title">Multi-Cloud Monthly Hosting Cost Estimates</h3>
          <p class="cloud-subtitle">Transparent pricing comparison across standard cloud hosting models for this application's safe production capacity.</p>
        </div>

        <div class="cloud-pricing-cards">
          <!-- 1. Budget VPS -->
          <div class="pricing-card">
            <div class="pricing-card-header">
              <div class="pricing-pill">Budget VPS</div>
              <span class="pricing-tier-tag">Cost-Effective</span>
            </div>
            <div class="provider-title">Hetzner / DigitalOcean</div>
            <div class="pricing-figure-box">
              <div class="price-amount">{{.Sizing.CostBudgetVPS}}</div>
            </div>
            <ul class="pricing-features">
              <li><span class="check-mark">✓</span> Single-tenant VPS / Bare-metal compute</li>
              <li><span class="check-mark">✓</span> Best for Docker Compose self-hosted setups</li>
              <li><span class="check-mark">✓</span> Maximum CPU &amp; RAM per dollar</li>
            </ul>
          </div>

          <!-- 2. Managed PaaS (Hero Card) -->
          <div class="pricing-card pricing-card-hero">
            <div class="hero-top-badge">RECOMMENDED FOR SPEED</div>
            <div class="pricing-card-header">
              <div class="pricing-pill pill-hero">Managed PaaS</div>
              <span class="pricing-tier-tag tag-hero">Zero-DevOps</span>
            </div>
            <div class="provider-title">Render / Fly.io / Railway</div>
            <div class="pricing-figure-box">
              <div class="price-amount price-hero">{{.Sizing.CostPaaS}}</div>
            </div>
            <ul class="pricing-features">
              <li><span class="check-mark check-hero">✓</span> Automated Git push-to-deploy workflows</li>
              <li><span class="check-mark check-hero">✓</span> Built-in TLS, auto-restart &amp; health checks</li>
              <li><span class="check-mark check-hero">✓</span> Fully managed PostgreSQL with daily backups</li>
            </ul>
          </div>

          <!-- 3. Hyperscaler -->
          <div class="pricing-card">
            <div class="pricing-card-header">
              <div class="pricing-pill">Hyperscaler</div>
              <span class="pricing-tier-tag">Enterprise Scale</span>
            </div>
            <div class="provider-title">AWS / Google Cloud</div>
            <div class="pricing-figure-box">
              <div class="price-amount price-enterprise">{{.Sizing.CostHyperscaler}}</div>
            </div>
            <ul class="pricing-features">
              <li><span class="check-mark">✓</span> Multi-Availability-Zone high availability</li>
              <li><span class="check-mark">✓</span> AWS ECS Fargate / EKS + AWS Aurora/RDS</li>
              <li><span class="check-mark">✓</span> SOC2, HIPAA &amp; Enterprise VPC isolation</li>
            </ul>
          </div>
        </div>
      </div>
      {{else if .Sizing.TotalCostEst}}
      <div class="cloud-comparison-area">
        <div class="total-cost-card">
          <div class="total-cost-label">Estimated Monthly Cloud Spend</div>
          <div class="total-cost-figure">{{.Sizing.TotalCostEst}}</div>
          <div class="total-cost-sub">Estimated across standard public cloud infrastructure for target workload.</div>
        </div>
      </div>
      {{end}}

      <div style="margin-top: 18px; padding: 12px 16px; background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.06); border-radius: 8px; font-size: 12px; color: var(--muted); line-height: 1.5;">
        <strong style="color: #94a3b8;">Methodology Notice:</strong> Sustainable operating capacity applies a configurable {{printf "%.0f" .Sizing.SafetyHeadroomPct}}% operational safety headroom to the empirical capacity boundary. Recommended infrastructure tiers and cost comparisons represent candidate starting baselines derived under test conditions; calibrate against production telemetry before committing resources.
      </div>
    </section>
  </div>

  <script>
    const labels = {{.ChartLabelsJSON}};
    const rpsData = {{.ChartRPSJSON}};
    const p95Data = {{.ChartP95JSON}};
    const cpuData = {{.ChartCPUJSON}};

    // RPS Chart
    new Chart(document.getElementById('rpsChart'), {
      type: 'line',
      data: {
        labels: labels,
        datasets: [{
          label: 'Throughput (RPS)',
          data: rpsData,
          borderColor: '#38bdf8',
          backgroundColor: 'rgba(56, 189, 248, 0.1)',
          fill: true,
          tension: 0.3
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { labels: { color: '#94a3b8' } } },
        scales: {
          x: { grid: { color: '#1e2433' }, ticks: { color: '#94a3b8' } },
          y: { grid: { color: '#1e2433' }, ticks: { color: '#94a3b8' } }
        }
      }
    });

    // Latency & CPU Chart
    const hasContainer = {{.HasContainerMetrics}};
    const datasets = [
      {
        label: 'p95 Latency (ms)',
        data: p95Data,
        borderColor: '#f59e0b',
        backgroundColor: 'rgba(245, 158, 11, 0.1)',
        tension: 0.3,
        yAxisID: 'y'
      }
    ];

    const chartScales = {
      x: { grid: { color: '#1e2433' }, ticks: { color: '#94a3b8' } },
      y: {
        position: 'left',
        grid: { color: '#1e2433' },
        ticks: { color: '#f59e0b' },
        title: { display: true, text: 'Latency (ms)', color: '#f59e0b' }
      }
    };

    if (hasContainer) {
      datasets.push({
        label: 'API CPU %',
        data: cpuData,
        borderColor: '#ef4444',
        borderDash: [5, 5],
        tension: 0.3,
        yAxisID: 'y1'
      });
      chartScales.y1 = {
        position: 'right',
        grid: { drawOnChartArea: false },
        ticks: { color: '#ef4444' },
        title: { display: true, text: 'CPU %', color: '#ef4444' }
      };
    }

    new Chart(document.getElementById('latencyCpuChart'), {
      type: 'line',
      data: {
        labels: labels,
        datasets: datasets
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { labels: { color: '#94a3b8' } } },
        scales: chartScales
      }
    });
  </script>
</body>
</html>`
