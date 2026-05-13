# Third-Party Integrations

This section covers connecting third-party tools to Michelangelo. If you are looking for documentation on Michelangelo's own components — the model registry, experiment tracking setup, serving infrastructure, or job scheduler — see the [Operator Guides index](../index.md).

## Experiment Tracking

| Guide | Description |
|-------|-------------|
| [Experiment Tracking Setup](../experiment-tracking.md) | Platform-level setup: network reachability, ConfigMap injection, auth, and operator/user boundary — read this before any tool-specific guide |
| [MLflow](mlflow.md) | Connect a self-hosted or Databricks-managed MLflow Tracking Server — network setup, auth, and MLflow vs Michelangelo registry comparison |

## Next Steps

- [Network & Ingress](../network.md) — Envoy proxy, ingress, TLS, and multi-cluster networking context for egress rules
- [Authentication](../authentication.md) — secrets, workload identity, and RBAC for credential management
- [Troubleshooting](../troubleshooting.md) — common failure modes and `kubectl` diagnostic commands
