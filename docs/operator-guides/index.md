# Operator Guides

These guides cover deploying, configuring, and integrating Michelangelo in a Kubernetes environment. They are written for platform engineers and infrastructure operators responsible for running Michelangelo in production or staging environments.

## Getting Started

For a fresh deployment, follow this recommended reading order:

1. **[Platform Setup](platform-setup.md)** — configure each component (API server, controller manager, worker, UI/Envoy) via ConfigMaps and Kustomize overlays
2. **[Register a Compute Cluster](jobs/register-a-compute-cluster-to-michelangelo-control-plane.md)** — connect an existing Kubernetes cluster so Michelangelo can dispatch Ray and Spark jobs to it
3. **[Cluster Setup for Serving](serving/cluster-setup.md)** — enable model inference on a local or remote cluster
4. **[Authentication](authentication.md)** — connect an identity provider and configure RBAC before opening to users

## Platform Configuration

| Guide | Description |
|-------|-------------|
| [Platform Setup](platform-setup.md) | ConfigMaps and key fields for API server, controller manager, worker, and UI/Envoy |
| [API Framework](api-framework.md) | Architecture overview of the Michelangelo API and control plane |

## Jobs & Compute

| Guide | Description |
|-------|-------------|
| [Jobs Overview](jobs/index.md) | Ray and Spark job lifecycle, compute selection, and observability |
| [Register a Compute Cluster](jobs/register-a-compute-cluster-to-michelangelo-control-plane.md) | Connect an existing Kubernetes cluster to the Michelangelo control plane |
| [Run a Pipeline on a Compute Cluster](jobs/run-uniflow-pipeline-on-compute-cluster.md) | Submit and monitor a Uniflow pipeline on a registered cluster |
| [Extend the Job Scheduler](jobs/extend-michelangelo-batch-job-scheduler-system.md) | Custom scheduling backends (Kueue, Volcano) and assignment strategies |

## Model Serving

| Guide | Description |
|-------|-------------|
| [Serving Overview](serving/index.md) | InferenceServer and Deployment lifecycle, architecture |
| [Cluster Setup for Serving](serving/cluster-setup.md) | Configure a cluster for inference |
| [Integrate a Custom Backend](serving/integrate-custom-backend.md) | Plugin interfaces for Triton, vLLM, TensorRT-LLM, and custom frameworks |

## UI

| Guide | Description |
|-------|-------------|
| [Deploying the UI](ui/deploying-michelangelo-ui.md) | Deploy the Michelangelo web UI to Kubernetes |
| [Local UI Development](ui/local-development-setup.md) | Run the UI locally for development |

## Integrations

| Guide | Description |
|-------|-------------|
| [MLflow](integrations/mlflow.md) | Experiment tracking, model registry sync, and evaluation with MLflow |

## Operations

| Guide | Description |
|-------|-------------|
| [Authentication](authentication.md) | OIDC identity provider setup, RBAC, session configuration, multi-tenant isolation |
| [Monitoring & Observability](monitoring.md) | Prometheus metrics, Grafana dashboards, and alerting rules |
| [Compliance](compliance.md) | SOC 2, GDPR, and HIPAA configuration |
| [Troubleshooting](troubleshooting.md) | Common failure modes and `kubectl` diagnostic commands |

## Ingester

| Guide | Description |
|-------|-------------|
| [Ingester Design](ingester-design.md) | Architecture and design of the Michelangelo data ingester |
| [Ingester Sandbox Validation](ingester-sandbox-validation.md) | Validate ingester behavior in a local sandbox |
