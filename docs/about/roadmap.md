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
| **0.4.0** | July 2026 | Release management + core pipeline platform — UniFlow, Ray/Spark integration, pipeline/run/trigger management, Michelangelo CLI, Michelangelo Studio |
| **0.5.0** | Q3 2026 | End-to-end LLM model management — Foundation Model fine-tuning, model registry, offline inference, progressive serving |
| **TBD** | H2 2026 | Agent Infrastructure |

## Versioning Policy

Michelangelo follows [Semantic Versioning 2.0.0](https://semver.org/) with stability declared per component, not per repository.

| Stability Level | Guarantee |
|---|---|
| **stable** | Backwards-compatible across all minor and patch versions within a major. Breaking changes only at the next major. |
| **beta** | API may change between minor versions. Migration notes required in CHANGELOG. Breaking changes called out explicitly. |
| **alpha** | Anything goes. Use for experiments and previews. May be removed without deprecation notice. |

## Available Now

These capabilities are shipped and available in the current release. Individual guides are the source of truth for detailed feature availability.

**Project & Pipeline Management**
- Project creation and lifecycle management
- Pipeline authoring via UniFlow (Python DSL) with YAML-based configuration
- Revision management and versioning
- Pipeline deletion with cascade cleanup (Pipeline → PipelineRun, TriggerRun)

**Pipeline Execution**
- Pipeline run execution
- Trigger-based runs (cron schedule)
- Backfill runs
- Pipeline notifications (email and Slack via custom action setup)

**Distributed Training**
- Ray job launch and management
- Persistent Ray clusters via RayCluster CRD
- Federated multi-cluster dispatch

**Model Serving**
- Inference server creation (Triton backend)
- Rolling deployment strategy
- Traffic routing

**Infrastructure & Compute**
- Compute cluster registration
- Storage management via any S3-compatible object store

**Automation & Self-Healing**
- Revision-gated state transitions
- Condition engine pattern
- Federated multi-cluster status sync
- Finalizer-based cascade deletion

## Planned

These are capabilities we intend to build. Items closer to the top of each section are nearer-term.

**Pipeline Management**
- Draft-based authoring workflow
- Dev/Prod environment labels derived from git branch
- Auto-flip triggers (automatic revision switching on new revision)
- Interval and batch rerun trigger types
- Overridable parameters via blobstore URL
- Concurrent update protection via optimistic locking
- Canvas release version validation
- Block dev-branch runs in production (safety gate enforcement)

**Model Deployment**
- Deployment rollout strategies (Blast, Zonal, Shadow/A-B)
- Endpoint traffic splitting and shadow routing
- Automatic rollback on alert firing
- Decommission workflow with no-traffic validation gate
- Traffic routing via Istio/Envoy OSS gateway
- Compute lockdown detection
- Global endpoint name uniqueness (cross-namespace validation)

**Distributed Training**
- Spark job launch on Kubernetes
- GPU SKU normalization and validation via ConfigMap
- mTLS injection via cert-manager or OSS SPIFFE
- Prometheus ConfigMap auto-creation per job
- Job immutability (15-minute lock after kill)
- Spark obsolescence enforcement (7-day auto-kill for runaway jobs)
- Resource usage metrics emission

**Generative AI & LLM**
- GenAI service deployment: first-class support for deploying and managing LLM-backed inference
- AI agent management with declarative agent definitions and LLM registry
- Prompt template management
- Guardrail policies (input/output safety filtering, bias detection)

**Infrastructure & Compute**
- GCS storage support
- Resource pool selection
- Vector dataset management for embedding and RAG/similarity search

**Feature Store**
- Feature and feature group management
- Online feature store (low-latency feature serving)
- Offline feature datasets
- Feature serving groups
- Feature monitor with drift detection (Wasserstein, KL-divergence, PSI, LOF)
- Feature quality metrics
- Lineage event tracking on create/delete via OpenLineage
- Cascading deletion (FeatureGroup → Dataset)

**Evaluation & Reporting**
- Model explainability (TreeSHAP, Integrated Gradients, Permutation Feature Importance, KernelSHAP)
- Experiment reports
- Structured evaluation reports
- Model cards

**Alerting & Monitoring**
- Near-real-time feature drift monitoring (Wasserstein, KL divergence, PSI)
- Feature consistency monitoring (online vs. offline skew detection)
- Batch feature drift detection
- Auto-generated drift and availability alerts
- Dashboard management via OSS Grafana operator
- Prometheus-based alerting for decommission gating
- Alert CRD management
- Default cron schedules by alert type

**Project Management**
- Team ownership via OSS ownership model (CODEOWNERS)
- Cloud zone annotations for multi-cloud routing
- Git repository migration allowlist
- Routing affinity inheritance (parent-to-child annotation propagation)

**Automation & Self-Healing**
- Lockdown self-healing: detect and auto-remediate cluster lockdown conditions

---

The best way to influence what comes next is to open a GitHub issue or discussion with your use case. We treat this page as a living document and update it as concrete designs emerge.
