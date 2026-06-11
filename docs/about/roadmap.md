---
sidebar_position: 2
sidebar_label: "Roadmap"
---

# Roadmap

Michelangelo is under active development. This page captures the current state of the platform and the direction we're headed. Things will shift as priorities evolve and the community gives feedback.

## Release Milestones

The OSS release is staged so each version makes a specific, scoped promise rather than trying to be everything at once.

| Version | Target | Focus |
|---|---|---|
| **0.2.0** | June 2026 | Release management + core pipeline platform — UniFlow, Ray/ Spark integration, pipeline/run/trigger management, MA commands, MA Studio |
| **0.3.0** | July 2026 | End-to-end LLM model management — Foundation Model fine-tuning, model registry, offline inference, progressive serving |
| **0.4.0** | H2 FY26 | Agent + LLM Gateway — K8s-native containerized agent (Agent Job API + Controller Manager) + LLM Gateway (LiteLLM or Bifrost, CRD-sync routing) |

## Versioning Policy

Michelangelo follows [Semantic Versioning 2.0.0](https://semver.org/) with stability declared per component, not per repository.

| Stability Level | Guarantee |
|---|---|
| **stable** | Backwards-compatible across all minor and patch versions within a major. Breaking changes only at the next major. |
| **beta** | API may change between minor versions. Migration notes required in CHANGELOG. Breaking changes called out explicitly. |
| **alpha** | Anything goes. Use for experiments and previews. May be removed without deprecation notice. |

## Available Now

These capabilities are shipped and available in the current release.

**Project & Pipeline Management**
- Project creation and lifecycle management
- Pipeline authoring in K3s CRD YAML, Python, and Uniflow
- Revision management and versioning
- Overridable parameters via S3/GCS URLs
- Auto-flip triggers on main branch merge

**Pipeline Execution**
- Pipeline run execution
- Trigger-based runs (cron, interval, and batch rerun)
- Backfill runs
- Batch rerun

**Distributed Training**
- Ray job launch and management
- Spark job launch on Kubernetes
- Persistent Ray clusters via RayCluster CRD
- Federated multi-cluster dispatch

**Model Serving**
- Inference server creation
- Deployment rollout strategies (Blast, Rolling, Zonal, Shadow/A-B)
- Endpoint traffic splitting and shadow routing
- Training insights

**Infrastructure & Compute**
- Compute cluster registration
- Resource pool selection
- Storage management via S3/GCS

**Automation & Self-Healing**
- Revision-gated state transitions
- Condition engine pattern
- Federated multi-cluster status sync

## In Progress

These features are actively being built and will land in upcoming releases.

**Pipeline Authoring**
- Draft-based authoring workflow
- Dev/Prod environment labels derived from git branch

**Model Deployment**
- Automatic rollback on alert firing
- Decommission workflow with no-traffic validation gate
- DeploymentEvent tracking

**Generative AI & LLM**
- GenAI service deployment: first-class support for deploying and managing LLM-backed inference endpoints

**Evaluation & Reporting**
- Experiment reports

**Alerting & Monitoring**
- Alert-triggered auto rollback
- Dashboard management via OSS Grafana operator
- Prometheus-based alerting for decommission gating

**Infrastructure**
- Notebook (Jupyter) sessions
- Docker image builds via Kaniko/BuildKit

**Automation**
- Finalizer-based cascade deletion (Pipeline → Revisions; FeatureGroup → Datasets)

## On the Radar

These are planned capabilities we are working towards adding down the road.

**Project Management**
- GenAI project flavor support
- Team ownership via OSS ownership model (CODEOWNERS)
- Cloud zone annotations for multi-cloud routing
- Git repository migration allowlist
- Routing affinity inheritance (parent-to-child annotation propagation)

**Pipeline Authoring & Execution**
- Concurrent update protection via optimistic locking
- Canvas release version validation
- Block dev-branch runs in production (safety gate enforcement)

**Distributed Training**
- GPU SKU normalization and validation via ConfigMap
- mTLS injection via cert-manager or OSS SPIFFE
- Prometheus ConfigMap auto-creation per job
- Job immutability (15-minute lock after kill)
- Spark obsolescence enforcement (7-day auto-kill for runaway jobs)
- Resource usage metrics emission

**Model Deployment**
- Lockdown self-healing: detect and auto-remediate cluster lockdown conditions
- Traffic routing via Istio/Envoy OSS gateway
- Compute lockdown detection
- Global endpoint name uniqueness (cross-namespace validation)

**Feature Store**
- Feature and feature group management
- Online feature store (low-latency feature serving)
- Offline feature datasets
- Feature serving groups
- Feature monitor with drift detection (Wasserstein, KL-divergence, PSI, LOF)
- Feature quality metrics
- Lineage event tracking on create/delete via OpenLineage
- Cascading deletion (FeatureGroup → Dataset)

**Generative AI & LLM**
- AI agent management with declarative agent definitions and LLM registry
- Prompt template management
- Guardrail policies (input/output safety filtering, bias detection)

**Evaluation & Reporting**
- Structured evaluation reports
- Model cards

**Alerting & Monitoring**
- Alert CRD management
- Default cron schedules by alert type

**Infrastructure & Compute**
- Vector dataset management for embedding and RAG/similarity search

**Automation & Self-Healing**
- Lockdown self-healing: detect and auto-remediate cluster lockdown conditions

---

The best way to influence what comes next is to open a GitHub issue or discussion with your use case. We treat this page as a living document and update it as concrete designs emerge.
