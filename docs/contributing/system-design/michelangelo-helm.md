# Helm Chart Plan: `michelangelo`

## Overview

The sandbox today is orchestrated by `python/michelangelo/cli/sandbox/sandbox.py`, which applies raw YAML files in sequence via `kubectl apply` and calls external Helm charts for KubeRay, Spark Operator, and Temporal. This plan describes converting the control plane into a first-class Helm chart so it can be installed, upgraded, and torn down with standard Helm commands against any cluster.

**Charts:**

| Chart | Installs | Target audience |
|---|---|---|
| `michelangelo` | All 5 control plane services + CRDs + RBAC | Any Kubernetes cluster — production, staging, or local k3d |
| `cadence` (optional subchart) | Cadence workflow engine (frontend, history, matching, worker, web UI) | Users who want a fully self-contained install without a managed Cadence service |
| `temporal` (optional subchart) | Temporal workflow engine | Self-contained installs choosing Temporal over Cadence |

**Goals:**
- `helm install michelangelo ./helm/michelangelo --set metadataStorage.host=... --set objectStorage.endpoint=... --set workflow.endpoint=...` — installs the full control plane against any cluster
- `helm install michelangelo ./helm/michelangelo -f helm/michelangelo/values-k3d.yaml` — installs against local k3d after `sandbox.py` sets up infrastructure
- `sandbox.py` manages infrastructure (MySQL, MinIO, Cadence/Temporal, Observability, Experimental) in every environment; it calls `helm install michelangelo` for the control plane instead of applying raw YAML

**Chart boundary:**

The `michelangelo` chart owns only the control plane. It assumes infrastructure already exists and accepts connection values pointing at it — the same boundary KubeRay and Temporal draw.

| Tier | Local k3d — managed by `sandbox.py` | Staging / Production — managed by user |
|---|---|---|
| MySQL | `kubectl apply` pod (as today) | RDS, Cloud SQL, etc. |
| MinIO / S3 | `kubectl apply` pod with hostPath or PVC (as today) | S3, GCS, etc. |
| Cadence / Temporal | `helm install` by `sandbox.py` (as today), or optional `cadence`/`temporal` subcharts | Managed service |
| Observability (Prometheus, Grafana) | `kubectl apply` by `sandbox.py` (as today) | Existing cluster stack |
| Experimental (MLflow, Fluent Bit) | `kubectl apply` by `sandbox.py` (as today) | User's choice |
| KubeRay, Spark Operator | `helm install` by `sandbox.py` (as today) | User's cluster operators |

---

## Current Architecture

The sandbox deploys two tiers of services:

**Infrastructure tier** (stateful, long-lived — stays in `sandbox.py`)
- MySQL 8.0 — backing store for all Michelangelo CRDs and workflow engines
- MinIO — S3-compatible object store for artifacts, models, logs
- Cadence or Temporal — workflow engine (mutually exclusive, chosen at install time)

**Control plane tier** (stateless, frequently redeployed — moves to Helm)
- `michelangelo-apiserver` — gRPC API server (port 15566)
- `michelangelo-envoy` — gRPC-Web proxy bridging UI to API server (port 8081)
- `michelangelo-ui` — React frontend (port 8090)
- `michelangelo-worker` — Cadence/Temporal workflow client
- `michelangelo-controllermgr` — Kubernetes controller manager

**Observability** (optional — stays in `sandbox.py`)
- Prometheus + Grafana

**Experimental** (optional — stays in `sandbox.py`)
- MLflow tracking server + nginx proxy
- Fluent Bit DaemonSet (log shipping to MinIO)

**Operator subcharts** (installed by `sandbox.py` — unchanged)
- KubeRay Operator (ray-system namespace)
- Spark Operator (spark-operator namespace)
- Temporal (when `workflow: temporal`)

---

## Proposed Chart Layout

```
helm/
└── michelangelo/
    ├── Chart.yaml               # includes optional cadence dependency
    ├── Chart.lock               # pins cadence-workflow/cadence v1.1.0 (committed)
    ├── values.yaml              # production defaults (ClusterIP, empty addresses)
    ├── values-k3d.yaml          # k3d overrides (NodePorts, local infra addresses)
    └── templates/
        ├── _helpers.tpl
        ├── NOTES.txt
        ├── crds/
        ├── rbac/
        │   ├── clusterrole.yaml
        │   ├── clusterrolebinding.yaml
        │   └── serviceaccount.yaml
        ├── tests/
        │   └── test-connection.yaml
        └── core/
            ├── controllermgr-deployment.yaml
            ├── controllermgr-configmap.yaml
            ├── controllermgr-service.yaml
            ├── apiserver-deployment.yaml   # conditional TLS volume + grpc/grpcs port
            ├── apiserver-service.yaml      # grpc/grpcs port name conditional on TLS
            ├── apiserver-configmap.yaml
            ├── apiserver-schema-init-configmap.yaml
            ├── apiserver-ingress.yaml      # Phase B gRPC Ingress (passthrough|grpc mode)
            ├── envoy-deployment.yaml
            ├── envoy-service.yaml
            ├── envoy-configmap.yaml        # conditional UpstreamTlsContext when apiserver TLS
            ├── envoy-ingress.yaml          # Phase A gRPC-Web Ingress
            ├── ui-deployment.yaml
            ├── ui-service.yaml
            ├── ui-configmap.yaml           # auto-derives apiBaseUrl from envoy ingress
            ├── ui-ingress.yaml             # Phase A HTTP Ingress
            ├── metadata-storage-secret.yaml
            ├── object-storage-secret.yaml
            ├── worker-deployment.yaml
            └── worker-configmap.yaml
```

Source files being migrated from `python/michelangelo/cli/sandbox/resources/`:
- `michelangelo-apiserver.yaml`
- `michelangelo-envoy.yaml`
- `michelangelo-ui.yaml`
- `michelangelo-worker.yaml`
- `michelangelo-controllermgr.yaml`
- `michelangelo-config.yaml`

---

## `Chart.yaml`

```yaml
apiVersion: v2
name: michelangelo
description: Michelangelo control plane — apiserver, envoy, UI, worker, controllermgr, CRDs, and RBAC
type: application
version: 0.1.0
appVersion: main

dependencies:
  - name: cadence
    version: "1.1.0"
    repository: https://cadence-workflow.github.io/cadence-charts
    condition: cadence.enabled   # disabled by default; enable when no external Cadence is available
  - name: temporal
    version: "0.44.0"
    repository: https://go.temporal.io/helm-charts
    condition: temporal.enabled  # disabled by default; enable when no external Temporal is available
```

Both subcharts are **disabled by default** (`cadence.enabled: false`, `temporal.enabled: false`).
Users who bring their own Cadence or Temporal service leave both disabled and point
`workflow.endpoint` at their existing service. Users who want a fully self-contained install enable
exactly one — `cadence.enabled=true` **or** `temporal.enabled=true`, never both.

Run `helm dependency update helm/michelangelo` after adding or updating the dependency to
download the chart into `helm/michelangelo/charts/`.

---

## `values.yaml` (abbreviated)

### `helm/michelangelo/values.yaml` — production defaults

```yaml
# Caller must provide these — no defaults
metadataStorage:
  driver: mysql     # "mysql" or "postgres"
  host: ""          # e.g. "my-rds.example.com" (MySQL/RDS) or "my-pg.example.com" (Postgres/Cloud SQL)
  port: 3306
  database: michelangelo
  rootPassword: ""

objectStorage:
  endpoint: ""      # e.g. "s3.amazonaws.com" (S3), "minio:9000" (MinIO), "storage.googleapis.com" (GCS)
  secure: false

workflow:
  endpoint: ""      # e.g. "cadence.internal:7933" or "temporal-frontend:7233"
  engine: cadence   # "cadence" or "temporal"

images:
  apiserver:     ghcr.io/michelangelo-ai/apiserver:main
  worker:        ghcr.io/michelangelo-ai/worker:main
  ui:            ghcr.io/michelangelo-ai/ui:main
  controllermgr: ghcr.io/michelangelo-ai/controllermgr:main
  envoy:         envoyproxy/envoy:v1.29-latest
  pullPolicy:    IfNotPresent

apiserver:
  enabled: true
  port: 15566
  service:
    type: ClusterIP
    nodePort: null

envoy:
  enabled: true
  port: 8081
  corsOrigins: ""   # set per environment
  service:
    type: ClusterIP
    nodePort: null

ui:
  enabled: true
  apiBaseUrl: ""    # e.g. "https://ma.internal/api"
  service:
    type: ClusterIP
    port: 80
    nodePort: null

worker:
  enabled: true
  replicas: 1

controllermgr:
  enabled: true
```

### `helm/michelangelo/values-k3d.yaml` — local sandbox overrides

Committed alongside the chart. `sandbox.py` passes this with `-f` after setting up infrastructure.
Addresses match the services `sandbox.py` already sets up via kubectl.

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
  endpoint: "cadence:7933"   # overridden to "temporal-frontend:7233" when --workflow temporal
  engine: cadence

images:
  pullPolicy: IfNotPresent

apiserver:
  service:
    type: NodePort
    nodePort: 30009

envoy:
  corsOrigins: "http://localhost:[0-9]+"
  service:
    type: NodePort
    nodePort: 30010

ui:
  apiBaseUrl: "http://localhost:8081"
  service:
    type: NodePort
    nodePort: 30011
```

Individual services can be disabled at install time using `--set`:

```bash
# Headless install — no UI or Envoy proxy
helm install michelangelo ./helm/michelangelo \
  -f helm/michelangelo/values-k3d.yaml \
  --set ui.enabled=false \
  --set envoy.enabled=false
```

Cadence and Temporal worker configs are mutually guarded in templates:

```yaml
{{- if eq .Values.workflow.engine "cadence" }}
# cadence provider
{{- end }}
{{- if eq .Values.workflow.engine "temporal" }}
# temporal provider
{{- end }}
```

---

## Key Design Decisions

### Cadence as Optional Subchart

The official [Cadence Helm chart](https://github.com/cadence-workflow/cadence-charts) (`cadence-workflow/cadence`,
current version `1.1.0`) is declared as a conditional dependency. It is disabled by default so the
`michelangelo` chart stays infrastructure-agnostic (matching the same boundary KubeRay and Temporal draw).

**When to enable:**

| Scenario | `cadence.enabled` |
|---|---|
| User brings their own Cadence service | `false` (default) |
| User brings Temporal | `false` |
| Self-contained install with no pre-existing workflow engine | `true` |

**Install with embedded Cadence (self-contained):**

```bash
helm dependency update helm/michelangelo   # download cadence chart into charts/

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

**Values passthrough to the Cadence subchart:**

Cadence subchart values are namespaced under `cadence:` in `values.yaml`. Key overrides:

```yaml
cadence:
  enabled: false   # set true to install Cadence alongside the control plane

  # Point Cadence at the same MySQL instance the control plane uses.
  # The Cadence chart creates its own schema in a separate database ("cadence").
  persistence:
    defaultStore: mysql
    additionalStores: {}
    mysql:
      driver: "mysql"
      host: ""           # same as metadataStorage.host
      port: 3306
      database: cadence  # separate database from "michelangelo"
      user: root
      password: ""       # same as metadataStorage.rootPassword
      existingSecret: ""

  # Disable the Cadence web UI if you don't need it (saves a pod).
  web:
    enabled: true
```

**Service name convention:** When the Cadence subchart is installed as part of this chart release,
its frontend Service is named `<release>-cadence-frontend`. The `workflow.endpoint` value must
match — set it to `<release>-cadence-frontend:7833` for gRPC.

**Mutual exclusivity with Temporal:** The `cadence.enabled` subchart and the `temporal.enabled`
subchart are mutually exclusive. The chart does not guard this combination — if a user sets both
`cadence.enabled=true` and `workflow.engine=temporal` (or both subcharts on at once), the unused
subchart still installs but the worker ignores it. The `NOTES.txt` warns when `workflow.engine` and
the enabled subchart disagree. Document in README that `cadence.enabled=true` implies
`workflow.engine=cadence`, and `temporal.enabled=true` implies `workflow.engine=temporal`.

**k3d local development:** The sandbox continues to install Cadence via `sandbox.py` using the
pre-existing `helm install` call. The subchart is not used in the local k3d flow — `values-k3d.yaml`
keeps `cadence.enabled: false` and `workflow.endpoint: cadence:7833` pointing at the
sandbox-managed Cadence service.

### Temporal as Optional Subchart

The official [Temporal Helm chart](https://github.com/temporalio/helm-charts)
(`temporalio/temporal`, current version `0.44.0`) is declared as a conditional dependency in
parallel with Cadence. It is disabled by default for the same reason — `michelangelo` stays
infrastructure-agnostic.

**When to enable:**

| Scenario | `temporal.enabled` |
|---|---|
| User brings their own Temporal service | `false` (default) |
| User brings Cadence | `false` |
| Self-contained install with no pre-existing workflow engine and Temporal preferred | `true` |

**Install with embedded Temporal (self-contained):**

```bash
helm dependency update helm/michelangelo   # download temporal chart into charts/

helm install michelangelo ./helm/michelangelo \
  --set temporal.enabled=true \
  --set workflow.engine=temporal \
  --set workflow.endpoint=michelangelo-temporal-frontend:7233 \
  --set temporal.server.config.persistence.default.sql.host=my-mysql.example.com \
  --set temporal.server.config.persistence.default.sql.password=secret \
  --set temporal.server.config.persistence.visibility.sql.host=my-mysql.example.com \
  --set temporal.server.config.persistence.visibility.sql.password=secret \
  --set metadataStorage.host=my-mysql.example.com \
  --set metadataStorage.rootPassword=secret \
  --set objectStorage.endpoint=s3.amazonaws.com \
  --set objectStorage.accessKeyId=AKID \
  --set objectStorage.secretAccessKey=SECRET \
  --set ui.apiBaseUrl=https://michelangelo.example.com/api
```

**Values passthrough to the Temporal subchart:**

Temporal subchart values are namespaced under `temporal:` in `values.yaml`. Key overrides:

```yaml
temporal:
  enabled: false   # set true to install Temporal alongside the control plane

  server:
    config:
      persistence:
        # Temporal creates TWO databases: `temporal` (history) and
        # `temporal_visibility` (search/list). Both auto-created by the schema job
        # when the MySQL user has CREATE DATABASE privilege.
        default:
          driver: sql
          sql:
            driver: mysql8     # IMPORTANT — see warning below
            host: ""           # same as metadataStorage.host
            port: 3306
            database: temporal
            user: root
            password: ""       # same as metadataStorage.rootPassword
        visibility:
          driver: sql
          sql:
            driver: mysql8
            host: ""
            port: 3306
            database: temporal_visibility
            user: root
            password: ""

  # Disable bundled backends — bring our own MySQL.
  cassandra: { enabled: false }
  mysql:     { enabled: false }
  postgresql: { enabled: false }
  elasticsearch: { enabled: false }

  # Temporal web UI. Useful for inspecting runs.
  web:
    enabled: true
```

> **WARNING — `mysql8`, not `mysql`.** The Temporal chart uses driver string `"mysql8"` for
> MySQL 8.x. The Cadence chart uses `"mysql"`. **Setting `driver: "mysql"` for Temporal is the
> #1 silent failure** — the schema tool falls back to a deprecated code path and the workflow
> engine starts but cannot read or write history correctly. Always use `mysql8` for the Temporal
> subchart's `default.sql.driver` and `visibility.sql.driver`.

**Service name convention:** When the Temporal subchart is installed as part of this chart release,
its frontend Service is named `<release>-temporal-frontend`. The `workflow.endpoint` value must
match — set it to `<release>-temporal-frontend:7233` (note: port `7233`, **not** Cadence's `7833`).

**Mutual exclusivity:** see the note in the Cadence section above — `cadence.enabled` and
`temporal.enabled` must not both be `true`.

**k3d local development:** As with Cadence, the subchart is not used in the local k3d flow.
`sandbox.py` continues to install Temporal directly via the pre-existing `helm install` call when
invoked with `--workflow temporal`.

### All Pods → Deployments

`michelangelo-apiserver`, `envoy`, and `michelangelo-worker` are currently bare `Pod` resources —
no self-healing, no rolling updates. All three are promoted to `Deployment`. `michelangelo-ui` is
already a `Deployment`.

### Schema Init Container (replacing kubectl apply)

The ingester schema (13 CRD tables in the `michelangelo` database) is currently applied via a ConfigMap + `kubectl apply`. This becomes an init container on the `michelangelo-apiserver` Pod:

```yaml
initContainers:
  - name: schema-init
    image: mysql:8.0   # or postgres:16 when .Values.metadataStorage.driver is "postgres"
    command: [sh, -c, "until mysql -h {{ .Values.metadataStorage.host }} -uroot -p$METADATA_ROOT_PASSWORD {{ .Values.metadataStorage.database }} < /schema/schema.sql; do sleep 2; done"]
    env:
      - name: METADATA_ROOT_PASSWORD
        value: {{ .Values.metadataStorage.rootPassword }}
    volumeMounts:
      - name: ingester-schema
        mountPath: /schema
```

This keeps the apiserver from starting until the schema is ready, eliminating the race condition that currently requires ordering in Python.

### Credentials Idempotency

The `_sync()` path in `sandbox.py` deliberately preserves the `object-storage-credentials` Secret across redeploys (CI may pre-populate it with real S3/GCS credentials). The chart mirrors this with:

```yaml
metadata:
  annotations:
    helm.sh/resource-policy: keep
```

This prevents `helm upgrade` from rotating credentials that were injected externally.

**Rename from `minio-credentials`:** The Secret was previously named `minio-credentials`. It is renamed to `object-storage-credentials` for consistency with the generic `objectStorage` values key — a Secret named after MinIO is misleading when the chart is configured against AWS S3 or GCS. The Secret contents remain unchanged (`AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` are already the standard S3-compatible format used by MinIO, S3, and GCS HMAC keys alike). This rename requires coordinated changes in Phase 2 to:
- `resources/minio-credentials.yaml` (file + `metadata.name`)
- `resources/michelangelo-worker.yaml` (`secretRef.name`)
- `resources/michelangelo-controllermgr.yaml` (`secretRef.name`)
- `sandbox.py` — `_ensure_credentials_secret()` and `_sync_config_from_secret()`

### RBAC: Wildcard for `michelangelo.api` Resources

The ClusterRole uses `resources: ["*"]` for the `michelangelo.api` API group rather than an
explicit resource list:

```yaml
- apiGroups: ["michelangelo.api"]
  resources: ["*"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["michelangelo.api"]
  resources: ["*/status"]
  verbs: ["get", "update", "patch"]
```

**Why a wildcard is safe here:** `michelangelo.api` is a private API group owned entirely by
Michelangelo. Every CRD in it is a Michelangelo resource that the controller manager and apiserver
legitimately need to access. A wildcard on a private group does not increase the blast radius
compared to an explicit list — the distinction matters for shared groups like `""` (core) or
`apps`, where a wildcard would grant access to resources owned by other workloads.

**Why not an explicit list:** The apiserver registers CRDs dynamically at startup via `crdSync`.
New CRDs (such as `cachedoutputs`, which was added after the initial chart) would require a chart
version bump to add to an explicit list. A missing entry causes an immediately visible
`permission-denied` error at runtime. Using `["*"]` on the private group avoids this toil and
keeps the RBAC in sync with the actual CRD set without manual maintenance.

**Contrast with other API groups:** All other groups in the ClusterRole (`ray.io`,
`sparkoperator.k8s.io`, `apps`, `""`, `coordination.k8s.io`, etc.) use explicit resource lists
because those are shared groups where least-privilege still matters.

### NodePorts via values-k3d.yaml

All NodePorts are defined in `values-k3d.yaml`, not hardcoded in templates. The k3d cluster is
created by `sandbox.py` with `--port` flags that publish those same NodePorts to localhost — the
values must stay in sync. A future improvement would have `sandbox.py` read `values-k3d.yaml` to
derive the port list automatically.

### Service Type Abstracted

All services default to `ClusterIP` in `values.yaml` (production-safe). `values-k3d.yaml` overrides
to `NodePort` for local access through k3d's published ports. Production and staging installs get
clean ClusterIP services with no external exposure by default.

### Per-Service `enabled` Toggle (Temporal pattern)

Every control plane service has an `enabled: true` boolean in `values.yaml`. All five default to
`true`. Templates wrap each Deployment + Service + ConfigMap in `{{- if .Values.<service>.enabled }}`.

This follows the same pattern as the [Temporal Helm chart](https://github.com/temporalio/helm-charts),
which toggles `web.enabled`, `admintools.enabled`, and individual `server.*` components the same way.

Use cases:
- `--set ui.enabled=false --set envoy.enabled=false` — headless/API-only install (no browser UI)
- `--set controllermgr.enabled=false` — disable the controller manager in clusters where an external operator is used
- `--set worker.enabled=false` — install without a workflow client (useful when the workflow engine is not yet provisioned)

No subchart `condition:` field in `Chart.yaml` is needed — all conditional logic stays in `values.yaml`.

### ConfigMaps Generated from Values

All hardcoded service addresses (`cadence:7833`, `minio:9091`, `michelangelo-apiserver:15566`) in
the current resource YAML files are replaced with value references. This makes the templates work
in any cluster without manual editing.

### michelangelo-config ConfigMap Absorbed

The global env ConfigMap (`michelangelo-config.yaml`, which sets `MA_FILE_SYSTEM`, `AWS_*`) is
replaced by `envFrom: secretRef: object-storage-credentials` on each service that needs it (worker,
controllermgr). No separate global ConfigMap needed.

### public-config Moved

The `public-config` ConfigMap (UI runtime config) is currently defined inside
`michelangelo-apiserver.yaml`. It is separated into `ui-configmap.yaml` where it belongs.

### Ingress Support (Phase A + Phase B)

External access follows three distinct paths requiring different Ingress strategies:

```
Browser          →  Ingress → UI (/)          HTTP/1.1, standard
Browser API      →  Ingress → Envoy (/api)    gRPC-Web (HTTP/1.1 compatible)
ma CLI / SDK     →  Ingress → Apiserver       raw gRPC (HTTP/2, separate hostname)
```

All Ingress resources are **disabled by default** and follow the same `enabled` toggle pattern
as other chart features.

#### Phase A — UI + Envoy Ingress (`ui-ingress.yaml`, `envoy-ingress.yaml`)

Both are standard `networking.k8s.io/v1` Ingress resources. The chart requires Kubernetes 1.27+
so no version branching is needed.

Key values:

```yaml
ui:
  ingress:
    enabled: false
    ingressClassName: ""          # nginx, traefik, alb — per-service for flexibility
    hostname: ""                  # e.g. "michelangelo.example.com"
    annotations: {}
    tls: []                       # [{ secretName, hosts[] }] array-of-objects

envoy:
  ingress:
    enabled: false
    ingressClassName: ""
    hostname: ""                  # can share UI hostname (path-split) or use subdomain
    path: "/"                     # use "/api" with rewrite annotation for path-split
    pathType: Prefix
    annotations: {}
    tls: []
```

gRPC-Web (envoy) is HTTP/1.1 compatible — no special backend-protocol annotation is needed,
unlike raw gRPC. Operators path-splitting on a shared hostname must add a rewrite annotation:

```yaml
# nginx path-split example
envoy:
  ingress:
    hostname: michelangelo.example.com
    path: "/api(/|$)(.*)"
    annotations:
      nginx.ingress.kubernetes.io/rewrite-target: /$2
      nginx.ingress.kubernetes.io/use-regex: "true"
```

**`apiBaseUrl` auto-derivation:** `ui-configmap.yaml` derives `apiBaseUrl` automatically when
`envoy.ingress.enabled=true`, `envoy.ingress.hostname` is set, and `ui.apiBaseUrl` is empty.
The scheme is `https` when `envoy.ingress.tls` is non-empty, `http` otherwise. Explicit
`ui.apiBaseUrl` always takes precedence.

**`corsOrigins` interaction:** the `envoy.corsOrigins` regex must match the public UI hostname
when behind Ingress — the `Origin:` header is preserved through the Ingress proxy. Set it to
match the UI's public origin (e.g. `"https://michelangelo\\.example\\.com"`).

#### Phase B — Apiserver TLS + gRPC Ingress (`apiserver-ingress.yaml`)

Raw gRPC requires HTTP/2, which most Ingress controllers do not proxy by default. Two modes:

| Mode | How it works | Requirement |
|---|---|---|
| `passthrough` | Controller forwards raw TLS bytes; pod terminates TLS | `apiserver.tls.enabled=true` (pod must serve TLS) |
| `grpc` | Controller terminates TLS, proxies H2C to pod; nginx needs `backend-protocol: GRPC` annotation | No pod TLS required (but recommended for production) |

Mode-specific annotations are **injected automatically** by the template:

```yaml
# passthrough → nginx.ingress.kubernetes.io/ssl-passthrough: "true"
# grpc        → nginx.ingress.kubernetes.io/backend-protocol: GRPC
```

The apiserver must use a **separate hostname** from UI/Envoy — gRPC (HTTP/2) and HTTP/1.1
cannot share the same Ingress listener reliably.

```yaml
apiserver:
  tls:
    enabled: false
    secretName: ""          # kubernetes.io/tls Secret with tls.crt + tls.key
    mountPath: /tls         # mount path inside apiserver container

  ingress:
    enabled: false
    mode: passthrough       # "passthrough" or "grpc"
    ingressClassName: ""
    hostname: ""            # e.g. "grpc.michelangelo.example.com" — must differ from UI/Envoy
    annotations: {}
    tls: []
```

**Coupling:** when `apiserver.tls.enabled=true`, three things change together:
1. Port name becomes `grpcs` in both Service and Deployment (was `grpc`)
2. TLS Secret is volume-mounted into the apiserver container at `apiserver.tls.mountPath`
3. `envoy-configmap.yaml` injects an `UpstreamTlsContext` on the apiserver cluster so envoy
   connects via TLS rather than cleartext H2C

**Phase A and Phase B are independent** — you can enable UI/Envoy Ingress without apiserver
Ingress, and vice versa.

#### Known Gap — Temporal Subchart

Resolved — the Temporal optional subchart is now implemented in parallel with Cadence. See the
[Temporal as Optional Subchart](#temporal-as-optional-subchart) section above for chart coordinates,
the `mysql8` driver requirement, and the `<release>-temporal-frontend:7233` service convention.

### Envoy Backend is Release-Scoped

The Envoy ConfigMap uses `{{ include "michelangelo.fullname" . }}-apiserver` rather than the bare
`michelangelo-apiserver` name so that multiple installs in different namespaces don't collide.

---

## Migration Phases

### Phase 1 — Infrastructure tier (no change to Python)

- Confirm all infrastructure services (MySQL, MinIO, Prometheus, Grafana, MLflow, Fluent Bit)
  remain as `kubectl apply` / `helm install` calls in `sandbox.py`
- CI gate: `.github/workflows/helm-lint.yaml` — ✓ **IMPLEMENTED** (PR #1160). Triggered on
  `paths: ['helm/**']`, runs `helm dependency update`, `helm lint`, and 6 `helm template` scenarios
  including subchart-enabled and mutual-exclusivity guard tests.
- No chart files created in this phase — this is a verification and CI gate phase only

### Phase 2 — `michelangelo` chart (all control plane services)

- Create `helm/michelangelo/` with `Chart.yaml`, `values.yaml`, `values-k3d.yaml`
- Both optional workflow subcharts implemented: `cadence` (1.1.0) and `temporal` (0.44.0), each
  gated by its own `*.enabled` condition and disabled by default
- Add `_helpers.tpl` with `michelangelo.fullname` helper
- Add CRDs in `templates/crds/`
- Add RBAC: ClusterRole + ClusterRoleBinding + ServiceAccount with `watchNamespace` support
- Migrate all 5 control plane services into `templates/core/`:
  - `apiserver-deployment.yaml` (Pod → Deployment, add schema-init container, add object-storage-credentials secretRef)
  - `apiserver-configmap.yaml` (generated from values)
  - `apiserver-service.yaml` (type from values)
  - `envoy-deployment.yaml` (Pod → Deployment)
  - `envoy-configmap.yaml` (backend address + CORS origins from values)
  - `envoy-service.yaml` (type from values)
  - `ui-deployment.yaml` (apiBaseUrl from values)
  - `ui-configmap.yaml` (moved from apiserver.yaml)
  - `ui-service.yaml` (type from values)
  - `worker-deployment.yaml` (Pod → Deployment, workflow engine guard)
  - `worker-configmap.yaml` (all endpoints from values, cadence/temporal guard)
  - `controllermgr-deployment.yaml` (all endpoints from values)
  - `controllermgr-configmap.yaml` (generated from values)
  - `controllermgr-service.yaml`
- Apply `helm.sh/resource-policy: keep` to credential Secrets
- Validate: `helm install michelangelo ./helm/michelangelo -f helm/michelangelo/values-k3d.yaml`
  works against k3d with infrastructure already up
- Validate: `helm upgrade --reuse-values` works

### Phase 3 — Observability + Experimental (no chart changes)

- Confirm Prometheus, Grafana, MLflow, Fluent Bit remain managed by `sandbox.py`
- Document the boundary explicitly in `docs/contributing/dev-environment.md`

### Phase 4 — `sandbox.py` integration

- Replace `_deploy_services()` body with `helm install michelangelo -f values-k3d.yaml`
- Replace `_sync()` app redeployment with `helm upgrade michelangelo --reuse-values`
- Thread `--workflow`, `--exclude`, `--include-experimental` flags:
  - `--workflow temporal` → `--set workflow.engine=temporal --set workflow.endpoint=michelangelo-temporal-frontend:7233 --set temporal.enabled=true --set cadence.enabled=false`
  - `--workflow cadence` (default) → `--set workflow.engine=cadence --set workflow.endpoint=michelangelo-cadence-frontend:7833 --set cadence.enabled=true --set temporal.enabled=false`
  - `--exclude apiserver` → `--set apiserver.enabled=false` (enabled guards already in templates per the Per-Service Toggle design decision)
- **Cadence migrated to Helm subchart in k3d:** `cadence.enabled=true` is set in `values-k3d.yaml` so Cadence is always part of `helm install michelangelo` for the Cadence workflow. The bare `cadence.yaml` Pod is removed.
- **Temporal migrated to Helm subchart in k3d:** `temporal.enabled=true` is passed dynamically via `--set` when `--workflow temporal`. Temporal's schema migration runs automatically via the subchart's schema-server Job. The separate `temporaltest` Helm release is eliminated.
- **Cadence cron schedule updates (PR #1128):** `updateCronScheduleIfChanged()` in the triggerrun
  controller calls `WorkflowClient.UpdateTrigger()` at runtime. The Cadence implementation is a
  **silent no-op** — it returns `nil` without updating (Cadence embeds cron schedules in the
  workflow definition at start time; in-place updates are not supported). The Temporal implementation
  uses `ScheduleHandle.Update()` and works correctly. Operators running `workflow.engine=cadence`
  who change a trigger's cron schedule must restart the trigger workflow manually for the change
  to take effect. No chart changes are required; this is a runtime behaviour difference, not a
  configuration difference.
- **Port synchronization (implemented):** `sandbox.py` reads control plane NodePorts from
  `values-k3d.yaml` at cluster-create time via `_helm_chart_ports(workflow)` rather than keeping
  a duplicate hardcoded list. `values-k3d.yaml` is the single source of truth. Cadence Web
  NodePort (30004) is also read from the chart now that Cadence runs as a subchart in k3d.
- Remove now-redundant per-service YAML application code from Python
- `sandbox.py sync` remains the CI entry point; CI scripts require no changes
- **Integration test timeout (PR #1156):** The CLI e2e test suite (`functional/cli/test_cli.py`)
  runs `ma pipeline dev-run` on the bert-cola example and takes ~15 minutes. Total integration
  test job timeout is 90 minutes. When Phase 4 replaces `_sync()` with `helm upgrade --reuse-values`,
  benchmark the upgrade overhead to confirm total job time stays within the limit.
- Update `docs/contributing/dev-environment.md` with Helm-based setup instructions

---

## What Stays in `sandbox.py`

Helm manages declarative Kubernetes state. These operations are inherently imperative and remain in
`sandbox.py` in every environment:

| Operation | Why it stays in Python |
|---|---|
| k3d cluster create/delete | Not a Kubernetes resource; requires `k3d` CLI |
| MySQL pod setup | Infrastructure — out of chart scope |
| MinIO pod setup (hostPath or PVC) | Infrastructure — out of chart scope |
| Cadence | **Moved to Helm subchart in k3d** (`cadence.enabled=true` in `values-k3d.yaml`). Cadence is now installed as part of `helm install michelangelo` in every environment. Domain registration still runs via `kubectl run` inside the cluster after `helm install` completes. Frontend ports (7833/7933) accessed via `kubectl port-forward`. Cadence Web exposed via NodePort 30004 → host port 8088. |
| Temporal | **Moved to Helm subchart in k3d** (`temporal.enabled=true` set dynamically by `_build_helm_set_args()` when `--workflow temporal`). Temporal is now installed as part of `helm install michelangelo --set temporal.enabled=true`. Schema migration runs automatically via the subchart's schema-server Job. Temporal Web exposed via NodePort 30005 → host port 8080. The `temporaltest` separate Helm release is no longer used. |
| KubeRay, Spark Operator Helm install | Infrastructure — out of chart scope |
| Prometheus + Grafana | Observability tier — out of chart scope |
| MLflow, Fluent Bit | Experimental tier — out of chart scope |
| Compute cluster creation | Creates a second k3d cluster with its own kubeconfig, then wires cross-cluster RBAC |
| Compute cluster CRD + secrets | Depends on kubeconfig extracted from the second cluster after it is running |
| Istio + inference demo | Kept in Python until inference reaches GA |

After this work, `sandbox.py` becomes:

```
_create()          → k3d create + infra setup + helm install michelangelo -f values-k3d.yaml
_sync()            → k3d ensure + infra sync + helm upgrade michelangelo --reuse-values
_destroy()         → helm uninstall michelangelo + infra teardown + k3d delete
_deploy_services() → removed (replaced by Helm)
```

---

## Resolved Decisions

### 1. Namespace: use `{{ .Release.Namespace }}`

Both KubeRay and Temporal use `{{ .Release.Namespace }}` everywhere — no chart creates or hardcodes
a namespace. The `michelangelo` chart uses `{{ .Release.Namespace }}` for all resources. The sandbox
defaults to `default` (matching current behavior) unless the user overrides with `-n`.

```yaml
metadata:
  namespace: {{ .Release.Namespace }}
```

For cross-namespace operator watching (KubeRay pattern), support a `watchNamespace` list in
`values.yaml` — when empty, the operator watches all namespaces via ClusterRole; when set, it uses
a namespaced Role per listed namespace.

### 2. Temporal schema migration stays in `sandbox.py`

Temporal schema migration (running `temporal-sql-tool` via `kubectl exec` into the admintools pod)
remains in `sandbox.py`. Temporal itself is infrastructure — the `michelangelo` chart has no opinion
on how or where Temporal is deployed.

### 3. MinIO persistence: `hostPath` or PVC

MinIO pod setup (including the hostPath vs PVC choice) stays in `sandbox.py` as infrastructure.
The `michelangelo` chart only consumes a `objectStorage.endpoint` value — it has no opinion on how MinIO
is provisioned or what backing storage it uses.

### 4. CI integration: `sandbox.py sync` stays as the entry point

CI continues calling `sandbox.py sync`. Phase 4 makes that method call
`helm upgrade michelangelo --reuse-values` internally for the control plane. No change to CI scripts
or pipelines — the Python interface is stable, the implementation switches to Helm underneath.

### 5. Port coordination: parse `values-k3d.yaml` in `sandbox.py _create()` (Phase 4)

`sandbox.py _create()` should read NodePort values from `values-k3d.yaml` to build the k3d
`--port` flags, rather than keeping a separate hardcoded list. Planned for Phase 4.
