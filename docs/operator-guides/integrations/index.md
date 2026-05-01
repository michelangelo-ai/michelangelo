# Integrations

Michelangelo runs alongside the ML infrastructure your organization already has. This page is the operator reference for connecting external systems and extending Michelangelo's built-in components. It targets platform engineers responsible for running Michelangelo in production.

## Built-in Components

These guides cover components that ship with Michelangelo. Operators configure them; they are not external systems.

| Guide | Description |
|-------|-------------|
| [Model Registry](model-registry.md) | Verify the registry is healthy, configure object store and RBAC, and integrate registered models with serving and CI/CD |

## External System Integrations

These guides cover connecting Michelangelo to systems your organization already runs.

| Guide | Description |
|-------|-------------|
| [Experiment Tracking](experiment-tracking.md) | Expose an external experiment tracking server to task pods — network setup, URI injection, and operator/user boundary |
| [MLflow](mlflow.md) | Connect a self-hosted or managed MLflow Tracking Server — ConfigMap injection, authentication, and registry comparison |

## Extending Built-in Components

Michelangelo exposes extension points for replacing or augmenting its core subsystems. Use these when the defaults don't fit your infrastructure.

| Guide | Description |
|-------|-------------|
| [Custom Serving Backend](../serving/integrate-custom-backend.md) | Add support for any inference framework — Triton, vLLM, TensorRT-LLM, or your own |
| [Extend the Job Scheduler](../jobs/extend-michelangelo-batch-job-scheduler-system.md) | Replace or extend the scheduler — integrate Kueue, Volcano, or implement a custom `JobQueue` and `AssignmentStrategy` |
| [Register a Compute Cluster](../jobs/register-a-compute-cluster-to-michelangelo-control-plane.md) | Connect an existing Kubernetes cluster so Michelangelo can dispatch Ray jobs to it |

## Next Steps

- [Platform Setup](../platform-setup.md) — ConfigMap reference for all components
- [Authentication](../authentication.md) — OIDC, RBAC, and service-to-service auth
- [Network & Ingress](../network.md) — Ingress setup, Envoy proxy config, TLS, multi-cluster networking
- [Monitoring](../monitoring.md) — Prometheus metrics, alerting, Grafana dashboards
- [Troubleshooting](../troubleshooting.md) — Common failure modes and `kubectl` diagnostic commands
