---
sidebar_position: 2
sidebar_label: "Helm Chart"
---

# Michelangelo Helm Chart

This page describes the Helm chart for the Michelangelo control plane — converting the existing `sandbox.py`-based deployment into a first-class Helm chart installable against any Kubernetes cluster.

---

## Overview

Today the sandbox is orchestrated by `sandbox.py`, which applies raw YAML files in sequence and calls external Helm charts for KubeRay, Spark Operator, and Temporal. This plan converts the control plane into a single Helm chart — installable, upgradable, and tearable with standard Helm commands.

**Any cluster** — one `helm install` deploys the full control plane with connection values pointing at existing infrastructure:

- Production — RDS, S3, managed Temporal
- Staging — your cluster's services
- Local k3d — via `values-k3d.yaml`

**Clear boundary** — the chart owns only the control plane. Stateful infrastructure (metadata DB, object storage, workflow engine) stays in `sandbox.py` — the same boundary KubeRay and Temporal draw:

- Stateless services → Helm
- Stateful infrastructure → `sandbox.py`
- Cluster operators → unchanged

**Per-service toggles** — every service has an `enabled: true` flag following the Temporal Helm chart pattern. Disable any service at install time:

```bash
helm install michelangelo ./helm/michelangelo --set ui.enabled=false
```

Common headless patterns:
- `--set ui.enabled=false --set envoy.enabled=false` — API-only install
- `--set controllermgr.enabled=false` — external operator scenario
- `--set envoy.enabled=false` — bring your own Envoy or Istio gateway

---

## Chart Boundary

| Tier | Local k3d (managed by sandbox.py) | Staging / Production (managed by user) |
|------|------------------------------------|----------------------------------------|
| Metadata DB (MySQL / Postgres) | `kubectl apply` pod | RDS, Cloud SQL, etc. |
| Object Storage (MinIO / S3 / GCS) | `kubectl apply` pod with hostPath or PVC | S3, GCS, etc. |
| Cadence / Temporal | `helm install` by sandbox.py | Managed service |
| Observability (Prometheus, Grafana) | `kubectl apply` by sandbox.py | Existing cluster stack |
| Experimental (MLflow, Fluent Bit) | `kubectl apply` by sandbox.py | User's choice |
| KubeRay, Spark Operator | `helm install` by sandbox.py | User's cluster operators |

---

## Architecture

The sandbox deploys two tiers. The infrastructure tier is stateful and long-lived — it stays in `sandbox.py`. The control plane tier is stateless and frequently redeployed — it moves to Helm.

### Infrastructure Tier — stays in sandbox.py

- MySQL 8.0 / Postgres — backing store for CRDs and workflow engines
- MinIO / S3 / GCS — object store for artifacts, models, logs
- Cadence or Temporal — workflow engine (mutually exclusive)
- Prometheus + Grafana — observability (optional)
- MLflow + Fluent Bit — experimental (optional)
- KubeRay, Spark Operator — cluster operators

### Control Plane Tier — managed by Helm

- `michelangelo-apiserver` — gRPC API server (port 15566)
- `michelangelo-envoy` — gRPC-Web proxy bridging UI to API server (port 8081)
- `michelangelo-ui` — React frontend (port 8090)
- `michelangelo-worker` — Cadence/Temporal workflow client
- `michelangelo-controllermgr` — Kubernetes controller manager

---

## Chart Layout

Source files are migrated from `python/michelangelo/cli/sandbox/resources/` into Helm templates.

```
helm/
└── michelangelo/
    ├── Chart.yaml
    ├── values.yaml              # production defaults (ClusterIP, empty addresses)
    ├── values-k3d.yaml          # k3d overrides (NodePorts, local infra addresses)
    └── templates/
        ├── _helpers.tpl
        ├── crds/
        ├── rbac/
        │   ├── clusterrole.yaml
        │   ├── clusterrolebinding.yaml
        │   └── serviceaccount.yaml
        └── core/
            ├── controllermgr-deployment.yaml
            ├── controllermgr-configmap.yaml
            ├── controllermgr-service.yaml
            ├── apiserver-deployment.yaml
            ├── apiserver-service.yaml
            ├── envoy-configmap.yaml
            ├── ui-deployment.yaml
            ├── ui-service.yaml
            ├── ui-configmap.yaml
            ├── worker-deployment.yaml
            └── worker-configmap.yaml
```

`Chart.yaml`:

```yaml
apiVersion: v2
name: michelangelo
description: Michelangelo control plane — apiserver, envoy, UI, worker, controllermgr, CRDs, and RBAC
type: application
version: 0.1.0
appVersion: main
```

---

## Values

Two values files ship with the chart. `values.yaml` has production-safe defaults (ClusterIP, empty connection strings). `values-k3d.yaml` overrides for local k3d development.

### values.yaml — production defaults

```yaml
metadataStorage:
  driver: ""        # "mysql" or "postgres"
  host: ""          # e.g. "my-rds.example.com"
  port: 3306
  database: michelangelo
  rootPassword: ""

objectStorage:
  endpoint: ""      # "s3.amazonaws.com" | "minio:9000" | "storage.googleapis.com"
  secure: false

workflow:
  endpoint: ""      # "cadence.internal:7933" or "temporal-frontend:7233"
  engine: cadence   # "cadence" or "temporal"

images:
  apiserver:     ghcr.io/michelangelo-ai/apiserver:main
  worker:        ghcr.io/michelangelo-ai/worker:main
  ui:            ghcr.io/michelangelo-ai/ui:main
  controllermgr: ghcr.io/michelangelo-ai/controllermgr:main
  envoy:         envoyproxy/envoy:v1.29-latest
  pullPolicy:    IfNotPresent

apiserver:     { enabled: true, port: 15566, service: { type: ClusterIP, nodePort: null } }
envoy:         { enabled: true, port: 8081,  corsOrigins: "", service: { type: ClusterIP, nodePort: null } }
ui:            { enabled: true, apiBaseUrl: "", service: { type: ClusterIP, port: 80, nodePort: null } }
worker:        { enabled: true, replicas: 1 }
controllermgr: { enabled: true }
```

### values-k3d.yaml — local sandbox overrides

```yaml
metadataStorage:
  driver: mysql
  host: mysql
  port: 3306
  database: michelangelo
  rootPassword: root

objectStorage:
  endpoint: "minio:9000"
  secure: false

workflow:
  endpoint: "cadence:7933"   # or "temporal-frontend:7233" with --set workflow.engine=temporal
  engine: cadence

images:
  pullPolicy: IfNotPresent

apiserver: { service: { type: NodePort, nodePort: 30009 } }
envoy:     { corsOrigins: "http://localhost:[0-9]+", service: { type: NodePort, nodePort: 30010 } }
ui:        { apiBaseUrl: "http://localhost:8081", service: { type: NodePort, nodePort: 30011 } }
```

### Common install patterns

```bash
# Local k3d install
helm install michelangelo ./helm/michelangelo -f helm/michelangelo/values-k3d.yaml

# Headless — no UI or Envoy proxy
helm install michelangelo ./helm/michelangelo \
  -f helm/michelangelo/values-k3d.yaml \
  --set ui.enabled=false --set envoy.enabled=false

# Bring your own Envoy / Istio gateway
helm install michelangelo ./helm/michelangelo --set envoy.enabled=false

# Temporal workflow engine
helm install michelangelo ./helm/michelangelo \
  --set workflow.engine=temporal \
  --set workflow.endpoint=temporal-frontend:7233
```

---

## Key Design Decisions

### All pods promoted to Deployments

`apiserver`, `michelangelo-envoy`, and `worker` are currently bare Pod resources — no self-healing, no rolling updates. All three are promoted to Deployment. `ui` is already a Deployment.

### Schema init container

The ingester schema (13 CRD tables) moves from a ConfigMap + `kubectl apply` to an `initContainer` on the apiserver pod — eliminating the startup race condition currently requiring Python ordering.

- Image: `mysql:8.0` or `postgres:16` based on `metadataStorage.driver`
- Loops until schema SQL applies before apiserver starts

### object-storage-credentials Secret

The `object-storage-credentials` Secret holds `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` — the standard S3-compatible format used by MinIO, AWS S3, and GCS HMAC keys.

:::note
Renamed from `minio-credentials` in Phase 2. Four files are updated together: `resources/minio-credentials.yaml`, `resources/michelangelo-worker.yaml`, `resources/michelangelo-controllermgr.yaml`, and `sandbox.py`.
:::

### NodePorts via values-k3d.yaml

All NodePorts are defined in `values-k3d.yaml`, not hardcoded in templates. The k3d cluster is created with matching `--port` flags. Phase 4 will parse the YAML automatically to derive these flags.

### Generic storage keys

`metadataStorage` (not `mysql`) accepts MySQL or Postgres via a `driver` field. `objectStorage` (not `minio`) accepts any S3-compatible endpoint. The chart has no opinion on which backend is used.

### michelangelo-envoy: gRPC-Web bridge only

`michelangelo-envoy` is a purpose-built proxy for one job: translate gRPC-Web (browser) → gRPC (apiserver). Users with their own Envoy or Istio should set `envoy.enabled=false` and configure their proxy with three HTTP filters in order: `grpc_web → cors → router`. The cluster must use HTTP/2 (`http2_protocol_options: {}`) and route to `michelangelo-apiserver:15566`.

---

## Migration Phases

### Phase 1 — Infrastructure tier verification

- Confirm all infrastructure services remain as `kubectl apply` / `helm install` calls in `sandbox.py`
- Add CI gate: `helm lint` + `helm template --debug` on every PR touching `helm/`
- No chart files created — verification and CI gate only

### Phase 2 — michelangelo chart: all control plane services

- Create `helm/michelangelo/` with `Chart.yaml`, `values.yaml`, `values-k3d.yaml`
- Add `_helpers.tpl` and CRDs + RBAC with `watchNamespace` support
- Migrate all 5 control plane services (Pod → Deployment)
- Rename `minio-credentials` → `object-storage-credentials` across 4 files
- Apply `helm.sh/resource-policy: keep` to credential Secret
- Validate install + upgrade against k3d with infrastructure up

### Phase 3 — Observability + experimental

- Confirm Prometheus, Grafana, MLflow, Fluent Bit remain managed by `sandbox.py`
- Document the boundary in `docs/contributing/dev-environment.md`
- No chart changes in this phase

### Phase 4 — sandbox.py integration

- Replace `_deploy_services()` with `helm install michelangelo`
- Replace `_sync()` redeployment with `helm upgrade --reuse-values`
- Thread `--workflow`, `--exclude`, `--include-experimental` flags
- Parse `values-k3d.yaml` to derive k3d `--port` flags automatically
- Remove redundant per-service YAML code from Python
- Update `dev-environment.md` with Helm-based setup instructions

After Phase 4, `sandbox.py` becomes:

| Method | Before | After |
|--------|--------|-------|
| `_create()` | k3d create + raw YAML | k3d create + infra setup + `helm install michelangelo -f values-k3d.yaml` |
| `_sync()` | per-service redeploy | `helm upgrade michelangelo --reuse-values` |
| `_destroy()` | per-service delete | `helm uninstall michelangelo` + infra teardown + k3d delete |
| `_deploy_services()` | applies 5 YAML files | removed — replaced by Helm |

---

## What Stays in sandbox.py

Helm manages declarative Kubernetes state. These operations are inherently imperative and remain in `sandbox.py` in every phase.

| Operation | Why it stays in Python |
|-----------|----------------------|
| k3d cluster create/delete | Not a Kubernetes resource; requires k3d CLI |
| MySQL / Postgres pod setup | Infrastructure — out of chart scope |
| MinIO / object storage pod setup | Infrastructure — out of chart scope |
| Cadence Helm install | Infrastructure — out of chart scope |
| Temporal Helm install + schema migration | Infrastructure — out of chart scope |
| KubeRay, Spark Operator Helm install | Infrastructure — out of chart scope |
| Prometheus + Grafana | Observability tier — out of chart scope |
| MLflow, Fluent Bit | Experimental tier — out of chart scope |
| Compute cluster creation | Creates a second k3d cluster with its own kubeconfig, then wires cross-cluster RBAC |
| Compute cluster CRD + secrets | Depends on kubeconfig extracted from the second cluster after it is running |
| Istio + inference demo | Kept in Python until inference reaches GA |

---

## Next Steps

- [Platform Setup](./platform-setup.md) — ConfigMap reference for all control plane components
- [Register a Compute Cluster](./jobs/register-a-compute-cluster-to-michelangelo-control-plane.md) — connect a Kubernetes cluster for Ray/Spark job dispatch
- [Dev Environment Setup](../contributing/dev-environment.md) — local k3d sandbox setup
