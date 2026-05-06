# Operator Guides

These guides cover deploying, configuring, and integrating Michelangelo in a Kubernetes environment. They target platform engineers and infrastructure operators who are responsible for running Michelangelo in production and for connecting it to the broader ML infrastructure their teams already use — experiment tracking, model registries, compute clusters, schedulers, and serving frameworks.

## Getting Started

For a fresh deployment, follow this recommended reading order:

1. **[Platform Setup](setup/platform-setup.md)** — configure each component (API server, controller manager, worker, UI/Envoy) via ConfigMaps and Kustomize overlays
2. **[Register a Compute Cluster](setup/register-a-compute-cluster-to-michelangelo-control-plane.md)** — connect an existing Kubernetes cluster so Michelangelo can dispatch Ray and Spark jobs to it
3. **[Cluster Setup for Serving](serving/cluster-setup.md)** — enable model inference on a local or remote cluster
4. **[Authentication](setup/authentication.md)** — connect an identity provider and configure RBAC before opening to users

## Setup & Configuration

| Guide | Description |
|-------|-------------|
| [Helm Chart](helm-chart.md) | Install the Michelangelo control plane with Helm — chart layout, values reference, and migration phases |
| [Platform Setup](setup/platform-setup.md) | ConfigMaps and key fields for API server, controller manager, worker, and UI/Envoy |
| [Network & Ingress](setup/network.md) | Envoy proxy, Ingress setup, TLS with cert-manager, and multi-cluster connectivity |
| [Authentication](setup/authentication.md) | OIDC identity provider setup, RBAC, session configuration, multi-tenant isolation |
| [Register a Compute Cluster](setup/register-a-compute-cluster-to-michelangelo-control-plane.md) | Connect an existing Kubernetes cluster to the Michelangelo control plane |

## Platform Components

| Guide | Description |
|-------|-------------|
| [Model Registry](components/model-registry.md) | Operate Michelangelo's built-in model registry, configure storage and RBAC, and integrate with serving and CI/CD |
| [Ingester Controller](components/ingester-configuration.md) | Deploy, configure, and operate the ingester that syncs CRDs into MySQL |

## Jobs & Compute

| Guide | Description |
|-------|-------------|
| [Jobs Overview](jobs/index.md) | Ray and Spark job lifecycle, compute selection, and observability |
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

## Third-Party Integrations

Michelangelo is designed to run alongside existing ML infrastructure. The guides below cover making external tools reachable from Michelangelo workloads.

| Guide | Description |
|-------|-------------|
| [Experiment Tracking Setup](experiment-tracking.md) | Make an experiment tracking server reachable from task pods — network, ConfigMap injection, auth, and operator/user boundary |
| [Browse all integrations](integrations/index.md) | MLflow and other third-party integration guides |

## Operations

| Guide | Description |
|-------|-------------|
| [Monitoring & Observability](operations/monitoring.md) | Prometheus scrape config, key metrics, alerting rules, Grafana dashboards, structured logging |
| [Compliance](operations/compliance.md) | SOC 2, GDPR, and HIPAA configuration |
| [Troubleshooting](operations/troubleshooting.md) | Common failure modes and `kubectl` diagnostic commands |

## Architecture & Reference

| Guide | Description |
|-------|-------------|
| [API Framework](api-framework.md) | Architecture overview of the Michelangelo API and control plane |
| [SQL Key Concepts and Terms](sql-key-concepts-and-terms.md) | Metadata schema, table naming, indexed fields, and SQL query patterns |
