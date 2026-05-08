Sandbox is a lightweight version of the Michelangelo cluster, designed specifically for development and testing. It also serves as an excellent tool for users to quickly explore the platform and familiarize themselves with its interface.

> **Note:** The Sandbox deployment is intended for development and testing purposes only and is not suitable for production environments.
> For guidance on creating a production-ready Michelangelo deployment, please refer to the `helm/michelangelo/` chart and its README.

## User Guide

### Prerequisites

**Required Software**

Please install the following software before proceeding:

- [Docker](https://docs.docker.com/get-started/get-docker)
- [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
- [k3d](https://k3d.io)
- [Helm 3.12+](https://helm.sh/docs/intro/install/)

### Install Michelangelo CLI

```bash
pip install michelangelo
ma sandbox --help
```

## Commands

| Command | Description |
|---|---|
| `ma sandbox create` | Create a new k3d cluster, deploy all infrastructure and the control plane |
| `ma sandbox sync` | Fast redeploy — upgrades only the control plane (leaves infrastructure running) |
| `ma sandbox delete` | Uninstall the control plane and delete the k3d cluster |
| `ma sandbox start` | Start a previously stopped k3d cluster |
| `ma sandbox stop` | Stop the k3d cluster (preserves data) |
| `ma sandbox demo pipeline` | Run the pipeline demo against a running sandbox |
| `ma sandbox demo inference` | Run the inference demo against a running sandbox |

## Architecture

The sandbox is split into two tiers managed by different tools:

| Tier | Tool | What it owns |
|---|---|---|
| **Infrastructure** | `sandbox.py` (`ma sandbox`) | MySQL, MinIO, Cadence/Temporal, Prometheus, Grafana, MLflow, Fluent Bit, KubeRay, Spark Operator, k3d cluster lifecycle |
| **Control plane** | Helm (`helm/michelangelo`) | apiserver, envoy, UI, worker, controllermgr, CRDs, RBAC |

After `ma sandbox create`, the control plane is managed as a Helm release named `michelangelo`:

```bash
helm list                  # shows the michelangelo release
helm status michelangelo   # current state and NOTES
helm test michelangelo     # smoke test (apiserver healthz)
```

## Workflow Engine Options

You can choose the workflow engine when creating a sandbox:

- **Cadence** (default):

```bash
ma sandbox create
# or explicitly:
ma sandbox create --workflow cadence
```

- **Temporal**:

```bash
ma sandbox create --workflow temporal
```

The workflow engine can be switched by deleting and recreating the sandbox. `ma sandbox sync` keeps the same engine as the original create.

## Excluding Services

Use `--exclude` to skip specific services:

```bash
ma sandbox create --exclude worker controllermgr
ma sandbox sync --exclude ui
```

**Control plane services** (Helm-managed): `apiserver`, `ui`, `worker`, `controllermgr`

**Infrastructure services**: `prometheus`, `grafana`, `ray`, `spark`

## Experimental Services

```bash
ma sandbox create --include-experimental fluent-bit mlflow
```

## Local Service URLs

| Service | URL | Notes |
|---|---|---|
| Michelangelo UI | http://localhost:8090 | |
| Envoy (gRPC-Web) | http://localhost:8081 | |
| Apiserver (gRPC) | localhost:15566 | |
| Cadence Web | http://localhost:8088 | Only when `--workflow cadence` (default) |
| Temporal Web | http://localhost:8080 | Only when `--workflow temporal` — port-forwarded automatically by sandbox |
| MinIO Console | http://localhost:9090 | |
| Grafana | http://localhost:3000 | |
| Prometheus | http://localhost:9092 | |

## Monitoring and Logging

Prometheus and Grafana are deployed automatically during `ma sandbox create`. No manual steps required.

### Access Prometheus

Prometheus is directly accessible at http://localhost:9092 — no port-forward needed.

### Available Metrics

The controller manager exposes comprehensive metrics including:

**CRD Unmarshal Metrics:**
- `cr_unmarshal_errors_total{crd_type="Pipeline",namespace="...",error_type="unmarshal_error"}` - CRD unmarshal errors by type and namespace

**Controller Runtime Metrics:**
- `controller_runtime_reconcile_total{controller="pipeline|raycluster|rayjob",result="success|error|requeue"}` - Reconciliation results
- `controller_runtime_active_workers{controller="..."}` - Active worker counts
- `controller_runtime_reconcile_errors_total{controller="..."}` - Total reconciliation errors

**Go Runtime Metrics:**
- `go_goroutines` - Number of goroutines
- `go_gc_duration_seconds` - Garbage collection duration
- Memory, heap, and GC statistics

### Sample Prometheus Queries

Use these queries in the Prometheus UI:

- **CRD unmarshal error rate**: `rate(cr_unmarshal_errors_total[5m])`
- **Controller reconciliation success rate**: `rate(controller_runtime_reconcile_total{result="success"}[5m])`
- **Active workers per controller**: `controller_runtime_active_workers`
- **Memory usage**: `go_gc_heap_objects_bytes`

### Log Collection with Fluent Bit

Enable Fluent Bit with `--include-experimental fluent-bit`:

```bash
ma sandbox create --include-experimental fluent-bit
```

Fluent Bit tails Ray job logs from `/tmp/ray/session_*/logs/job-*.log` and stores them in MinIO S3-compatible storage in JSON format.
