<div align="center">

# CapacityLab

**Zero-agent capacity planning CLI for backend APIs and Docker stacks.**

Simulate realistic workloads, detect the exact saturation point, pinpoint bottlenecks, and output production sizing recommendations with multi-cloud cost estimates.

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![CI](https://github.com/Abuaslamtech/capacitylab/actions/workflows/ci.yml/badge.svg)](https://github.com/Abuaslamtech/capacitylab/actions/workflows/ci.yml)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20WSL2-lightgrey)](#)

</div>

---

## Quickstart (60 Seconds)

### Option A: Test an API Right Now (Zero Config)
```bash
capacitylab run --url https://api.example.com --start-users 5 --max-users 50 --open
```

### Option B: Interactive Setup Wizard
```bash
# 1. Interactive setup (generates capacitylab.yaml)
capacitylab init

# 2. Pre-flight check (verifies syntax & network connectivity)
capacitylab validate

# 3. Run benchmark and view interactive HTML report
capacitylab run --open
```

### Option C: Try the Pre-Configured Docker Sandbox
Test CapacityLab in 60 seconds without configuring any backend using the [`examples/quickstart/`](examples/quickstart/) sandbox:
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

## Testing Scenarios: What to Run & What It Does

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

scenarios:
  - name: invoices
    weight: 100
    flow:
      - get: /health
      - get: /invoices
```

**What it does & output:**
Increments virtual users step-by-step. Pinpoints the saturation knee-point where latency surges, calculates safe production capacity, and outputs transparent multi-cloud cost estimates:

```
===============================================================
                    CAPACITYLAB BENCHMARK VERDICT              
===============================================================

Target Workload:           50 concurrent users
Max Observed Capacity:     25 concurrent users
Safe Production Load:      ~17 users (30% safety buffer)

Primary Bottleneck:        Downstream Latency Saturation
Sizing Rationale:          Calculated from sustained traffic ceiling of 28.3 RPS.

Recommended Production Sizing (Small Tier - with 30% safety buffer)
  • Target Host:     1.0 vCPU, 1 GB RAM (Estimated from traffic ceiling)

Multi-Cloud Cost Estimates:
  ➔ Budget VPS:      $6–$10/mo (Hetzner CX22 / DO 1GB)
  ➔ Managed PaaS:    $14–$21/mo (Render Starter Web + DB / Fly.io)
  ➔ Hyperscaler:     $25–$45/mo (AWS t4g.small + RDS db.t4g.micro)

Interactive report: capacity-report.html
```

---

### 2. Right-Size a Local Docker Stack
**Goal:** Measure exact container CPU and memory requirements to set optimal resource limits in Docker Compose or Kubernetes.

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

scenarios:
  - name: orders
    weight: 100
    flow:
      - get: /api/orders
```

**What it does & output:**
Scrapes Docker cgroup metrics, PostgreSQL pool connections, and Redis memory every 500ms without installing any agent inside your container:

```
Primary Bottleneck:        API CPU Saturation (88.4% utilization)

Recommended Production Sizing (Compute-Optimized Tier - with 30% safety buffer)
  • API Container:   2.0 vCPU, 1 GB RAM (Peak CPU: 88.4%)
  • PostgreSQL:      100 max connections (Peak: 42 connections)
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

## All Commands & Flags Reference

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

## License

Apache 2.0 — see [LICENSE](LICENSE).
