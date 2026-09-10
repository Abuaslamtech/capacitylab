package report

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"time"
)

// DashboardRunSummary represents a condensed run for the dashboard view
type DashboardRunSummary struct {
	ID             string  `json:"id"`
	Date           string  `json:"date"`
	AppName        string  `json:"app_name"`
	TargetURL      string  `json:"target_url"`
	SustainableVUs int     `json:"sustainable_vus"`
	MaxObservedVUs int     `json:"max_observed_vus"`
	PeakRPS        float64 `json:"peak_rps"`
	P95Ms          float64 `json:"p95_ms"`
	Bottleneck     string  `json:"bottleneck"`
	Fix            string  `json:"fix"`
}

// DashboardData drives the multi-run historical dashboard template
type DashboardData struct {
	GeneratedAt     string
	TotalRuns       int
	LatestApp       string
	Runs            []DashboardRunSummary
	ChartLabelsJSON template.JS
	ChartCapJSON    template.JS
	ChartRPSJSON    template.JS
}

// GenerateDashboardHTML bakes historical runs into an interactive multi-run dashboard
func GenerateDashboardHTML(filepathStr string, runs []DashboardRunSummary) error {
	var labels []string
	var capacities []int
	var rps []float64

	// Sort chronologically (oldest to newest) for trend chart
	for i := len(runs) - 1; i >= 0; i-- {
		r := runs[i]
		labels = append(labels, r.Date)
		capacities = append(capacities, r.SustainableVUs)
		rps = append(rps, r.PeakRPS)
	}

	labelsJSON, _ := json.Marshal(labels)
	capJSON, _ := json.Marshal(capacities)
	rpsJSON, _ := json.Marshal(rps)

	latestApp := "All Apps"
	if len(runs) > 0 {
		latestApp = runs[0].AppName
	}

	data := DashboardData{
		GeneratedAt:     time.Now().Format("Jan 02, 2006 • 15:04:05 MST"),
		TotalRuns:       len(runs),
		LatestApp:       latestApp,
		Runs:            runs,
		ChartLabelsJSON: template.JS(labelsJSON),
		ChartCapJSON:    template.JS(capJSON),
		ChartRPSJSON:    template.JS(rpsJSON),
	}

	tmpl, err := template.New("dashboard").Parse(dashboardTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse dashboard template: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(filepathStr), 0755); err != nil {
		return err
	}

	file, err := os.Create(filepathStr)
	if err != nil {
		return fmt.Errorf("failed to create dashboard file: %w", err)
	}
	defer file.Close()

	return tmpl.Execute(file, data)
}

const dashboardTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>CapacityLab • Historical Trends Dashboard</title>
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
    
    .header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 32px; border-bottom: 1px solid var(--border); padding-bottom: 24px; }
    .title-group h1 { font-size: 28px; font-weight: 700; letter-spacing: -0.5px; margin-bottom: 6px; }
    .title-group h1 span { color: var(--accent); }
    .meta { font-size: 14px; color: var(--muted); }
    .badge { background: #1e293b; color: var(--accent); padding: 6px 12px; border-radius: 6px; font-size: 13px; font-weight: 600; border: 1px solid var(--border); }

    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 16px; margin-bottom: 32px; }
    .card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 12px; padding: 20px; }
    .card-label { font-size: 12px; text-transform: uppercase; letter-spacing: 0.5px; color: var(--muted); margin-bottom: 8px; font-weight: 600; }
    .card-value { font-size: 28px; font-weight: 700; }
    .val-green { color: var(--success); }
    .val-blue { color: var(--accent); }

    .chart-card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 12px; padding: 24px; margin-bottom: 32px; }
    .chart-title { font-size: 16px; font-weight: 600; margin-bottom: 16px; }
    .chart-box { position: relative; height: 320px; width: 100%; }

    .table-card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 12px; padding: 24px; overflow-x: auto; margin-bottom: 32px; }
    table { width: 100%; border-collapse: collapse; text-align: left; font-size: 14px; }
    th { color: var(--muted); font-weight: 600; border-bottom: 1px solid var(--border); padding: 12px 16px; font-size: 12px; text-transform: uppercase; }
    td { padding: 14px 16px; border-bottom: 1px solid #171c28; }
    tr:hover td { background-color: rgba(56, 189, 248, 0.03); }
    tr:last-child td { border-bottom: none; }
    .tag-pass { background: rgba(16, 185, 129, 0.15); color: var(--success); padding: 3px 8px; border-radius: 4px; font-size: 12px; font-weight: 600; }
    .tag-fail { background: rgba(239, 68, 68, 0.15); color: var(--danger); padding: 3px 8px; border-radius: 4px; font-size: 12px; font-weight: 600; }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <div class="title-group">
        <h1>CapacityLab <span>History Dashboard</span></h1>
        <div class="meta">{{.LatestApp}} • {{.GeneratedAt}}</div>
      </div>
      <div class="badge">{{.TotalRuns}} Runs Recorded</div>
    </div>

    <div class="chart-card">
      <div class="chart-title">Capacity & Throughput Growth Over Time</div>
      <div class="chart-box">
        <canvas id="trendChart"></canvas>
      </div>
    </div>

    <div class="table-card">
      <div class="chart-title">All Historical Runs</div>
      <table>
        <thead>
          <tr>
            <th>Run ID</th>
            <th>Date</th>
            <th>Application</th>
            <th>Sustainable Capacity</th>
            <th>Peak Throughput</th>
            <th>p95 Latency</th>
            <th>Primary Bottleneck</th>
          </tr>
        </thead>
        <tbody>
          {{range .Runs}}
          <tr>
            <td style="font-family: monospace; font-size: 13px; color: var(--accent);">{{.ID}}</td>
            <td>{{.Date}}</td>
            <td><strong>{{.AppName}}</strong></td>
            <td><span class="tag-pass">~{{.SustainableVUs}} users</span></td>
            <td>{{printf "%.0f" .PeakRPS}} RPS</td>
            <td>{{printf "%.1f" .P95Ms}} ms</td>
            <td>
              {{if eq .Bottleneck "None"}}
                <span class="tag-pass">Healthy</span>
              {{else}}
                <span class="tag-fail">{{.Bottleneck}}</span>
              {{end}}
            </td>
          </tr>
          {{end}}
        </tbody>
      </table>
    </div>
  </div>

  <script>
    const labels = {{.ChartLabelsJSON}};
    const capacities = {{.ChartCapJSON}};
    const rps = {{.ChartRPSJSON}};

    const ctx = document.getElementById('trendChart').getContext('2d');
    new Chart(ctx, {
      type: 'line',
      data: {
        labels: labels,
        datasets: [
          {
            label: 'Sustainable Users',
            data: capacities,
            borderColor: '#10b981',
            backgroundColor: 'rgba(16, 185, 129, 0.1)',
            fill: true,
            tension: 0.3,
            yAxisID: 'y'
          },
          {
            label: 'Peak Throughput (RPS)',
            data: rps,
            borderColor: '#38bdf8',
            borderDash: [5, 5],
            fill: false,
            tension: 0.3,
            yAxisID: 'y1'
          }
        ]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: 'index', intersect: false },
        scales: {
          x: { grid: { color: '#1e2433' }, ticks: { color: '#94a3b8' } },
          y: {
            type: 'linear',
            position: 'left',
            title: { display: true, text: 'Sustainable Users', color: '#10b981' },
            grid: { color: '#1e2433' },
            ticks: { color: '#10b981' }
          },
          y1: {
            type: 'linear',
            position: 'right',
            title: { display: true, text: 'Peak RPS', color: '#38bdf8' },
            grid: { drawOnChartArea: false },
            ticks: { color: '#38bdf8' }
          }
        },
        plugins: {
          legend: { labels: { color: '#f1f5f9' } }
        }
      }
    });
  </script>
</body>
</html>
`
