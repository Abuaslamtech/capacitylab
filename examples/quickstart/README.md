# Quickstart Sandbox

This directory contains a microservice stack to test CapacityLab with Docker container resource telemetry.

## Architecture

- **`main.go`**: Go HTTP service implementing health checks, crypto-auth simulation (`/api/v1/auth/login`), item persistence with lock concurrency, and database jitter.
- **`docker-compose.yml`**: Runs `quickstart-api` constrained to **0.50 vCPU** and **256MB RAM** to test container saturation limits.
- **`capacitylab.yaml`**: User journey testing authentication token extraction, authorization headers, dynamic random template variables, and SLA thresholds.

## Quickstart Walkthrough

### 1. Start the Sandbox Stack

```bash
docker compose up -d --build
```

Verify that the service is running:
```bash
curl http://localhost:8080/health
# {"service":"quickstart-api","status":"ok"}
```

### 2. Run CapacityLab

From this directory:
```bash
capacitylab run --open
```

### 3. What CapacityLab Measures

1. **Warmup Phase**: Primes the container and caches for 5 seconds before measuring.
2. **Knee-Point Detection**: Sentinel tracks latency acceleration curves to detect the inflection point where queueing begins.
3. **Container Telemetry**: Scrapes live Docker CPU and RAM utilization from the container daemon.
4. **Root-Cause Classification**: Reports when CPU approaches the 0.50 core limit or latency breaches SLA.
5. **Interactive Report**: Generates `capacity-report.html` with latency histograms, stage telemetry, and multi-cloud sizing cost estimates.

### 4. Cleanup

```bash
docker compose down
```
