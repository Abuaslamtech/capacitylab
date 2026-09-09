# CapacityLab Architecture & Technical Decisions

> **Build vs. Use Strategy, Component Boundaries, and Architectural Rationale**

This document details the architectural decisions for **CapacityLab**, outlining which existing open-source libraries and services are leveraged, which core components are built from scratch, and why.

---

## 1. Core Architectural Principle

> **"Never reinvent container runtimes, HTTP protocol engines, or charting engines. Dedicate 100% of engineering effort to the 'Brain': metric correlation, saturation detection, root-cause bottleneck classification, and infrastructure recommendation heuristics."**

CapacityLab is distributed as a **single, self-contained static binary** (written in Go) with zero required external dependencies on the host machine other than Docker.

```text
┌────────────────────────────────────────────────────────────────────────┐
│                        CAPACITYLAB ARCHITECTURE                        │
├───────────────────────────────────┬────────────────────────────────────┤
│       WHAT WE USE (LEVERAGED)     │       WHAT WE BUILD (CUSTOM)       │
├───────────────────────────────────┼────────────────────────────────────┤
│ • Docker Go SDK (cgroups v2)      │ • Local Benchmarking Sentinel      │
│ • k6 / Embedded Go HTTP Pool      │ • Saturation & Knee-Point Engine   │
│ • Native Database Clients (pgx)   │ • Bottleneck Classifier            │
│ • Cobra CLI & Lipgloss ANSI UI    │ • Capacity Sizing Recommender      │
│ • uPlot / Chart.js (inlined JS)   │ • Self-Contained HTML Generator    │
└───────────────────────────────────┴────────────────────────────────────┘
```

---

## 2. Technical Decision Matrix

| Layer | Responsibility | Strategy | Solution / Library | Rationale |
| :--- | :--- | :---: | :--- | :--- |
| **CLI & Flags** | Command routing (`run`, `init`, `validate`, `report`) | **USE** | `github.com/spf13/cobra` | Battle-tested industry standard (used by Kubernetes `kubectl`, GitHub CLI). |
| **Terminal UX** | Progress bars, tables, formatted terminal output | **USE** | `charmbracelet/lipgloss`, `tablewriter` | Lightweight ANSI styling without heavy terminal dependencies. |
| **Container Control** | cgroup v2 limits (`cpu.cfs_quota_us`, `memory.max`) | **USE** | `github.com/docker/docker/client` | Official Docker Go SDK. Full kernel-level container lifecycle and resource management. |
| **Traffic Engine** | HTTP load generation, virtual user scheduling | **USE** | Embedded `k6` wrapper / Native Go HTTP pool | Avoids building complex HTTP/2, TLS handshake, keep-alive, and connection pools from scratch. |
| **PostgreSQL Stats** | Connection pool counts, cache hits, slow queries | **USE** | `github.com/jackc/pgx/v5` | Direct query to Postgres catalogs (`pg_stat_activity`, `pg_stat_database`) over standard SQL connection. No external agent needed. |
| **Redis Stats** | Memory usage, ops/sec, evictions, clients | **USE** | `github.com/redis/go-redis/v9` | Direct inspection of Redis `INFO` over standard connection. |
| **Interactive Charts** | Visual time-series curves in browser report | **USE** | `uPlot` or `Chart.js` (Embedded) | Inlined minified static JS file embedded directly into Go binary via `//go:embed`. Zero network access needed. |
| **Local Host Sentinel** | Prevent test contamination & false bottlenecks | **BUILD** | `internal/sentinel` | Protects against same-machine CPU competition, cgroup quota throttling vs host starvation, and localhost socket exhaustion. |
| **Saturation Engine** | Detect exact inflection points & SLA breaches | **BUILD** | `internal/analyzer/saturation.go` | Automated knee-point detection identifying where latency diverges from linear to exponential. |
| **Bottleneck Classifier**| Pinpoint root causes across API, DB, and network | **BUILD** | `internal/analyzer/bottleneck.go` | Multi-dimensional correlation matrix ranking primary vs secondary constraints. |
| **Sizing Recommender** | Calculate sustainable capacity & cloud tiers | **BUILD** | `internal/recommender` | Applies safety buffers (e.g. 30% headroom) and maps workload to standard server sizes. |
| **Report Bundler** | Generate standalone offline HTML reports | **BUILD** | `internal/report` | Embeds raw metrics into an interactive single-file HTML bundle. |

---

## 3. Deep Dive: What We USE (Existing Services & Libraries)

### A. Container Runtime: Official Docker Go SDK
* **Package:** `github.com/docker/docker/client`
* **Why We Don't Build It:** Docker already manages Linux namespaces, cgroups v2, virtual ethernet bridges, and container lifecycles.
* **How We Use It:**
  1. Inspect running containers defined in `capacitylab.yaml`.
  2. Dynamically adjust CPU quotas (`Resources.CPUQuota`, `Resources.CPUPeriod`) and memory limits (`Resources.Memory`) using `ContainerUpdate`.
  3. Stream container resource metrics using `ContainerStats`.
  4. Query host daemon environment to detect if Docker is running natively on Linux or inside Docker Desktop (macOS/Windows hypervisor).

### B. Traffic Simulation: Headless k6 & Embedded Go HTTP Pool
* **Engine:** `k6` CLI (subprocess mode) + embedded Go HTTP worker fallback.
* **Why We Don't Build It:** A reliable load generator requires:
  * Connection pooling with aggressive keep-alive reuse.
  * Robust TLS handshake caching.
  * Configurable virtual user (VU) scheduling and pacing.
  * Multi-scenario sequencing with dynamic response variable extraction.
  Building this from scratch would turn the project into "yet another load testing tool" rather than a capacity planner.
* **How We Use It:** CapacityLab generates execution scripts from `capacitylab.yaml`, triggers the runner, and captures structured telemetry over a local IPC stream or stdout.

### C. Database Telemetry: Native Client Drivers
* **Packages:** `github.com/jackc/pgx/v5`, `github.com/redis/go-redis/v9`
* **Why We Don't Use Heavy Monitoring Agents (Prometheus / Datadog):**
  * Installing Prometheus node exporters, Datadog agents, or OpenTelemetry collectors on local developer setups introduces massive friction.
  * Instead, CapacityLab opens **a single read-only client connection** to Postgres and Redis to harvest system metrics directly.
* **Metrics Scraped:**
  * **PostgreSQL:**
    ```sql
    -- Active vs Idle Connections
    SELECT state, count(*) FROM pg_stat_activity GROUP BY state;
    
    -- Buffer Cache Hit Ratio
    SELECT blks_hit * 100.0 / NULLIF(blks_hit + blks_read, 0) AS cache_hit_ratio FROM pg_stat_database;
    
    -- Waiting Queries & Locks
    SELECT count(*) FROM pg_locks WHERE NOT granted;
    ```
  * **Redis:** Issued `INFO stats` and `INFO memory` periodically (every 1 second) to inspect `used_memory`, `instantaneous_ops_per_sec`, `connected_clients`, and `evicted_keys`.

### D. Visual Charting: Inlined Zero-Dependency JS
* **Library:** `uPlot` (or `Chart.js`)
* **Why We Don't Use Hosted SaaS or Heavy Web Frameworks:**
  * Developers benchmark proprietary backends locally and do not want to upload database performance stats to an external cloud.
  * A single, standalone HTML file that works completely offline with zero CDN dependencies is portable, private, and permanent.
* **How We Use It:** The minified JS file is bundled into the Go binary at compile time using `//go:embed`. When `capacitylab run` completes, it renders a standalone `capacity-report.html` with data baked in as JSON.

---

## 4. Deep Dive: What We BUILD (The Proprietary Engine)

The custom software inside CapacityLab represents the **"Brain"**—the logic that transforms raw numbers into infrastructure decisions:

```text
                               ┌────────────────────────┐
                               │   Scraped Telemetry    │
                               │  (Docker + DBs + k6)   │
                               └───────────┬────────────┘
                                           │
                                           ▼
                               ┌────────────────────────┐
                               │  1. SENTINEL FILTER    │
                               │  (Filter Host Noise)   │
                               └───────────┬────────────┘
                                           │ Clean Data
                                           ▼
                               ┌────────────────────────┐
                               │  2. KNEE-POINT ENGINE  │
                               │  (Detect Saturation)   │
                               └───────────┬────────────┘
                                           │ Inflection Points
                                           ▼
                               ┌────────────────────────┐
                               │ 3. BOTTLENECK RANKER   │
                               │  (Classify Root Cause) │
                               └───────────┬────────────┘
                                           │ Ranked Bottlenecks
                                           ▼
                               ┌────────────────────────┐
                               │ 4. SIZING RECOMMENDER  │
                               │  (Hardware Tiers)      │
                               └───────────┬────────────┘
                                           │
                                ┌──────────┴──────────┐
                                ▼                     ▼
                       Terminal Verdict        HTML Interactive
```

### Component 1: Local Benchmarking Sentinel (`internal/sentinel`)
Running benchmarks on a developer's local machine introduces noise that invalidates test results. The Sentinel actively monitors and prevents:
1. **Load Generator Competition:** Monitors the load generator's own CPU/RAM. If it consumes $>35\%$ of total host cores, the Sentinel raises a `RUNNER_OVERHEAD_WARNING` to prevent misidentifying host saturation as an API bottleneck.
2. **Docker cgroup Throttling vs. Host Saturation:** Reads `/sys/fs/cgroup/cpu.stat` (`throttled_usec`). If the container is throttled by Docker while host CPU has idle capacity, it confirms the container hit its specific cgroup limit.
3. **Ephemeral Port Exhaustion:** Monitors `TIME_WAIT` sockets on the loopback interface (`127.0.0.1`) to prevent artificial connection resets.

### Component 2: Saturation & Knee-Point Evaluator (`internal/analyzer/saturation.go`)
Instead of waiting for an application to crash completely with 100% 500 errors, the Saturation Engine detects the **knee-point**—the inflection point where response latency diverges from linear scaling into exponential queueing.

* **Algorithm:**
  * Tracks moving average and standard deviation of p95 latency across load steps:
    $$\Delta \text{Latency} = \frac{\text{p95}_{n} - \text{p95}_{n-1}}{\text{VUs}_{n} - \text{VUs}_{n-1}}$$
  * When $\Delta \text{Latency}$ exceeds the slope threshold (default: $3\times$ baseline slope), or when hard SLA limits are breached:
    * $\text{CPU} > 85\%$
    * $\text{p95 Latency} > \text{Threshold}$
    * $\text{Error Rate} > 1.0\%$
  * The step is flagged as **Saturated**, and progressive ramping halts cleanly.

### Component 3: Root-Cause Bottleneck Classifier (`internal/analyzer/bottleneck.go`)
When saturation is detected, the classifier evaluates cross-service telemetry to pinpoint the primary constraint:

```text
Saturation Detected
       │
       ├─► [API cgroup throttled > 15%] AND [DB CPU < 50%]
       │   └──► Primary: API CPU Saturation
       │
       ├─► [Postgres CPU > 80%] OR [active query locks > 5]
       │   └──► Primary: PostgreSQL Query / CPU Bottleneck
       │
       ├─► [API Errors > 1%] AND [Postgres active == max_connections]
       │   └──► Primary: Database Connection Pool Starvation (Client-Side)
       │
       ├─► [API Container RSS steadily increasing across all stages without drop]
       │   └──► Primary: Application Memory Leak / OOM Risk
       │
       └─► [Redis used_memory > maxmemory] AND [evicted_keys > 0]
           └──► Primary: Redis Cache Eviction Pressure
```

### Component 4: Sizing & Recommendation Engine (`internal/recommender`)
Translates empirical test results into production hardware specifications:
1. **Safety Margin Headroom:** Deducts a 30% safety buffer from the maximum observed capacity:
   $$C_{\text{sustainable}} = C_{\text{max}} \times 0.70$$
2. **Cloud Tier Mapping:** Compares observed resource consumption against standard VPS/Cloud instance specs:
   * Micro Tier: 0.5 vCPU, 512 MB RAM
   * Small Tier: 1.0 vCPU, 1 GB RAM
   * Medium Tier: 2.0 vCPU, 2 GB RAM
   * Compute-Optimized: 2.0+ vCPU, 1 GB RAM
   * Memory-Optimized: 1.0 vCPU, 4+ GB RAM
3. **Over-Provisioning Warning:** Flags configurations that exceed target capacity by $>100\%$, showing estimated cost savings.

### Component 5: Single-File Interactive HTML Report Bundler (`internal/report`)
* Bakes test execution metadata, time-series curves, bottleneck diagnostic cards, and recommendations into a self-contained HTML file.
* Uses synchronized scrubbing: moving the mouse over any timestamp highlights the latency, API CPU, and Postgres connection count simultaneously.
* Zero external network calls; opens instantly in any browser.

---

## 5. Non-Functional Decisions & Tradeoffs

### Why Go?
* **Single Static Binary:** No need for users to install Python, Node.js, or Java runtimes.
* **First-Class Docker SDK:** Docker is written in Go; the official Docker client SDK is maintained in Go.
* **Low Memory Footprint:** The CLI runner itself consumes $<25\text{MB}$ of RAM, minimizing interference with the local backend being benchmarked.

### Why Agentless Telemetry?
* Requiring developers to install APM agents or sidecars kills adoption. By querying standard Docker stats APIs and database SQL catalogs, CapacityLab works out-of-the-box on existing Docker Compose setups.

### Why Local-First (No Mandatory Cloud)?
* Developers run proprietary, unreleased software locally.
* Zero authentication, zero privacy concerns, zero subscription fees.
