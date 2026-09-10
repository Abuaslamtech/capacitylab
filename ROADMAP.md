# CapacityLab Roadmap

This document outlines planned improvements and milestones for **CapacityLab**. 

CapacityLab prioritizes bottleneck diagnosis and reproducible capacity boundaries over adding generic load-generation features.

---

## Release Milestones

### v0.2.0: Reproducibility and Bottleneck Evidence (Upcoming)
- [ ] **Test Environment Fingerprinting**:
  - Automatically record Git commit SHA, host kernel, CPU topology, available RAM, Docker engine version, and config hash into every run's metadata.
  - Display reproducibility stamp on reports: `"Result reproducible against config hash 8f3a2c1 with Docker v26.1"`.
- [ ] **Multi-Signal Diagnostic Correlation**:
  - Enhance bottleneck reports with side-by-side evidence metrics (e.g., *"PostgreSQL pool utilization crossed 90% while API CPU remained <45%"*).
- [ ] **GitHub PR Commenter Integration**:
  - Automatically format `--fail-on-regression` results into rich Markdown summary comments posted to GitHub Pull Requests.

---

### v0.3.0: Statistical Rigor and Confidence Bounds
- [ ] **Confidence Interval Capacity Sizing**:
  - Output capacity estimates with confidence bounds across multiple stage slices (e.g., `Estimated Sustainable Capacity: 170–190 VUs (Best Estimate: 183 VUs)`).
- [ ] **Variance & Jitter Rejection**:
  - Automatic detection of cold-start cache anomalies and transient network spikes to prevent false knee-point triggers.
- [ ] **Custom Pacing Models**:
  - Support Poisson arrival distributions and customized think-time profiles.

---

### v0.4.0: Framework Sandboxes
- [ ] **NestJS + PostgreSQL Sandbox**: Pre-configured Docker Compose stack demonstrating ORM connection pool limits.
- [ ] **Express + MongoDB Sandbox**: Demonstrating NoSQL indexing and unindexed aggregation memory spikes.
- [ ] **Python (FastAPI / Celery) Sandbox**: Demonstrating async event-loop blocking and worker queue saturation.
- [ ] **Kubernetes Micro-Agent (Optional Mode)**: Zero-privilege daemonset helper for multi-node pod resource scraping.

---

## Contributing

To propose changes or pick up a planned item:
1. Open an issue or discussion on GitHub to outline your approach.
2. Submit a pull request against `main`.
