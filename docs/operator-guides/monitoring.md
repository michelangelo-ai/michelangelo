# Monitoring & Observability

Michelangelo components expose Prometheus metrics that integrate with a standard Kubernetes observability stack. This guide covers scrape configuration, key metrics to monitor, alerting rules, and logging configuration.

## Prometheus Scrape Configuration

### Controller Manager

The controller manager exposes metrics on port `8091` (configured via `metricsBindAddress`). If you are using the [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator), create a `ServiceMonitor`:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: michelangelo-controllermgr
  namespace: ma-system
  labels:
    app: michelangelo-controllermgr
spec:
  selector:
    matchLabels:
      app: michelangelo-controllermgr
  endpoints:
  - port: metrics          # Must match the Service port name for port 8091
    path: /metrics
    interval: 30s
```

### Health Probes

The controller manager exposes health endpoints on port `8083` (configured via `healthProbeBindAddress`):

| Endpoint | Purpose |
|----------|---------|
| `GET :8083/healthz` | Liveness — is the process alive? |
| `GET :8083/readyz` | Readiness — is the controller ready to reconcile? |

These are used by Kubernetes liveness and readiness probes, but you can also poll them from your monitoring stack for coarser-grained health checks.

### API Server

The API server (port `15566`) exposes standard gRPC metrics. If you have a Prometheus scrape job for gRPC services, point it at the API server pod.

### Envoy Proxy

Envoy can expose an admin stats interface for scraping request counts, latency histograms, and upstream error rates. The admin interface is **not enabled by default** in the Michelangelo Envoy configuration — you must add an `admin:` block to your Envoy ConfigMap to enable it. See the [Envoy admin documentation](https://www.envoyproxy.io/docs/envoy/latest/operations/admin) for setup instructions. Once enabled, add a Prometheus scrape job targeting the admin port.

---

## Key Metrics

### Pipeline Runs

| Metric | Description | Unit |
|--------|-------------|------|
| `pipelinerun_result_total` | Pipeline run results, by `state`, `pipeline_type`, `environment`, `tier` | Count |
| `pipelinerun_result_failure_total` | Failed pipeline runs, with `failure_reason` label | Count |
| `pipelinerun_duration_seconds` | Pipeline run execution duration (histogram) | Seconds |
| `pipelinerun_failed` | Gauge: 1 if most recent run failed, 0 if succeeded | Gauge |
| `pipelinerun_step_success_total` | Step completions, by `step_name` and `pipeline_type` | Count |
| `pipeline_ready_total` | Pipelines reaching Ready state | Count |

### Workflow Engine

Workflow metrics are emitted by the Cadence or Temporal server, not by Michelangelo. Consult your workflow engine's documentation for its native Prometheus metrics. Michelangelo's worker-side reconcile metrics are captured under the `pipelinerun_*` counters above.

### Model Serving

| Metric | Description | Unit |
|--------|-------------|------|
| `michelangelo_inferenceserver_ready_count` | Healthy InferenceServer instances | Count |
| `michelangelo_deployment_rollout_duration_seconds` | Time to complete a model rollout | Seconds |
| `envoy_cluster_upstream_rq_5xx` | 5xx error responses from inference backends | Count |
| `envoy_cluster_upstream_rq_time` | Request latency histogram to inference servers | Seconds |

### Controller Manager Health

The controller manager uses `controller-runtime` metrics — these are standard across all Kubernetes operators:

| Metric | Description | Unit |
|--------|-------------|------|
| `controller_runtime_reconcile_errors_total` | Reconcile errors, by `controller` label | Count |
| `controller_runtime_reconcile_time_seconds` | Reconcile duration histogram | Seconds |
| `workqueue_depth` | Work items queued, by `name` label (one per controller) | Count |
| `workqueue_retries_total` | Work item retries — elevated value indicates persistent failures | Count |

---

## Alerting Rules

Add these rules to your Prometheus configuration:

```yaml
groups:
- name: michelangelo
  rules:

  # Scheduling backlog: more than 50 jobs waiting for more than 5 minutes
  - alert: JobSchedulingBacklogHigh
    expr: michelangelo_scheduler_queue_depth > 50
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Job scheduling backlog is high ({{ $value }} jobs)"
      description: >
        The scheduler queue has {{ $value }} jobs waiting for more than 5 minutes.
        Check cluster availability: kubectl -n ma-system get clusters

  # No healthy compute clusters — new jobs cannot be scheduled
  - alert: NoHealthyComputeClusters
    expr: michelangelo_cluster_count{status="ready"} < 1
    for: 2m
    labels:
      severity: critical
    annotations:
      summary: "No healthy compute clusters available"
      description: >
        All registered compute clusters are unhealthy. No new jobs can be scheduled.
        Check cluster status: kubectl -n ma-system describe clusters

  # Controller reconcile errors — sustained error rate from any controller
  - alert: ControllerReconcileErrorRate
    expr: rate(controller_runtime_reconcile_errors_total[5m]) > 0.1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Controller {{ $labels.controller }} has high reconcile error rate"
      description: >
        The {{ $labels.controller }} controller is failing reconciles at
        {{ $value | humanize }} errors/sec. Check logs:
        kubectl -n ma-system logs deployment/michelangelo-controllermgr

  # Inference latency: P99 above 500ms for 5 minutes
  - alert: InferenceLatencyHigh
    expr: >
      histogram_quantile(0.99,
        rate(envoy_cluster_upstream_rq_time_bucket[5m])
      ) > 500
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Inference P99 latency is above 500ms"
      description: >
        The 99th percentile inference request latency is {{ $value }}ms.
        Check InferenceServer and model-sync sidecar logs.

  # Inference error rate: more than 1% of requests returning 5xx
  - alert: InferenceErrorRateHigh
    expr: >
      rate(envoy_cluster_upstream_rq_5xx[5m])
      / rate(envoy_cluster_upstream_rq_total[5m]) > 0.01
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Inference 5xx error rate above 1%"
      description: >
        {{ $value | humanizePercentage }} of inference requests are returning 5xx errors.
```

---

## Grafana Dashboard

Create a Grafana dashboard with these panels to get operational visibility at a glance.

### Overview row

| Panel | Query | Visualization |
|-------|-------|---------------|
| Job queue depth | `michelangelo_scheduler_queue_depth` | Time series |
| Healthy compute clusters | `michelangelo_cluster_count{status="ready"}` | Stat |
| Active InferenceServers | `michelangelo_inferenceserver_ready_count` | Stat |
| Temporal task backlog | `temporal_task_queue_backlog` | Time series |

### Jobs row

| Panel | Query | Visualization |
|-------|-------|---------------|
| Scheduling latency P50/P99 | `histogram_quantile(0.5, ...)` / `histogram_quantile(0.99, ...)` on `michelangelo_scheduler_assignment_duration_seconds` | Time series |
| Job provisioning latency | `histogram_quantile(0.99, michelangelo_job_provisioning_duration_seconds_bucket)` | Time series |

### Serving row

| Panel | Query | Visualization |
|-------|-------|---------------|
| Request rate | `rate(envoy_cluster_upstream_rq_total[5m])` | Time series |
| Request latency P50/P99 | `histogram_quantile(0.5/0.99, rate(envoy_cluster_upstream_rq_time_bucket[5m]))` | Time series |
| 5xx error rate | `rate(envoy_cluster_upstream_rq_5xx[5m])` | Time series |
| Active model deployments | `michelangelo_inferenceserver_ready_count` | Table |

### Controller health row

| Panel | Query | Visualization |
|-------|-------|---------------|
| Reconcile error rate by controller | `rate(controller_runtime_reconcile_errors_total[5m])` | Time series |
| Reconcile latency P99 | `histogram_quantile(0.99, rate(controller_runtime_reconcile_time_seconds_bucket[5m]))` | Time series |
| Work queue depth | `workqueue_depth` | Time series |

---

## Structured Logging

All Michelangelo components emit structured logs. Configure log format and level in the relevant ConfigMap:

```yaml
logging:
  level: info          # debug | info | warn | error
  development: false   # true enables human-readable console output
  encoding: json       # json for production; console for development
```

For production deployments use `encoding: json` so your log aggregation system (Loki, Elasticsearch, CloudWatch Logs, etc.) can parse and query fields natively.

### Important log fields to index

| Field | Description |
|-------|-------------|
| `level` | Log severity |
| `logger` | Component/controller name |
| `msg` | Log message |
| `namespace` | Kubernetes resource namespace |
| `name` | Kubernetes resource name |
| `operation` | Controller operation (e.g., `create_ray_cluster`, `schedule_job`) |
| `error` | Error message (present on error-level logs) |

Indexing these fields allows you to efficiently query all events for a specific resource (`namespace` + `name`), filter by controller (`logger`), or find all failures across the control plane (`level: error`).
