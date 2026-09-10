package report

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"runtime"
	"time"
)

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
	AppName             string
	TargetURL           string
	ContainerName       string
	GeneratedAt         string
	TargetVUs           int
	MaxObservedVUs      int
	SustainableVUs      int
	PrimaryBottleneck   string
	Stages              []StageData
	DB                  DatabaseTelemetry
	MatrixTiers         []TierData
	HasMatrix           bool
	ChartLabelsJSON     template.JS
	ChartVUsJSON        template.JS
	ChartRPSJSON        template.JS
	ChartP95JSON        template.JS
	ChartCPUJSON        template.JS
}

// GenerateHTML bakes the telemetry into a self-contained, interactive HTML file
func GenerateHTML(
	filepath string,
	appName, targetURL, containerName, bottleneck string,
	targetVUs, maxVUs, sustainableVUs int,
	stages []StageData,
	dbTele DatabaseTelemetry,
	matrixTiers []TierData,
) error {
	var labels []string
	var vus []int
	var rps []float64
	var p95 []float64
	var cpu []float64

	for _, s := range stages {
		labels = append(labels, fmt.Sprintf("%d VUs", s.VUs))
		vus = append(vus, s.VUs)
		rps = append(rps, s.RPS)
		p95 = append(p95, s.P95Ms)
		cpu = append(cpu, s.CPUPercent)
	}

	labelsJSON, _ := json.Marshal(labels)
	vusJSON, _ := json.Marshal(vus)
	rpsJSON, _ := json.Marshal(rps)
	p95JSON, _ := json.Marshal(p95)
	cpuJSON, _ := json.Marshal(cpu)

	data := ReportData{
		AppName:           appName,
		TargetURL:         targetURL,
		ContainerName:     containerName,
		GeneratedAt:       time.Now().Format("Jan 02, 2006 • 15:04:05 MST"),
		TargetVUs:         targetVUs,
		MaxObservedVUs:    maxVUs,
		SustainableVUs:    sustainableVUs,
		PrimaryBottleneck: bottleneck,
		Stages:            stages,
		DB:                dbTele,
		MatrixTiers:       matrixTiers,
		HasMatrix:         len(matrixTiers) > 0,
		ChartLabelsJSON:   template.JS(labelsJSON),
		ChartVUsJSON:      template.JS(vusJSON),
		ChartRPSJSON:      template.JS(rpsJSON),
		ChartP95JSON:      template.JS(p95JSON),
		ChartCPUJSON:      template.JS(cpuJSON),
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
  <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
  <style>
    :root {
      --bg: #090a0f;
      --card-bg: #12151e;
      --border: #1e2433;
      --text: #f1f5f9;
      --muted: #94a3b8;
      --accent: #38bdf8;
      --success: #10b981;
      --warning: #f59e0b;
      --danger: #ef4444;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
    body { background-color: var(--bg); color: var(--text); padding: 40px 20px; }
    .container { max-width: 1200px; margin: 0 auto; }
    
    /* Header */
    .header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 32px; border-bottom: 1px solid var(--border); padding-bottom: 24px; }
    .title-group h1 { font-size: 28px; font-weight: 700; letter-spacing: -0.5px; margin-bottom: 6px; }
    .title-group h1 span { color: var(--accent); }
    .meta { font-size: 14px; color: var(--muted); }
    .badge { background: #1e293b; color: var(--accent); padding: 4px 10px; border-radius: 6px; font-size: 12px; font-weight: 600; border: 1px solid var(--border); }

    /* Summary Grid */
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 16px; margin-bottom: 32px; }
    .card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 12px; padding: 20px; }
    .card-label { font-size: 12px; text-transform: uppercase; letter-spacing: 0.5px; color: var(--muted); margin-bottom: 8px; font-weight: 600; }
    .card-value { font-size: 28px; font-weight: 700; }
    .card-sub { font-size: 13px; color: var(--muted); margin-top: 6px; }
    .val-green { color: var(--success); }
    .val-red { color: var(--danger); }
    .val-blue { color: var(--accent); }

    /* Chart Card */
    .chart-card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 12px; padding: 24px; margin-bottom: 32px; }
    .chart-title { font-size: 16px; font-weight: 600; margin-bottom: 16px; }
    .chart-box { position: relative; height: 320px; width: 100%; }

    /* Table */
    .table-card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 12px; padding: 24px; overflow-x: auto; margin-bottom: 32px; }
    table { width: 100%; border-collapse: collapse; text-align: left; font-size: 14px; }
    th { color: var(--muted); font-weight: 600; border-bottom: 1px solid var(--border); padding: 12px 16px; font-size: 12px; text-transform: uppercase; }
    td { padding: 14px 16px; border-bottom: 1px solid #171c28; }
    tr:last-child td { border-bottom: none; }
    .tag-fail { background: rgba(239, 68, 68, 0.15); color: var(--danger); padding: 3px 8px; border-radius: 4px; font-size: 12px; font-weight: 600; }
    .tag-pass { background: rgba(16, 185, 129, 0.15); color: var(--success); padding: 3px 8px; border-radius: 4px; font-size: 12px; font-weight: 600; }
    .tag-warn { background: rgba(245, 158, 11, 0.15); color: var(--warning); padding: 3px 8px; border-radius: 4px; font-size: 12px; font-weight: 600; }

    /* Sizing Box */
    .recom-box { background: linear-gradient(145deg, #131b2e, #101624); border: 1px solid #2563eb44; border-radius: 12px; padding: 24px; margin-bottom: 32px; }
    .recom-box h3 { font-size: 18px; margin-bottom: 12px; color: var(--accent); }
    .recom-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 16px; margin-top: 16px; }
    .recom-item { background: #0b1120; border: 1px solid var(--border); border-radius: 8px; padding: 14px; }
    .recom-item .label { font-size: 12px; color: var(--muted); }
    .recom-item .val { font-size: 18px; font-weight: 700; margin-top: 4px; color: #fff; }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <div class="title-group">
        <h1>CapacityLab <span>Report</span></h1>
        <div class="meta">{{.AppName}} • {{.TargetURL}} • {{.GeneratedAt}}</div>
      </div>
      <div class="badge">Container: {{.ContainerName}}</div>
    </div>

    <!-- Metrics Cards -->
    <div class="grid">
      <div class="card">
        <div class="card-label">Recommended Capacity</div>
        <div class="card-value val-green">~{{.SustainableVUs}} users</div>
        <div class="card-sub">With 30% production safety buffer</div>
      </div>
      <div class="card">
        <div class="card-label">Max Observed Load</div>
        <div class="card-value val-blue">{{.MaxObservedVUs}} users</div>
        <div class="card-sub">Target workload: {{.TargetVUs}} users</div>
      </div>
      <div class="card">
        <div class="card-label">Primary Bottleneck</div>
        <div class="card-value val-red" style="font-size: 16px; line-height: 1.4; margin-top: 6px;">{{.PrimaryBottleneck}}</div>
      </div>
    </div>

    <!-- Database Telemetry Section (If present) -->
    {{if or .DB.HasPostgres .DB.HasRedis}}
    <div class="grid" style="margin-bottom: 32px;">
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
    <div class="grid" style="grid-template-columns: 1fr 1fr;">
      <div class="chart-card">
        <div class="chart-title">Throughput (RPS) vs Concurrent Users</div>
        <div class="chart-box">
          <canvas id="rpsChart"></canvas>
        </div>
      </div>
      <div class="chart-card">
        <div class="chart-title">p95 Latency (ms) & Container CPU %</div>
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
            <td>{{printf "%.1f" .CPUPercent}}%</td>
            <td>{{printf "%.1f" .MemoryMB}} MB</td>
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

    <!-- Infrastructure Sizing Recommendation -->
    <div class="recom-box">
      <h3>Recommended Production Sizing</h3>
      <p style="font-size: 14px; color: var(--muted);">Based on observed container resource consumption, this configuration ensures sustained throughput without cascading queueing.</p>
      <div class="recom-grid">
        <div class="recom-item">
          <div class="label">API Compute</div>
          <div class="val">1.0 vCPU</div>
        </div>
        <div class="recom-item">
          <div class="label">API Memory</div>
          <div class="val">512 MB RAM</div>
        </div>
        <div class="recom-item">
          <div class="label">PostgreSQL Sizing</div>
          <div class="val">1.0 vCPU • 1 GB</div>
        </div>
        <div class="recom-item">
          <div class="label">Redis Sizing</div>
          <div class="val">0.25 vCPU • 256 MB</div>
        </div>
      </div>
    </div>
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
    new Chart(document.getElementById('latencyCpuChart'), {
      type: 'line',
      data: {
        labels: labels,
        datasets: [
          {
            label: 'p95 Latency (ms)',
            data: p95Data,
            borderColor: '#f59e0b',
            tension: 0.3,
            yAxisID: 'y'
          },
          {
            label: 'API CPU %',
            data: cpuData,
            borderColor: '#ef4444',
            borderDash: [5, 5],
            tension: 0.3,
            yAxisID: 'y1'
          }
        ]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { labels: { color: '#94a3b8' } } },
        scales: {
          x: { grid: { color: '#1e2433' }, ticks: { color: '#94a3b8' } },
          y: {
            position: 'left',
            grid: { color: '#1e2433' },
            ticks: { color: '#f59e0b' },
            title: { display: true, text: 'Latency (ms)', color: '#f59e0b' }
          },
          y1: {
            position: 'right',
            grid: { drawOnChartArea: false },
            ticks: { color: '#ef4444' },
            title: { display: true, text: 'CPU %', color: '#ef4444' }
          }
        }
      }
    });
  </script>
</body>
</html>`
