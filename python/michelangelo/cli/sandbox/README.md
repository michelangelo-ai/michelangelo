Sandbox is a lightweight version of the Michelangelo cluster, designed specifically for development and testing. It also serves as an excellent tool for users to quickly explore the platform and familiarize themselves with its interface.

> **Note:** The Sandbox deployment is intended for development and testing purposes only and is not suitable for production environments.
> For guidance on creating a production-ready Michelangelo deployment, please refer to the Deployment Guide.

## User Guide

### Prerequisites

**Required Software**

Please install the following software before proceeding:

- [Docker](https://docs.docker.com/get-started/get-docker)
- [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
- [k3d](https://k3d.io)

### Install Michelangelo CLI

```bash
pip install michelangelo
ma sandbox --help
```

## Workflow Engine Options

You can choose the workflow engine when creating a Michelangelo sandbox:

- To create a sandbox using **Temporal**, use:

```bash
ma sandbox --workflow temporal
```

- To create a sandbox using **Cadence**, use either of the following commands:

```bash
ma sandbox
# or explicitly
ma sandbox --workflow cadence
```

For detailed instructions and additional setup options, please follow the [Temporal Development Environment Guide](https://learn.temporal.io/getting_started/typescript/dev_environment/).

## Monitoring and Logging

The sandbox includes monitoring and logging capabilities to observe the controller manager and collect Ray job logs.

### Metrics Collection with Prometheus

1. **Deploy Prometheus:**
   ```bash
   kubectl apply -f resources/prometheus.yaml
   ```

2. **Wait for deployment to be ready:**
   ```bash
   kubectl wait --for=condition=available deployment/prometheus --timeout=60s
   ```

3. **Access Prometheus:**
   ```bash
   # Forward Prometheus (runs in background)  
   kubectl port-forward svc/prometheus 9090:9090 &
   ```
   - **Prometheus UI**: http://localhost:9090

### Log Collection with Fluent Bit

The sandbox includes Fluent Bit for collecting Ray job logs and storing them in MinIO:

1. **Components are deployed automatically** - Fluent Bit DaemonSet and MinIO are included in the sandbox setup
2. **Log collection** - Fluent Bit tails Ray job logs from `/tmp/ray/session_*/logs/job-*.log`
3. **Storage** - Logs are stored in MinIO S3-compatible storage in JSON format

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

### Architecture

The sandbox monitoring and logging architecture:

1. **Controller Manager** - Runs as a pod in k3d, exposes metrics at `:8090/metrics`
2. **Prometheus** - Scrapes controller manager metrics via Kubernetes service discovery
3. **Fluent Bit** - DaemonSet collects Ray logs and sends them to MinIO
4. **MinIO** - S3-compatible storage for logs and artifacts

All components run inside the k3d cluster with proper Kubernetes service networking.

