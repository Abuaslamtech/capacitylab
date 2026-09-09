# CapacityLab (`capacitylab`)

> **Developer Infrastructure & Capacity Planning Engine**  
> *Run your backend locally, simulate realistic workloads, measure real container resource consumption, find the exact breaking point, and estimate the production infrastructure needed before you deploy.*

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Status: Specification & Plan](https://img.shields.io/badge/Status-Spec%20%26%20Plan-success.svg)](#)
[![Go Version](https://img.shields.io/badge/Go-1.22%2B-cyan.svg)](https://go.dev/)

---

## Table of Contents

1. [Product Vision & Core Promise](#1-product-vision--core-promise)
2. [Market Positioning: Why Existing Tools Fall Short](#2-market-positioning-why-existing-tools-fall-short)
3. [Product Specifications & MVP Requirements](#3-product-specifications--mvp-requirements)
4. [Architecture & System Design](#4-architecture--system-design)
5. [Configuration Specification (`capacitylab.yaml`)](#5-configuration-specification-capacitylabyaml)
6. [Critical Engineering Traps & Battle-Tested Fixes](#6-critical-engineering-traps--battle-tested-fixes)
7. [Saturation Detection & Bottleneck Analysis Engine](#7-saturation-detection--bottleneck-analysis-engine)
8. [Resource Sizing & Recommendation Formula](#8-resource-sizing--recommendation-formula)
9. [User Experience: Terminal, HTML Report & Dashboard](#9-user-experience-terminal-html-report--dashboard)
10. [Step-by-Step Implementation Roadmap](#10-step-by-step-implementation-roadmap)
11. [Verification & Acceptance Criteria for V1](#11-verification--acceptance-criteria-for-v1)

---

## 1. Product Vision & Core Promise

### The Problem
When deploying an application to a VPS (Hetzner, DigitalOcean), PaaS (Render, Fly.io, Railway), or container orchestrator (AWS ECS, Google Cloud Run, Kubernetes), developers are forced to answer:

> *"How many vCPUs and gigabytes of RAM does my backend and database actually need to sustain 500 or 5,000 peak concurrent users?"*

Today, 95% of developers either:
1. **Wildly overprovision** (paying $80/month for a 4 vCPU / 8 GB instance when a $7/month 1 vCPU instance would suffice), or
2. **Underprovision and crash on launch day** because of unexpected database connection pool starvation or CPU throttling.

### The Solution: CapacityLab
CapacityLab is an open-source, CLI-first developer tool with a zero-dependency interactive HTML report and local web dashboard. It bridges the gap between **load generation**, **container resource isolation**, and **automated infrastructure capacity estimation**.

```text
Install CLI
   ↓
capacitylab init (generate capacitylab.yaml)
   ↓
capacitylab run
   ↓
[Spins up Docker containers with controlled cgroups: 0.5 CPU / 512 MB, 1 CPU / 1 GB, etc.]
   ↓
[Runs realistic weighted user journeys: Auth -> Dashboard -> Record Sale]
   ↓
[Scrapes container cgroups, Postgres stats, Redis info, latency percentiles]
   ↓
[Detects saturation point & primary bottleneck]
   ↓
[Outputs terminal verdict + generates standalone interactive capacity-report.html]
```

### The Output
```text
Backend Capacity Benchmark Complete
─────────────────────────────────────────────────────────────
Target Workload:               1,000 concurrent users
Maximum Sustainable Capacity:  720 concurrent users (with 30% safety headroom)
Absolute Saturation Point:     940 concurrent users

Primary Bottleneck:            PostgreSQL Connection Pool & CPU
Secondary Bottleneck:          API CPU Throttling
Non-Limiting Services:         Redis, Network Bandwidth, Disk I/O

Recommended Production Configuration:
  • API Service:      2 vCPU,  1 GB RAM  (Expected peak load: 58% CPU)
  • PostgreSQL:       1 vCPU,  2 GB RAM  (Recommended max_connections: 50 + PgBouncer)
  • Redis:            0.25 vCPU, 256 MB RAM

Interactive Report: file:///path/to/capacity-report.html
```

---

## 2. Market Positioning: Why Existing Tools Fall Short

| Dimension | Load Testers (k6, Locust, wrk) | APM Tools (Datadog, Grafana) | Enterprise Cloud Optimizers (StormForge, Akamas) | **CapacityLab** |
| :--- | :--- | :--- | :--- | :--- |
| **Primary Output** | RPS, latency percentiles, error rates | Raw metric charts, traces, log streams | Autonomous cloud cluster tuning | **Actionable CPU/RAM recommendations & bottleneck diagnosis** |
| **System Visibility** | Resource-blind (external black box) | Passive metric observation | Active cloud cluster changes | **Closed-loop container cgroup & DB internal profiling** |
| **Target Environment** | Any HTTP endpoint | Running production / staging | Live Kubernetes clusters | **Local developer machine (`localhost` / Docker)** |
| **Cost & Friction** | Free / Open Source | $50–$300+/month, high config | $30,000+/year enterprise SaaS | **100% Free, Local, Open Source** |
| **Answers "What size server do I buy?"** | ❌ No | ❌ No (requires manual human analysis) | ✅ Yes (in live Kubernetes only) | **✅ Yes (locally before spending any money)** |

---

## 3. Product Specifications & MVP Requirements

### Target Personas
* **Backend & Full-Stack Developers:** Sizing API instances and databases before shipping.
* **Indie Hackers & Startups:** Optimizing infrastructure cost vs. reliability.
* **DevOps / Platform Engineers:** Defining accurate baseline CPU/memory `requests` and `limits` for Docker Compose or Kubernetes manifests.

### MVP Scope (The 7 Core Deliverables)
1. **Multi-Step User Journey Scenarios:** Define realistic workflows (e.g. login $\rightarrow$ browse $\rightarrow$ mutate state $\rightarrow$ fetch summary) with stage weights and think-times.
2. **Progressive Step-Ramping Load Engine:** Ramp concurrent virtual users (VUs) or RPS systematically (e.g., 10 $\rightarrow$ 25 $\rightarrow$ 50 $\rightarrow$ 100 $\rightarrow$ 250 $\rightarrow$ 500 $\rightarrow$ 1000).
3. **Controlled Docker Resource Isolation:** Launch or update target containers under strict cgroup constraints (e.g., 0.5 CPU, 1 CPU, 2 CPU; 512 MB, 1 GB, 2 GB).
4. **Deep Metrics Harvesting:**
   * **Host/App:** CPU usage %, cgroup throttling count (`nr_throttled`), Resident Set Size (RSS) memory, open file descriptors, network bytes, p50/p95/p99 latency, HTTP status codes.
   * **PostgreSQL:** Active connections, connection pool waiting queries, transaction commit/rollback rate, cache hit ratio (`blks_hit / (blks_hit + blks_read)`), lock wait count.
   * **Redis:** Memory used, instantaneous ops/sec, connected clients, hit/miss ratio, memory evictions.
5. **Saturation Detection:** Detect breaking points deterministically when thresholds (CPU > 85%, p95 > 250ms, error rate > 1%, DB connection exhaustion) are breached.
6. **Bottleneck Root-Cause Engine:** Automatically rank which component degraded first (API CPU vs. DB locks vs. memory leak vs. connection pool).
7. **Dual-Surface Reporting:** Instant terminal summary table + a standalone, interactive, single-file HTML report (`capacity-report.html`) containing multi-variable time-series curves.

---

## 4. Architecture & System Design

CapacityLab is built in **Go** as a single static binary. It interacts with the local Docker daemon via the official Docker Go SDK and embeds an engine for load generation, metrics harvesting, and analytics.

```text
                                  ┌───────────────────────────┐
                                  │   capacitylab.yaml        │
                                  └─────────────┬─────────────┘
                                                │
                                                ▼
                                  ┌───────────────────────────┐
                                  │      CLI Controller       │
                                  │  (init, validate, run)    │
                                  └─────────────┬─────────────┘
                                                │
              ┌─────────────────────────────────┼─────────────────────────────────┐
              ▼                                 ▼                                 ▼
   ┌────────────────────┐            ┌────────────────────┐            ┌────────────────────┐
   │   Runtime Engine   │            │    Load Engine     │            │  Telemetry Engine  │
   │  (Docker SDK)      │            │  (k6 / Native Go)  │            │  (cgroups & DBs)   │
   ├────────────────────┤            ├────────────────────┤            ├────────────────────┤
   │ • cgroup limits    │            │ • Multi-stage VUs  │            │ • Docker stats API │
   │ • CPU quota/period │            │ • Weighted journeys│            │ • cpu.stat throttle│
   │ • Memory max/swap  │            │ • Dynamic payloads │            │ • Postgres pg_stat │
   │ • Health checks    │            │ • Latency tagging  │            │ • Redis INFO stats │
   └─────────┬──────────┘            └─────────┬──────────┘            └─────────┬──────────┘
             │                                 │                                 │
             └─────────────────────────────────┼─────────────────────────────────┘
                                               │
                                               ▼
                                  ┌───────────────────────────┐
                                  │     Analytics Core        │
                                  ├───────────────────────────┤
                                  │ • Knee-point detection    │
                                  │ • Saturation evaluator    │
                                  │ • Bottleneck classifier   │
                                  │ • Sizing recommender      │
                                  └─────────────┬─────────────┘
                                                │
                                ┌───────────────┴───────────────┐
                                ▼                               ▼
                     ┌─────────────────────┐         ┌─────────────────────┐
                     │   Terminal Output   │         │ Interactive HTML &  │
                     │  (ANSI / TUI Table) │         │ Local Web Dashboard │
                     └─────────────────────┘         └─────────────────────┘
```

### Component Breakdown
* **`cmd/capacitylab`**: CLI interface built with `cobra`.
* **`internal/config`**: YAML loader, schema validator, and default interpolator.
* **`internal/runtime`**: Docker engine client wrapper; sets container cgroup limits, monitors container lifecycle, and manages networks.
* **`internal/load`**: Load generation abstraction (bundles a native headless runner with optional k6 adapter).
* **`internal/monitor`**: Scrapers for cgroups v2 (`cpu.stat`, `memory.current`, `throttled_usec`), PostgreSQL internal catalogs (`pg_stat_activity`, `pg_stat_database`), and Redis `INFO`.
* **`internal/analyzer`**: Deterministic bottleneck locator, saturation detection algorithms, and safety margin calculations.
* **`internal/report`**: Terminal table generator and self-contained HTML report builder (bundles Chart.js/uPlot inline).
* **`internal/dashboard`**: Lightweight embedded HTTP server (`net/http`) for historical report exploration.

---

## 5. Configuration Specification (`capacitylab.yaml`)

```yaml
version: "1"

# 1. Target Application Definition
application:
  name: billbolt-backend
  url: http://localhost:3000
  startup:
    compose_file: ./docker-compose.yml
    wait_for_health: true
    timeout: 60s

# 2. Controlled Service Limits to Profile
services:
  api:
    container: billbolt-api
    test_matrix:
      cpu: ["0.5", "1.0", "2.0"]
      memory: ["512MB", "1GB", "2GB"]

  postgres:
    container: billbolt-postgres
    dsn: "postgres://postgres:postgres@localhost:5432/billbolt?sslmode=disable"
    max_connections: 100

  redis:
    container: billbolt-redis
    addr: "localhost:6379"

# 3. Workload Ramp & Stage Strategy
workload:
  type: step-ramp
  start_users: 10
  max_users: 1000
  step: 50
  step_duration: 45s
  warmup_duration: 20s
  pacing:
    think_time_min: 200ms
    think_time_max: 800ms

# 4. Saturation & SLA Thresholds (Breaking Criteria)
thresholds:
  max_cpu_percent: 85
  max_memory_percent: 80
  max_p95_latency_ms: 250
  max_error_rate_percent: 1.0
  max_cpu_throttle_percent: 15

# 5. Realistic Weighted User Journeys
scenarios:
  - name: browse_dashboard
    weight: 40
    flow:
      - get: /api/v1/auth/me
      - get: /api/v1/dashboard/metrics
      - get: /api/v1/notifications

  - name: record_sale_transaction
    weight: 35
    flow:
      - get: /api/v1/inventory?limit=20
      - post:
          path: /api/v1/sales
          body:
            customer_id: "{{uuid}}"
            items: [{ product_id: "prod-1", quantity: 2 }]
      - get: /api/v1/sales/{{response.body.sale_id}}/receipt

  - name: generate_inventory_report
    weight: 25
    flow:
      - get: /api/v1/reports/inventory?period=30d
```

---

## 6. Critical Engineering Traps & Battle-Tested Fixes

Benchmarking servers on a developer's local machine is notorious for false readings. CapacityLab is engineered specifically to eliminate these 6 failure modes:

```text
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                        LOCAL BENCHMARKING FAILURE MODES                                │
├────────────────────────────┬─────────────────────────────┬─────────────────────────────┤
│ 1. Machine Interference    │ 2. Docker Desktop VM Skew   │ 3. Cold Cache Distortion    │
│ Load runner steals CPU     │ macOS/Win VM limits host    │ Unwarmed DB buffers cause   │
│ from target containers     │ perception of cores         │ artificial early spikes     │
├────────────────────────────┼─────────────────────────────┼─────────────────────────────┤
│ 4. Localhost Socket Limits │ 5. Client Pool vs DB Server │ 6. GC Latency Conflation    │
│ TIME_WAIT port exhaustion  │ App pool exhausted while    │ Minor runtime GC pauses     │
│ falsely masquerades as 500s│ DB server CPU is idling     │ mistaken for DB bottleneck  │
└────────────────────────────┴─────────────────────────────┴─────────────────────────────┘
```

### Trap 1: The Same-Machine Interference Trap
* **The Symptom:** At 800 virtual users, the load generator itself consumes 4 CPU cores on the laptop. The backend starves for CPU cycles, leading to artificial latency spikes and invalid failure reports.
* **The Fix:**
  1. **CPU Pinning (`cpuset`):** CapacityLab pins target application containers to designated logical cores (e.g. cores 0–3) using Docker's `--cpuset-cpus` and assigns the load generator to remaining cores (e.g. cores 4–7).
  2. **Self-Overhead Sentinel:** The engine continuously monitors its own process CPU and memory consumption. If the load generator consumes $> 35\%$ of total host resources, a `LOAD_GENERATOR_SATURATION` warning is triggered, preventing false backend blame.
  3. **cgroup Throttling Counter:** Instead of relying merely on host CPU %, CapacityLab reads `/sys/fs/cgroup/cpu.stat` (`nr_throttled` and `throttled_usec`). If the container is throttled by Docker while host CPU is available, it proves the container hit its specific limit, not host starvation.

### Trap 2: The Docker Desktop Virtualization Trap (macOS & Windows)
* **The Symptom:** On Apple Silicon or Windows (WSL2), Docker runs inside a lightweight Linux hypervisor. An app configured for "2 vCPUs" might be constrained by the global 4 vCPU ceiling of the entire VM, or thermal throttling on a laptop might reduce clock speeds mid-test.
* **The Fix:**
  1. **Hypervisor Topology Detection:** CapacityLab inspects the Docker daemon info on startup. If running under Docker Desktop, it detects the VM's assigned CPU and memory allocation.
  2. **Normalized cgroup Quota:** Limits are enforced via CFS (Completely Fair Scheduler) period/quota calculations (`cpu.cfs_quota_us` / `cpu.cfs_period_us`) relative to assigned container limits, making results reproducible regardless of host VM size.
  3. **Host Headroom Check:** If Docker Desktop's VM memory exceeds 85% utilization across all background containers, the test pauses with an actionable alert to increase Docker Desktop's resource allocations.

### Trap 3: Cold Cache vs. Warm Cache Skew
* **The Symptom:** The first 50 users hit cold Postgres `shared_buffers`, un-cached Redis keys, and un-JITed runtime code, showing high p95 latency. At 250 users, the database is warm and latency drops. This produces an inverted, inaccurate capacity curve.
* **The Fix:**
  1. **Mandatory Warm-up Phase:** Before recording benchmark statistics, CapacityLab runs a configured warm-up stage (e.g. 20s at 10% load).
  2. **Cache Hit Ratio Verification:** CapacityLab checks Postgres `blks_hit / (blks_hit + blks_read)` and Redis `keyspace_hits / (keyspace_hits + keyspace_misses)`. The recording phase only begins once cache metrics stabilize.

### Trap 4: Localhost Ephemeral Port / Socket Exhaustion
* **The Symptom:** At 500+ requests per second on `localhost`, the OS runs out of ephemeral TCP ports. Requests fail with `connection reset by peer` or `bind: address already in use` (`TIME_WAIT` pileup), which looks like an application crash.
* **The Fix:**
  1. **HTTP Keep-Alive & Connection Pooling:** The load engine enforces aggressive HTTP connection reuse (`MaxIdleConns` / `KeepAlive: 60s`).
  2. **Synthetic Socket Health Monitor:** The CLI tracks open TCP sockets in `TIME_WAIT` and `CLOSE_WAIT` states on the loopback adapter. If exhaustion approaches 80% of `net.ipv4.ip_local_port_range`, connections are automatically throttled or switched to Unix Domain Sockets where supported.

### Trap 5: Client Connection Pool Exhaustion vs. Database Server Saturation
* **The Symptom:** An API starts returning 500s or 504s. A naive tool reports "Database is down". In reality, the database server is sitting at 5% CPU with 95 available connections, but the API's internal connection pool was hard-coded to a maximum of 10 connections.
* **The Fix:**
  1. **Correlated Query Probing:** CapacityLab checks Postgres `pg_stat_activity` directly. If the active connection count equals the client pool ceiling while `pg_stat_database` shows low query execution times, CapacityLab explicitly diagnoses:
     `Bottleneck: API-side Database Connection Pool Exhausted (Current pool: 10). Database server has 90 idle slots.`

### Trap 6: Garbage Collection (GC) Latency Conflation
* **The Symptom:** Node.js, Go, or JVM runtimes experience periodic stop-the-world GC pauses, creating isolated p99 latency spikes that do not reflect true throughput saturation.
* **The Fix:**
  1. **Knee-Point Smoothing:** Latency degradation is evaluated over a continuous 10-second sliding window rather than single transient outliers.
  2. **Memory Growth Correlation:** If p99 spikes coincide with an immediate steep drop in container memory RSS (classic major GC garbage collection), the event is tagged as runtime GC pressure, not database or infrastructure failure.

---

## 7. Saturation Detection & Bottleneck Analysis Engine

CapacityLab uses a deterministic multi-stage classification matrix to diagnose exactly what broke and why:

```text
                     Workload Step Complete
                               │
                               ▼
                    Did SLA Breach Occur?
                    (p95 > 250ms, Errors > 1%, CPU > 85%)
                               │
                ┌──────────────┴──────────────┐
             No │                             │ Yes
                ▼                             ▼
       Increment Workload          Analyze Telemetry Matrix
                                              │
       ┌────────────────────────┬─────────────┴──────────┬────────────────────────┐
       ▼                        ▼                        ▼                        ▼
[API CPU Throttled]     [PG CPU / Lock Spike]    [Pool Count == Max]     [RSS Memory > 90%]
       │                        │                        │                        │
       ▼                        ▼                        ▼                        ▼
 Primary Bottleneck:      Primary Bottleneck:      Primary Bottleneck:      Primary Bottleneck:
    API CPU Core          PostgreSQL Queries       DB Client Pool Size       Memory Leak / OOM
```

### Deterministic Rule Matrix

| Condition | Diagnostic Output | Actionable Remediation |
| :--- | :--- | :--- |
| `API cgroup throttled_usec > 15%` & `DB CPU < 50%` | **API CPU Saturation** | Increase API container vCPU or optimize CPU-bound route handlers. |
| `Postgres CPU > 80%` & `pg_stat_activity active > 80%` | **PostgreSQL CPU Saturation** | Missing indexes, unoptimized queries, or insufficient database CPU. |
| `API 5xx errors spike` & `Postgres connections == max_connections` | **Database Pool Exhaustion** | Increase pool size or introduce a connection pooler (e.g. PgBouncer). |
| `Container Memory monotonically rises without plateau` | **Memory Leak / OOM Risk** | Application fails to release allocations under sustained load. |
| `Redis used_memory > maxmemory` & `evicted_keys > 0` | **Redis Eviction Pressure** | Enlarge Redis memory ceiling or refine TTL / eviction policies. |

---

## 8. Resource Sizing & Recommendation Formula

CapacityLab does not simply output the maximum load before crashing. It computes two distinct figures:

### 1. Maximum Observed Capacity ($C_{\text{max}}$)
The highest concurrent user count / RPS where all SLA thresholds were still strictly met before the subsequent breaking step.

### 2. Recommended Sustainable Capacity ($C_{\text{rec}}$)
In production, running hardware at 95% CPU is unacceptable because unexpected traffic spikes will cause cascading outages. CapacityLab applies a **Target Operating Safety Margin** (default: 30% headroom):

$$C_{\text{rec}} = C_{\text{max}} \times (1 - \text{Headroom}) = C_{\text{max}} \times 0.70$$

### 3. Sizing Optimization Algorithm
To recommend the minimum viable server size for a user's stated target (e.g., "I need to support 1,000 peak users"):
1. The tool tests the lowest configured resource slice (e.g., 0.5 vCPU, 512 MB).
2. If $C_{\text{rec}} < \text{Target}$, it evaluates the next tier in the matrix (e.g. 1.0 vCPU, 1 GB).
3. Once $C_{\text{rec}} \ge \text{Target}$, that tier is marked **Recommended**.
4. Higher tiers that exceed the target by $> 100\%$ are flagged as **Over-Provisioned (Wasted Cost)**.

```text
Target: 1,000 Concurrent Users

Matrix Evaluation:
  • 0.5 vCPU / 512 MB  ───► Capacity: 210 users  [❌ Insufficient]
  • 1.0 vCPU / 1 GB    ───► Capacity: 520 users  [❌ Insufficient]
  • 2.0 vCPU / 1 GB    ───► Capacity: 980 users  [⚠️ Marginal (98% of target)]
  • 2.0 vCPU / 2 GB    ───► Capacity: 1,350 users [✅ Recommended (35% safety buffer)]
  • 4.0 vCPU / 4 GB    ───► Capacity: 2,800 users [💰 Over-provisioned]
```

---

## 9. User Experience: Terminal, HTML Report & Dashboard

CapacityLab adopts the hybrid UX model perfected by Lighthouse and Playwright:

### Surface 1: Terminal Execution View
During the test, a high-density, flicker-free terminal UI displays live telemetry:

```text
CapacityLab v0.1.0 — Running Benchmark [billbolt-backend]
Target: 1,000 VUs | Matrix: API [1 vCPU, 1GB] | DB [1 vCPU, 2GB]

Stage 1: Warmup (20s)                    [DONE] 50 VUs  — Cache hit: 98.4%
Stage 2: 100 VUs                         [PASS] p95: 42ms  | CPU: 18% | DB CPU: 12%
Stage 3: 250 VUs                         [PASS] p95: 58ms  | CPU: 34% | DB CPU: 26%
Stage 4: 500 VUs                         [PASS] p95: 112ms | CPU: 64% | DB CPU: 51%
Stage 5: 750 VUs                         [PASS] p95: 168ms | CPU: 82% | DB CPU: 74%
Stage 6: 1,000 VUs                       [FAIL] p95: 412ms | CPU: 96% | DB CPU: 94%

[!] Saturation Breached at 880 VUs:
    • API CPU exceeded threshold (96% > 85%)
    • p95 latency breached SLA (412ms > 250ms)
    • PostgreSQL active query locks spiked to 14

Writing report to: ./capacity-report.html
```

### Surface 2: Standalone Interactive HTML Report (`capacity-report.html`)
Generated automatically on completion:
* **Zero Dependencies:** Pure self-contained HTML/CSS with embedded vector charts. Double-click to open in any browser without running a server.
* **Synchronized Scrubbing:** Hovering over a latency spike at `T+3m45s` highlights the exact CPU throttle event, memory usage, and Postgres active query count at that exact second.
* **Bottleneck Deep-Dive Card:** Displays actionable suggestions (e.g. index recommendations, connection pool adjustments).
* **Exportable:** Easily zipped, committed to version control, or attached to a pull request.

### Surface 3: Local Live Dashboard (`capacitylab dashboard`)
Spins up a lightweight local web server (`localhost:4242`):
* Allows developers to view real-time streaming charts over WebSockets while tests execute.
* Stores historical benchmark runs in `~/.capacitylab/history/`.
* Side-by-side comparative diffs: Compare `Run #12 (1 vCPU)` against `Run #13 (2 vCPU)` to see exact performance gains per dollar.

---

## 10. Step-by-Step Implementation Roadmap

### Phase 1: Core Foundation & Configuration Engine
* [ ] Initialize Go project structure (`cmd/capacitylab`, `internal/...`).
* [ ] Implement `capacitylab.yaml` parser, validator, and schema definitions.
* [ ] Implement `capacitylab init` to scaffold sample configuration and Compose files.
* [ ] Implement `capacitylab validate` to verify Docker connectivity and endpoint health.

### Phase 2: Docker Runtime & cgroup Profiler
* [ ] Integrate Docker Go SDK (`github.com/docker/docker/client`).
* [ ] Implement dynamic container resource configuration (updating CPU quota/period and memory limits).
* [ ] Implement cgroup v2 metrics scraper reading `/sys/fs/cgroup` (`cpu.stat`, `memory.current`, `throttled_usec`).
* [ ] Implement host environment detector (Linux native vs Docker Desktop on macOS/Windows vs WSL2).

### Phase 3: Load Engine & Scenario Runner
* [ ] Build workload driver supporting progressive step-ramping (VUs & RPS).
* [ ] Implement scenario executor for multi-step weighted user journeys (GET, POST, dynamic JSON extraction).
* [ ] Build connection pool and socket manager to prevent local `TIME_WAIT` ephemeral port exhaustion.
* [ ] Implement warm-up stabilization phase before recording metrics.

### Phase 4: Database Deep Telemetry (PostgreSQL & Redis)
* [ ] Implement PostgreSQL collector:
  * Connection pool utilization (`pg_stat_activity`).
  * Cache hit ratio (`pg_stat_database`).
  * Longest running active queries and lock contention.
* [ ] Implement Redis collector:
  * Memory consumption, instantaneous ops/sec, client count, cache hit/miss ratio (`INFO`).
* [ ] Design generic `DatabaseMonitor` interface to permit future database adapters (MySQL, MongoDB).

### Phase 5: Analytics, Saturation & Recommendation Engine
* [ ] Build SLA monitor checking CPU, RAM, p95/p99 latency, and error rate thresholds.
* [ ] Implement knee-point detection algorithm to identify inflection points where latency diverges.
* [ ] Implement root-cause bottleneck classifier (ranking API CPU, DB CPU, DB connections, RAM).
* [ ] Implement sizing calculation algorithm ($C_{\text{max}}$, $C_{\text{rec}}$, and recommended hardware tier).

### Phase 6: Reporting & Visualization
* [ ] Build rich terminal output with live progress bars and summary tables.
* [ ] Build single-file interactive HTML report generator with embedded, interactive time-series charts.
* [ ] Implement `capacitylab report` command to regenerate reports from raw JSON test runs.
* [ ] Implement `capacitylab dashboard` local web server to browse and compare historical runs.

---

## 11. Verification & Acceptance Criteria for V1

To guarantee real-world utility, the V1 implementation must be validated against a realistic reference backend:

| Test Scenario | Validation Criteria |
| :--- | :--- |
| **Reference Backend** | A standard REST application (Node.js/NestJS or Go) with a PostgreSQL database and Redis cache running via Docker Compose. |
| **Controlled CPU Bottleneck** | Artificially constrain API container to 0.5 vCPU. CapacityLab must flag **"Primary Bottleneck: API CPU Saturation"** and identify the exact saturation point. |
| **Controlled DB Pool Bottleneck** | Set API connection pool to 5 while Postgres supports 100. CapacityLab must specifically report **"Database Connection Pool Starvation"** rather than generic database failure. |
| **Controlled DB Query Bottleneck** | Introduce a missing index on a 100,000 row table queried in a scenario. CapacityLab must flag **"PostgreSQL Query Latency Bottleneck"** with high database CPU. |
| **Recommendation Accuracy** | For a stated target of 500 VUs, CapacityLab must successfully identify the exact lowest resource tier (e.g. 1 vCPU / 1 GB) that satisfies the target with $\ge 25\%$ safety headroom. |
| **Zero Host Contamination** | The test run must not fail due to ephemeral port exhaustion or load generator CPU competition. |

---

## License

CapacityLab is licensed under the [Apache 2.0 License](LICENSE).
