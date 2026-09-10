<div align="center">

# CapacityLab

### Find your backend's real capacity before your users do.

**Open-source CLI that load-tests your backend API, discovers its measured capacity boundary, classifies bottlenecks, and estimates candidate starting infrastructure.**

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![CI](https://github.com/Abuaslamtech/capacitylab/actions/workflows/ci.yml/badge.svg)](https://github.com/Abuaslamtech/capacitylab/actions/workflows/ci.yml)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20WSL2-lightgrey)](#)

</div>

---

## The CapacityLab Methodology: Observed vs. Inferred vs. Recommended

Most load generators (k6, Locust, JMeter) stop at throughput charts and latency percentiles. CapacityLab answers the engineering question that follows: **"How much traffic can this backend safely carry under this workload, what breaks first, and what starting infrastructure should we provision?"**

To keep findings verifiable, every benchmark decision is structured into three layers:

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. OBSERVED METRICS                                                         │
│    Empirical test measurements: peak VUs, P50/P95 latency, RPS, error %    │
│    and live container cgroup telemetry (CPU %, RAM MB, DB connection pools). │
├─────────────────────────────────────────────────────────────────────────────┤
│ 2. INFERRED BOTTLENECK                                                      │
│    Algorithmic root-cause diagnosis: identifies saturation knee-points      │
│    and correlates latency surges to specific resources (CPU, DB, network). │
├─────────────────────────────────────────────────────────────────────────────┤
│ 3. RECOMMENDED SIZING                                                       │
│    Operational infrastructure guidance: applies a configurable safety factor│
│    and maps workload demands to candidate starting cloud infrastructure.   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Measured Boundary vs. Safe Production Load
A capacity benchmark does not prove an immutable physical maximum. It discovers performance behavior under a specific test configuration, dataset, network path, and SLA criteria.

CapacityLab explicitly distinguishes:
- **Measured Capacity Boundary:** The highest load observed during the test with zero SLA breaches.
- **Safe Production Load:** A recommended production threshold calculated via a configurable safety factor:

$$\text{Safe Production Load} = \text{Measured Capacity Boundary} \times \text{safety\_factor}$$

By default, CapacityLab applies `safety_factor: 0.70` (providing a **30% operational headroom buffer** for traffic spikes, background jobs, and latency variance). Configure this in `capacitylab.yaml`:

```yaml
thresholds:
  max_p95_latency_ms: 1500
  max_error_rate_percent: 1.0
  safety_factor: 0.70    # Range 0.10 to 1.0 (e.g., 0.70 = 30% headroom, 0.80 = 20% headroom)
```

---

## Quickstart

### Option A: Test a Hosted API Directly
```bash
capacitylab run --url https://api.example.com --start-users 5 --max-users 50 --open
```

### Option B: Guided Setup Wizard
```bash
# 1. Interactive setup (generates capacitylab.yaml)
capacitylab init

# 2. Pre-flight check (verifies syntax & network connectivity)
capacitylab validate

# 3. Run benchmark and view interactive HTML report
capacitylab run --open
```

### Option C: Run the Docker Sandbox
Run a test without configuring an external backend using the [`examples/quickstart/`](examples/quickstart/) sandbox:
```bash
cd examples/quickstart
docker compose up -d --build
capacitylab run --open

# When finished
docker compose down
```
Benchmarks a sample Go microservice constrained to **0.5 vCPU / 256MB RAM** with authenticated user journeys, JWT token extraction, and live container cgroup telemetry.

---

## Installation

```bash
# Build from source (Go 1.22+)
git clone https://github.com/Abuaslamtech/capacitylab.git
cd capacitylab
go build -o capacitylab ./cmd/capacitylab
sudo mv capacitylab /usr/local/bin/

# Or via go install
go install github.com/Abuaslamtech/capacitylab/cmd/capacitylab@latest

# Verify
capacitylab version
```

---

## Testing Scenarios

### 1. Benchmark a Hosted / Cloud API
**Goal:** Determine how much traffic an external API (Render, Fly.io, AWS, Staging) can handle, and estimate monthly hosting costs.

**Command:**
```bash
capacitylab run --config capacitylab.yaml --open
```

**Config (`capacitylab.yaml`):**
```yaml
application:
  name: billing-api
  url: https://billing-api.onrender.com/api/v1

workload:
  type: step-ramp
  start_users: 5
  max_users: 50
  step: 10
  step_duration: 10s

thresholds:
  max_p95_latency_ms: 1500
  max_error_rate_percent: 1.0
  safety_factor: 0.70 # 30% operational headroom

scenarios:
  - name: invoices
    weight: 100
    flow:
      - get: /health
      - get: /invoices
```

**What it does & output:**
Increments virtual users step-by-step. Discovers the capacity boundary where latency diverges, calculates safe operating capacity with configurable headroom, and outputs transparent multi-cloud cost estimates:

```
===============================================================
             CAPACITY DECISION & EVIDENCE SUMMARY              
===============================================================

[OBSERVED METRICS]
  Target Workload Tested:    50 concurrent users
  Measured Capacity Boundary: 25 concurrent users
  Boundary Throughput:       28.3 req/s (p50: 182.4ms, p95: 394.1ms, errors: 0.0%)
  Runner Host Integrity:     Zero runner starvation (Peak CPU: 3.4%, RAM: 28.1 MB, TIME_WAIT: 12)

[INFERRED BOTTLENECK]
  Limiting Resource:         [CRITICAL] Downstream Latency Saturation
  Diagnostic Evidence:       p95 latency surged 3.1x crossing SLA threshold at stage 3
  Actionable Fix:            Optimize endpoint query latency or scale upstream database read replicas

[RECOMMENDED SIZING]
  Safe Production Load:      ~17 concurrent users (using 0.70 safety factor / 30% operational headroom)
  Candidate Starting Tier:   Small Tier
  Target Host Sizing:        1.0 vCPU, 1 GB RAM (Estimated from traffic ceiling)

  Multi-Cloud Cost Estimates:
    ➔ Budget VPS:            $6–$10/mo (Hetzner CX22 / DO 1GB)
    ➔ Managed PaaS:          $14–$21/mo (Render Starter Web + DB / Fly.io)
    ➔ Hyperscaler:           $25–$45/mo (AWS t4g.small + RDS db.t4g.micro)

  Note: Tiers and costs are candidate starting estimates derived under test conditions. Calibrate against production.
─────────────────────────────────────────────────────────────
```

---

### 2. Right-Size a Local Docker Stack
**Goal:** Measure container CPU and memory consumption under load to set optimal resource limits in Docker Compose or Kubernetes.

**Command:**
```bash
docker compose up -d
capacitylab run --open
```

**Config (`capacitylab.yaml`):**
```yaml
application:
  name: order-service
  url: http://localhost:8080

services:
  api:
    container: order-service-api   # Container name to monitor
    cpu: "1.0"                     # Optional: enforce 1.0 core limit
    memory: "512MB"                # Optional: enforce 512MB limit
  postgres:
    dsn: postgres://user:pass@localhost:5432/orders?sslmode=disable
  redis:
    addr: localhost:6379

thresholds:
  max_p95_latency_ms: 500
  max_error_rate_percent: 1.0
  safety_factor: 0.70

scenarios:
  - name: orders
    weight: 100
    flow:
      - get: /api/orders
```

**What it does & output:**
Scrapes Docker cgroup metrics, PostgreSQL pool connections, and Redis memory every 500ms without installing any agent inside your container:

```
[INFERRED BOTTLENECK]
  Limiting Resource:         [CRITICAL] API CPU Saturation (88.4% utilization)
  Diagnostic Evidence:       cgroup CPU quota throttled; latency doubled from 45ms to 92ms

[RECOMMENDED SIZING]
  Safe Production Load:      ~35 concurrent users (using 0.70 safety factor / 30% operational headroom)
  Candidate Starting Tier:   Compute-Optimized Tier
    • API Container Sizing:  2.0 vCPU, 1 GB RAM (Peak CPU: 88.4%, RAM: 142 MB)
    • PostgreSQL Sizing:     100 max connections (Peak: 42 connections)
```

---

### 3. Test Real User Journeys (Auth & Dynamic Payloads)
**Goal:** Simulate full user sessions (Register ➔ Login ➔ Extract Token ➔ Authenticated Queries) instead of static GET endpoints.

**Command:**
```bash
capacitylab run
```

**Config (`capacitylab.yaml`):**
```yaml
scenarios:
  - name: user_checkout_journey
    weight: 100
    steps:
      - name: Register
        method: POST
        path: /auth/register
        body: '{"email":"user_{{uuid}}@example.com","password":"secret"}'
        expect:
          status: 201

      - name: Login
        method: POST
        path: /auth/login
        body: '{"email":"user_{{uuid}}@example.com","password":"secret"}'
        extract:
          jwt: "data.token"       # Extracts response field into ${jwt}
        expect:
          status: 200

      - name: Protected Dashboard
        method: GET
        path: /api/dashboard
        headers:
          Authorization: "Bearer ${jwt}"
        expect:
          status: 200
```

**What it does:**
Maintains isolated session state per virtual user. Automatically generates random values via `{{uuid}}`, `{{timestamp}}`, and `{{random_int:min:max}}`, and extracts response tokens to authenticate subsequent requests.

---

### 4. Soak / Endurance Test (Detect Memory Leaks)
**Goal:** Maintain steady load over 10m–1h to detect slow memory leaks, connection pool exhaustion, or cache degradation.

**Command:**
```bash
capacitylab run --config soak.yaml
```

**Config (`soak.yaml`):**
```yaml
workload:
  type: soak            # Holds constant load for the full duration
  start_users: 50       # Steady concurrent virtual users
  duration: 30m         # Endurance run duration
```

**What it does:**
Holds a constant 50 VUs for 30 minutes, monitors memory growth across intervals, and alerts if memory climbs continuously without stabilizing.

---

### 5. Spike / Flash-Crowd Test
**Goal:** Verify backend elasticity and error recovery when traffic surges suddenly (e.g., product drops or flash sales).

**Command:**
```bash
capacitylab run --config spike.yaml
```

**Config (`spike.yaml`):**
```yaml
workload:
  type: spike
  start_users: 10       # Baseline load (10 users)
  max_users: 200        # Instant surge to 200 users
  step_duration: 1m     # Duration of the surge
```

**What it does:**
Executes a baseline warmup, shocks the application with a 20x traffic surge, and drops back to baseline to evaluate queue draining and recovery time.

---

### 6. Hardware Sizing Matrix Sweep
**Goal:** Test multiple CPU and RAM combinations automatically to find the best price-to-performance tier.

**Command:**
```bash
capacitylab run --matrix
```

**Config (`capacitylab.yaml`):**
```yaml
services:
  api:
    container: my-api
    test_matrix:
      cpu: ["0.5", "1.0", "2.0"]
      memory: ["512MB", "1GB", "2GB"]
```

**What it does:**
Iterates through each hardware tier, benchmarks sustainable capacity per spec, and prints a comparative matrix identifying where you hit diminishing returns.

---

### 7. CI/CD Performance Regression Gate
**Goal:** Automatically fail pull requests if an application change degrades capacity or latency by more than 10%.

**Command:**
```bash
capacitylab run --fail-on-regression --output-json results.json
```

**GitHub Actions (`.github/workflows/capacity.yml`):**
```yaml
name: Capacity Check
on: [pull_request]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: docker compose up -d

      - name: Run Capacity Regression Gate
        run: capacitylab run --fail-on-regression --output-json results.json

      - name: Upload HTML Report
        uses: actions/upload-artifact@v4
        if: always()
        with:
          name: capacity-report
          path: capacity-report.html
```

**What it does:**
Compares current capacity against the baseline in `~/.capacitylab/history/`. If throughput drops >10% or p95 latency degrades >10%, exits with code `1`:

```
✖ REGRESSION DETECTED: Sustainable capacity dropped by 18.5% (threshold: 10%)
  Previous: 120 VUs  ➔  Current: 98 VUs
exit status 1
```

---

## CLI Utilities & Workflows

### Pre-Flight Config Validation
Test your config file syntax and verify Docker daemon / database connectivity before launching load:
```bash
capacitylab validate
```

### Scripted / Headless Initialization
Initialize a config without interactive prompts (ideal for scripting):
```bash
capacitylab init --type api --url https://api.example.com --yes
capacitylab init --type docker --auth --yes
```

### Live Real-Time Dashboard
Stream live latency curves, RPS, and container telemetry in your browser during a benchmark:
```bash
capacitylab dashboard --open
```

### Compare Past Benchmark Runs
Diff any two historical runs (or omit IDs to compare the last two):
```bash
# View list of saved runs
capacitylab history

# Diff two runs
capacitylab compare run-1718000100 run-1718005200
```
```
Metric               Base (run-1718000100)   Current (run-1718005200)   Delta
Max Capacity         150 VUs                 180 VUs                    +20.0%  ✔
P95 Latency          142.4ms                 118.1ms                    -17.1%  ✔
Throughput (RPS)     245.2                   298.6                      +21.8%  ✔
Primary Bottleneck   API CPU Saturation      API CPU Saturation         Same
```

### Re-Generate HTML Report From History
Open an earlier benchmark report without re-running traffic:
```bash
capacitylab report run-1718000100 --open
```

### Shell Autocompletion
Install autocompletion for your shell:
```bash
# Bash
capacitylab completion bash | sudo tee /etc/bash_completion.d/capacitylab > /dev/null

# Zsh
capacitylab completion zsh > "${fpath[1]}/_capacitylab"
```

---

## Command Reference

| Command | Key Flags | What It Does |
| :--- | :--- | :--- |
| `capacitylab init` | `-t, --type`, `-u, --url`, `--auth`, `-y, --yes`, `-f, --force` | Creates `capacitylab.yaml` (interactive wizard or scripted) |
| `capacitylab validate` | `-c, --config` | Validates YAML syntax & checks Docker/DB connectivity |
| `capacitylab run` | `--url`, `--start-users`, `--max-users`, `--step`, `-c, --config` | Executes benchmark and prints terminal verdict |
| `capacitylab run --open` | `-o, --output` | Runs benchmark and automatically opens HTML report |
| `capacitylab run --fail-on-regression` | `--output-json` | Exits with code 1 if capacity drops >10% (for CI/CD) |
| `capacitylab run --matrix` | `-m` | Sweeps CPU/RAM matrix combinations |
| `capacitylab dashboard` | `-p, --port`, `--open` | Starts live SSE streaming dashboard on `localhost:4242` |
| `capacitylab history` | — | Lists all historical benchmark runs from `~/.capacitylab/history/` |
| `capacitylab compare` | `[run-id-1] [run-id-2]` | Compares two runs and displays metric diffs and deltas |
| `capacitylab report` | `[run-id]`, `-o, --output`, `--open` | Regenerates standalone interactive HTML report from past run |
| `capacitylab completion` | `[bash\|zsh\|fish\|powershell]` | Generates shell autocompletion script |
| `capacitylab version` | — | Prints CLI version and build platform |

---

## Development

```bash
# Run test suite with race detector
go test -v -race ./...

# Static analysis
go vet ./...

# Build binary
go build -o capacitylab ./cmd/capacitylab
```

---

## Roadmap

See [ROADMAP.md](ROADMAP.md) for upcoming milestones, including test environment fingerprinting, confidence interval sizing, and multi-framework sandbox templates.

---

## License

Apache 2.0. See [LICENSE](LICENSE).
