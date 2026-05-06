---
sidebar_position: 2
sidebar_label: "Helm Chart"
---

# Michelangelo Helm Chart

Converting the ML control plane from raw `kubectl apply` to a first-class Helm chart installable against any Kubernetes cluster.

| Metric | Value |
|--------|-------|
| Control Plane Services | 5 |
| Chart Files | 30 |
| Migration Phases | 4 |
| K8s Resources | 21 |

---

## Overview

The sandbox today is orchestrated by `sandbox.py`, which applies raw YAML files via `kubectl apply` and calls external Helm charts for KubeRay, Spark Operator, and Temporal. This design converts the control plane into a first-class Helm chart so it can be installed, upgraded, and torn down with standard Helm commands.

| Chart | Installs | Target Audience |
|-------|----------|-----------------|
| `michelangelo` | All 5 control plane services + CRDs + RBAC | Any Kubernetes cluster — production, staging, or local k3d |
| `cadence` (optional subchart) | Cadence workflow engine (frontend, history, matching, worker, web UI) | Users who want a fully self-contained install without a managed Cadence service |

### Goals

**Single command install:**

```bash
helm install michelangelo ./helm/michelangelo \
  --set metadataStorage.host=... \
  --set objectStorage.endpoint=... \
  --set workflow.endpoint=...
```

**Local k3d shortcut** — after `sandbox.py` sets up infrastructure:

```bash
helm install michelangelo ./helm/michelangelo -f helm/michelangelo/values-k3d.yaml
```

### Chart Boundary

The `michelangelo` chart owns only the control plane. Infrastructure must exist before install — the chart accepts connection values pointing at it.

| Tier | Local k3d (sandbox.py) | Production (user-managed) |
|------|------------------------|---------------------------|
| MySQL | `kubectl apply` pod | RDS, Cloud SQL, etc. |
| MinIO / S3 | `kubectl apply` pod | S3, GCS, etc. |
| Cadence / Temporal | `helm install` by sandbox.py or optional subchart | Managed service |
| Observability | `kubectl apply` by sandbox.py | Existing cluster stack |
| Experimental (MLflow, Fluent Bit) | `kubectl apply` by sandbox.py | User's choice |
| KubeRay, Spark Operator | `helm install` by sandbox.py | User's cluster operators |

---

## Architecture

Three tiers, with clear ownership boundaries:

**Infrastructure tier** — Stateful, long-lived. Stays in `sandbox.py`. MySQL, MinIO, Cadence/Temporal.

**Control plane tier** — Stateless, frequently redeployed. Moves to Helm. All 5 services become Deployments:
- `michelangelo-apiserver` — gRPC API server (port 15566)
- `michelangelo-envoy` — gRPC-Web proxy (port 8081)
- `michelangelo-ui` — React frontend (port 80)
- `michelangelo-worker` — Cadence/Temporal workflow client
- `michelangelo-controllermgr` — Kubernetes controller manager

**Observability tier** — Optional. Prometheus + Grafana stay in `sandbox.py`.

---

## Chart Layout

```
helm/
└── michelangelo/
    ├── Chart.yaml               # includes optional cadence dependency
    ├── README.md
    ├── values.yaml              # production defaults (ClusterIP, empty addresses)
    ├── values-k3d.yaml          # k3d overrides (NodePorts, local infra addresses)
    ├── files/
    │   └── schema/
    │       └── mysql-init-schema.sql   # 578-line schema extracted from mysql-ingester.yaml
    ├── crds/                    # placeholder — CRDs self-register at apiserver startup
    └── templates/
        ├── _helpers.tpl
        ├── NOTES.txt            # post-install instructions, branches on Service type
        ├── rbac/
        │   ├── serviceaccount.yaml
        │   ├── clusterrole.yaml        # least-privilege (replaces boot.yaml cluster-admin)
        │   └── clusterrolebinding.yaml
        ├── tests/
        │   └── test-connection.yaml    # helm test hook
        └── core/                       # 14 templates for 5 services
            ├── apiserver-{deployment,service,configmap,schema-init-configmap}.yaml
            ├── envoy-{deployment,service,configmap}.yaml
            ├── ui-{deployment,service,configmap}.yaml
            ├── worker-{deployment,configmap}.yaml
            ├── controllermgr-{deployment,service,configmap}.yaml
            ├── metadata-storage-secret.yaml    # resource-policy: keep
            └── object-storage-secret.yaml      # resource-policy: keep
```

`Chart.yaml` — dependency declaration:

```yaml
apiVersion: v2
name: michelangelo
description: Michelangelo control plane — apiserver, envoy, UI, worker, controllermgr, CRDs, and RBAC
type: application
version: 0.1.0
appVersion: "0.2.1"
kubeVersion: ">=1.27.0-0"

dependencies:
  - name: cadence
    version: "1.1.0"
    repository: https://cadence-workflow.github.io/cadence-charts
    condition: cadence.enabled   # disabled by default
```

---

## Key Design Decisions

### All Pods → Deployments

`apiserver`, `envoy`, `worker`, and `controllermgr` were bare Pod resources — no self-healing, no rolling updates. All four are promoted to Deployment. `ui` was already a Deployment.

### Schema Init Container

The ingester schema (13 CRD tables) was applied via a standalone Job. It is now two init containers on the apiserver Pod — eliminating the ordering race condition in Python:

```yaml
initContainers:
  - name: wait-for-metadata-storage   # polls until MySQL/Postgres is ready
    image: mysql:8.0
    command: [sh, -c, "until mysqladmin ping -h $METADATA_HOST ..."]

  - name: schema-init                 # applies 578-line SQL idempotently
    image: mysql:8.0
    command: [sh, -c, "mysql -h $METADATA_HOST ... < /schema/init-schema.sql"]
    volumeMounts:
      - name: schema
        mountPath: /schema
```

### Credentials Idempotency

Both credential Secrets (`metadata-storage-secret` and `object-storage-secret`) are annotated `helm.sh/resource-policy: keep` — `helm uninstall` never destroys credentials that may have been injected externally by CI.

### Per-Service enabled Toggle

Every service has `enabled: true` in `values.yaml`. Templates wrap each Deployment + Service + ConfigMap in `{{- if .Values.<service>.enabled }}`, following the Temporal Helm chart pattern:

```bash
# Headless install — API-only, no browser UI
helm install michelangelo ./helm/michelangelo \
  -f helm/michelangelo/values-k3d.yaml \
  --set ui.enabled=false \
  --set envoy.enabled=false
```

### Fail-Fast Required Values

Every required value uses the `required` template function — `helm install` fails before creating any resource with a clear error pointing to the README:

```yaml
host: {{ required "metadataStorage.host is required — see README#values-reference"
         .Values.metadataStorage.host | quote }}
```

### Least-Privilege RBAC

Replaces `boot.yaml`'s `cluster-admin` grant with a scoped `ClusterRole` covering only what `controllermgr` and `apiserver` actually need: CRD lifecycle, Michelangelo CRs, KubeRay/Spark CRs, pods/services, configmaps/secrets, and leader-election leases.

### watchNamespace Support

When `controllermgr.watchNamespace` is empty (default), the chart emits a `ClusterRole` watching all namespaces. When set to a list, it emits a namespaced `Role` + `RoleBinding` per entry.

### Envoy Backend is Release-Scoped

The Envoy ConfigMap references `{{ include "michelangelo.fullname" . }}-apiserver` rather than the bare name — multiple installs in different namespaces don't collide.

---

## Cadence as Optional Subchart

The official `cadence-workflow/cadence` chart (v1.1.0) is declared as a conditional dependency. It is disabled by default so the `michelangelo` chart stays infrastructure-agnostic.

| Scenario | `cadence.enabled` |
|----------|-------------------|
| Bring your own Cadence service | `false` (default) |
| Using Temporal instead | `false` (default) |
| Self-contained install, no pre-existing workflow engine | `true` |

### Self-Contained Install

```bash
# Download the cadence subchart first
helm dependency update helm/michelangelo

# Install everything — control plane + Cadence
helm install michelangelo ./helm/michelangelo \
  --set cadence.enabled=true \
  --set workflow.engine=cadence \
  --set workflow.endpoint=michelangelo-cadence-frontend:7833 \
  --set metadataStorage.host=my-mysql.example.com \
  --set metadataStorage.rootPassword=secret \
  --set objectStorage.endpoint=s3.amazonaws.com \
  --set objectStorage.accessKeyId=AKID \
  --set objectStorage.secretAccessKey=SECRET \
  --set ui.apiBaseUrl=https://michelangelo.example.com/api
```

### Values Passthrough

Cadence subchart values are namespaced under `cadence:`. Key overrides share MySQL with the control plane but use a separate `cadence` database:

```yaml
cadence:
  enabled: false

  persistence:
    defaultStore: mysql
    mysql:
      driver: mysql
      host: ""           # same as metadataStorage.host
      port: 3306
      database: cadence  # separate database from "michelangelo"
      user: root
      password: ""

  web:
    enabled: true        # set false to skip the Cadence web UI pod
```

### Service Naming

When the subchart is installed as part of this release, the frontend Service is named `<release>-cadence-frontend`. Set `workflow.endpoint=<release>-cadence-frontend:7833` to match.

### Mutual Exclusivity with Temporal

`cadence.enabled=true` implies `workflow.engine=cadence`. The chart does not guard the combination — setting both `cadence.enabled=true` and `workflow.engine=temporal` installs Cadence but the worker ignores it.

### k3d Local Development — Unchanged

`values-k3d.yaml` keeps `cadence.enabled: false`. `sandbox.py` continues to install Cadence separately and the worker points at `cadence:7833` as before.

---

## Migration Phases

### Phase 1 — Infrastructure tier verification

- Confirm all infra services remain as `kubectl apply` / `helm install` in `sandbox.py`
- Add CI gate: `helm lint` + `helm template --debug` on every PR touching `helm/`
- No chart files created in this phase

### Phase 2 — michelangelo chart: all control plane services ✓ Done

- 30 chart files created under `helm/michelangelo/`
- All 5 services promoted from Pods to Deployments
- Schema init containers, least-privilege RBAC, credential Secrets with `resource-policy: keep`
- Validated: `helm lint` ✓ · `helm install` on k3d ✓ · `helm test` ✓ · `helm upgrade --reuse-values` ✓

### Phase 3 — Observability + Experimental: documentation

- Confirm Prometheus, Grafana, MLflow, Fluent Bit remain in `sandbox.py`
- Document the boundary in `docs/contributing/dev-environment.md`

### Phase 4 — sandbox.py integration

- Replace `_deploy_services()` with `helm install michelangelo -f values-k3d.yaml`
- Replace `_sync()` app redeployment with `helm upgrade --reuse-values`
- Thread `--workflow`, `--exclude`, `--include-experimental` flags to Helm `--set`
- Parse `values-k3d.yaml` in `_create()` to derive k3d `--port` flags automatically
- Rename `minio-credentials` → `object-storage-credentials` in `sandbox.py`

After Phase 4, `sandbox.py` becomes:

```
_create()          → k3d create + infra setup + helm install michelangelo -f values-k3d.yaml
_sync()            → k3d ensure + infra sync + helm upgrade michelangelo --reuse-values
_destroy()         → helm uninstall michelangelo + infra teardown + k3d delete
_deploy_services() → removed (replaced by Helm)
```

---

## What Stays in sandbox.py

Helm manages declarative Kubernetes state. These operations are inherently imperative and remain in Python:

| Operation | Why it stays in Python |
|-----------|------------------------|
| k3d cluster create/delete | Not a Kubernetes resource; requires k3d CLI |
| MySQL pod setup | Infrastructure — out of chart scope |
| MinIO pod setup | Infrastructure — out of chart scope |
| Cadence Helm install | Stays in `sandbox.py` for local k3d; optionally moved to chart via `cadence.enabled=true` for self-contained production installs |
| Temporal Helm install + schema migration | Infrastructure — out of chart scope |
| KubeRay, Spark Operator Helm install | Infrastructure — out of chart scope |
| Prometheus + Grafana | Observability tier — out of chart scope |
| MLflow, Fluent Bit | Experimental tier — out of chart scope |
| Compute cluster creation + RBAC | Creates a second k3d cluster; cross-cluster RBAC depends on runtime kubeconfig |
| Istio + inference demo | Kept in Python until inference reaches GA |

---

## Next Steps

- [Platform Setup](./platform-setup.md) — ConfigMap reference for all control plane components
- [Register a Compute Cluster](./jobs/register-a-compute-cluster-to-michelangelo-control-plane.md) — connect a Kubernetes cluster for Ray/Spark job dispatch
- [Dev Environment Setup](../contributing/dev-environment.md) — local k3d sandbox setup
