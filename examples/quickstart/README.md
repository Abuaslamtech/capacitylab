# Quickstart Sandbox

This directory contains a complete, self-contained microservice stack to test CapacityLab in under 60 seconds with Docker container resource telemetry.

## Architecture

- **`main.go`**: High-performance Go HTTP service implementing health checks, simulated crypto-auth (`/api/v1/auth/login`), item persistence with lock concurrency, and simulated database jitter.
- **`docker-compose.yml`**: Spins up `quickstart-api` throttled to **0.50 vCPU** and **256MB RAM** to demonstrate real container saturation limits.
- **`capacitylab.yaml`**: Multi-step user journey exercising authentication token extraction, authorization headers, dynamic random template variables, and SLA threshold enforcement.

## 30-Second Walkthrough

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

### 3. What CapacityLab Will Demonstrate

1. **Synthetic Warmup**: Primes the container and caches for 5 seconds before measuring.
2. **Knee-Point Detection**: Sentinel monitors latency acceleration curves and flags the inflection point where queueing begins.
3. **Container Telemetry**: Collects live Docker CPU % and RAM utilization from the container daemon.
4. **Root-Cause Classification**: Automatically reports when CPU approaches the 0.50 core throttle limit or latency breaches SLA.
5. **Interactive Report**: Generates `capacity-report.html` with latency histograms, stage telemetry, and multi-cloud sizing cost estimates.

### 4. Cleanup

```bash
docker compose down
```
