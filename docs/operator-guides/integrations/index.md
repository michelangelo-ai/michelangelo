# Third-Party Integrations

This section covers connecting third-party tools to Michelangelo. If you are looking for documentation on Michelangelo's own components — the model registry, experiment tracking setup, serving infrastructure, or job scheduler — see the [Operator Guides index](../index.md).

Before configuring any tool below, complete [Experiment Tracking Setup](../experiment-tracking.md) — the platform-level guide for network reachability, ConfigMap injection, auth, and the operator/user boundary that applies to all third-party tracking integrations.

| Guide | Description |
|-------|-------------|
| [Comet ML](cometml.md) | Connect to Comet ML's experiment tracking — network setup, API key injection, PyTorch Lightning / Ray Train / HuggingFace Transformers / custom training loop patterns, distributed experiment coordination, and Comet ML vs Michelangelo model registry comparison|
| [MLflow](mlflow.md) | Connect a self-hosted or Databricks-managed MLflow Tracking Server — network setup, auth, and MLflow vs Michelangelo registry comparison |

## Next Steps

- [Network & Ingress](../setup/network.md) — configure egress rules, Envoy proxy, ingress, TLS, and multi-cluster networking
- [Authentication](../setup/authentication.md) — manage secrets, workload identity, and RBAC for credential handling
- [Troubleshooting](../operations/troubleshooting.md) — diagnose common failure modes with `kubectl` diagnostic commands