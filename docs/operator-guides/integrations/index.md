# Integrating with Your ML Stack

Michelangelo is designed to run alongside the ML infrastructure your organization already has. This section covers how to connect it to external systems and how to extend its built-in components.

---

## External System Integrations

> **Coming soon** — Guides for connecting Michelangelo to external experiment tracking servers and model registries are in progress. See #1041 and #1042.


---

## Extending Built-in Components

Michelangelo exposes extension points for replacing or augmenting its core subsystems. Use these when the defaults don't fit your infrastructure.

### Serving

| Guide | Description |
|-------|-------------|
| [Custom Serving Backend](../serving/integrate-custom-backend.md) | Implement the `Backend`, `ModelConfigProvider`, and `RouteProvider` interfaces to add support for any inference framework — Triton, vLLM, TensorRT-LLM, or your own |

### Job Scheduling

| Guide | Description |
|-------|-------------|
| [Extend the Job Scheduler](../jobs/extend-michelangelo-batch-job-scheduler-system.md) | Replace or extend the scheduler — integrate Kueue, Volcano, or implement a custom `JobQueue` and `AssignmentStrategy` |
| [Register a Compute Cluster](../jobs/register-a-compute-cluster-to-michelangelo-control-plane.md) | Connect an existing Kubernetes cluster so Michelangelo can dispatch Ray and Spark jobs to it |

---

## Related Operator Guides

- [Platform Setup](../platform-setup.md) — ConfigMap reference for all components
- [Authentication](../authentication.md) — OIDC, RBAC, and service-to-service auth
- [Network & Ingress](../network.md) — Ingress setup, Envoy proxy config, TLS, multi-cluster networking
- [Monitoring](../monitoring.md) — Prometheus metrics, alerting, Grafana dashboards
