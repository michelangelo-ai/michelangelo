# Michelangelo Helm Chart

[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/michelangelo)](https://artifacthub.io/packages/helm/michelangelo/michelangelo)

The `michelangelo` Helm chart installs the Michelangelo control plane — apiserver, gRPC-Web proxy (envoy), UI, workflow worker, controller manager, CRDs, and RBAC — into any Kubernetes cluster.

The chart owns only the **control plane**. Infrastructure (metadata storage, object storage, workflow engine) is your responsibility — bring your own RDS / Cloud SQL, S3 / GCS, and Cadence / Temporal, or use the local development setup below.

## Prerequisites

| Requirement | Version | Notes |
|---|---|---|
| Kubernetes | 1.27+ | Tested on 1.27 – 1.30. CRDs use `apiextensions.k8s.io/v1`. |
| Helm | 3.12+ | Required for the `lookup` and `required` template functions used by the chart. |
| Metadata storage | MySQL 8.0 or PostgreSQL 14+ | The chart provisions schema via an init container; you provide a reachable host and root credentials. |
| Object storage | S3-compatible | S3, GCS (HMAC), MinIO, or any S3-API endpoint. The chart consumes `endpoint`, access key, and secret key. |
| Workflow engine | Cadence or Temporal | Bring your own (any reachable host:port), or set `cadence.enabled=true` to install the official Cadence chart as a subchart. See [Bundled Cadence](#bundled-cadence-optional-subchart). |
| Helm dependencies | – | Run `helm dependency build ./helm/michelangelo` once before the first install if you enable any subchart (`cadence.enabled=true`). |
| Optional: cluster operators | KubeRay, Spark Operator | Required only if your pipelines use Ray or Spark tasks. Install separately from their upstream charts. |

## Quick Install

### Local development (k3d)

The Michelangelo CLI provisions a local k3d cluster, MySQL, MinIO, and Cadence, then installs this chart with `values-k3d.yaml`:

```bash
pip install michelangelo
michelangelo sandbox up
```

To run `helm install` directly against an existing k3d cluster with infrastructure already up:

```bash
helm install michelangelo ./helm/michelangelo -f ./helm/michelangelo/values-k3d.yaml
```

### Production

```bash
helm install michelangelo ./helm/michelangelo \
  --namespace michelangelo --create-namespace \
  --set metadataStorage.host=my-rds.example.com \
  --set metadataStorage.rootPassword=$METADATA_ROOT_PASSWORD \
  --set objectStorage.endpoint=s3.amazonaws.com \
  --set objectStorage.accessKeyId=$AWS_ACCESS_KEY_ID \
  --set objectStorage.secretAccessKey=$AWS_SECRET_ACCESS_KEY \
  --set workflow.endpoint=temporal-frontend.temporal:7233 \
  --set workflow.engine=temporal
```

For repeatable installs, write a `values-prod.yaml` and pass it with `-f` instead of long `--set` chains. Never put credentials in a file you commit to git — use `--set` from a secrets manager, or pre-create the `object-storage-credentials` Secret in the release namespace and let Helm leave it alone (the chart marks it `helm.sh/resource-policy: keep`).

## Bundled Cadence (optional subchart)

If you do not have a Cadence or Temporal service, you can install the official [Cadence Helm chart](https://github.com/cadence-workflow/cadence-charts) (`cadence-workflow/cadence` v1.1.0) as part of this release by setting `cadence.enabled=true`. Users with an existing managed Cadence or Temporal service should leave it disabled (default) and point `workflow.endpoint` at their own service.

The bundled subchart is **not** used by `michelangelo sandbox up`. The local sandbox provisions its own Cadence outside the chart.

### When to use it

| You have… | Setting |
|---|---|
| A managed Cadence cluster | `cadence.enabled=false` (default) |
| A managed Temporal cluster | `cadence.enabled=false` + `workflow.engine=temporal` |
| Nothing — fully self-contained install | `cadence.enabled=true` |

### Prerequisite: download the subchart

```bash
helm dependency build ./helm/michelangelo
```

`Chart.lock` is committed, so `build` fetches the locked Cadence version exactly. Use `helm dependency update` only when you intentionally want to re-resolve and rewrite the lock (e.g., bumping the subchart version in `Chart.yaml`).

Skipping this step produces: `Error: found in Chart.yaml, but missing in charts/ directory: cadence`

### Install command

Step 1 — fetch the subchart:

```bash
helm dependency build ./helm/michelangelo
```

Step 2 — install:

```bash
helm install michelangelo ./helm/michelangelo \
  --namespace michelangelo --create-namespace \
  --set cadence.enabled=true \
  --set workflow.engine=cadence \
  --set workflow.endpoint=michelangelo-cadence-frontend:7833 \
  --set metadataStorage.host=my-mysql.example.com \
  --set metadataStorage.rootPassword=$METADATA_ROOT_PASSWORD \
  --set cadence.config.persistence.database.sql.hosts=my-mysql.example.com \
  --set cadence.config.persistence.database.sql.password=$METADATA_ROOT_PASSWORD \
  --set objectStorage.endpoint=s3.amazonaws.com \
  --set objectStorage.accessKeyId=$AWS_ACCESS_KEY_ID \
  --set objectStorage.secretAccessKey=$AWS_SECRET_ACCESS_KEY \
  --set ui.apiBaseUrl=https://michelangelo.example.com/api
```

`workflow.endpoint` uses the form `<release>-cadence-frontend:7833`. Verify the actual Service name with:

```bash
kubectl --namespace michelangelo get svc -l app.kubernetes.io/name=cadence,app.kubernetes.io/component=frontend
```

### MySQL setup

Cadence creates and writes to **two** databases:

| Database | Purpose |
|---|---|
| `cadence` | Workflow history, task lists, domains |
| `cadence_visibility` | Workflow list/search queries |

The MySQL user needs `CREATE DATABASE` privilege for both. To pre-create them manually:

```sql
CREATE DATABASE cadence;
CREATE DATABASE cadence_visibility;
GRANT ALL PRIVILEGES ON cadence.* TO 'your_user'@'%';
GRANT ALL PRIVILEGES ON cadence_visibility.* TO 'your_user'@'%';
```

The Michelangelo control plane uses the `michelangelo` database — there is no conflict. You can share a single MySQL instance by setting both `metadataStorage.host` and `cadence.config.persistence.database.sql.hosts` to the same hostname.

### Mutual exclusivity with Temporal

`cadence.enabled=true` with `workflow.engine=temporal` is a misconfiguration — the Cadence pods install but the worker ignores them. The chart prints a warning in `NOTES.txt` if both are set.

## Bundled Temporal (optional subchart)

If you prefer Temporal over Cadence, you can install the official [Temporal Helm chart](https://github.com/temporalio/helm-charts) (`temporalio/temporal` v0.44.0) as part of this release by setting `temporal.enabled=true`. Users with an existing managed Temporal service should leave it disabled (default) and point `workflow.endpoint` at their own service.

The bundled subchart is **not** used by `michelangelo sandbox up`. The local sandbox provisions its own Temporal outside the chart.

**Do not enable both `cadence.enabled=true` and `temporal.enabled=true`** — pick one workflow engine per release.

### When to use it

| You have… | Setting |
|---|---|
| A managed Temporal cluster | `temporal.enabled=false` (default) |
| A managed Cadence cluster | `temporal.enabled=false`, set `workflow.engine=cadence` |
| Nothing — fully self-contained install | `temporal.enabled=true` |

### Prerequisite: download the subchart

```bash
helm dependency build ./helm/michelangelo
```

Skipping produces: `Error: found in Chart.yaml, but missing in charts/ directory: temporal`

### Install command

Step 1 — fetch the subchart:

```bash
helm dependency build ./helm/michelangelo
```

Step 2 — install:

```bash
helm install michelangelo ./helm/michelangelo \
  --namespace michelangelo --create-namespace \
  --set temporal.enabled=true \
  --set workflow.engine=temporal \
  --set workflow.endpoint=michelangelo-temporal-frontend:7233 \
  --set metadataStorage.host=my-mysql.example.com \
  --set metadataStorage.rootPassword=$METADATA_ROOT_PASSWORD \
  --set temporal.server.config.persistence.default.sql.host=my-mysql.example.com \
  --set temporal.server.config.persistence.default.sql.password=$METADATA_ROOT_PASSWORD \
  --set temporal.server.config.persistence.visibility.sql.host=my-mysql.example.com \
  --set temporal.server.config.persistence.visibility.sql.password=$METADATA_ROOT_PASSWORD \
  --set objectStorage.endpoint=s3.amazonaws.com \
  --set objectStorage.accessKeyId=$AWS_ACCESS_KEY_ID \
  --set objectStorage.secretAccessKey=$AWS_SECRET_ACCESS_KEY \
  --set ui.apiBaseUrl=https://michelangelo.example.com/api
```

### Key differences from the Cadence subchart

| | Cadence | Temporal |
|---|---|---|
| MySQL driver | `"mysql"` | **`"mysql8"`** (different string — do not use `"mysql"`) |
| Frontend port | `7833` | `7233` |
| Frontend Service | `<release>-cadence-frontend` | `<release>-temporal-frontend` |
| Main database | `cadence` | `temporal` |
| Visibility database | `cadence_visibility` | `temporal_visibility` |
| Persistence key path | `config.persistence.database.sql.*` | `server.config.persistence.default.sql.*` |

### MySQL setup

Temporal creates two databases — same privilege requirements as Cadence:

| Database | Purpose |
|---|---|
| `temporal` | Workflow history, task queues, namespaces |
| `temporal_visibility` | Workflow list/search queries |

Pre-create if your MySQL user lacks `CREATE DATABASE`:

```sql
CREATE DATABASE temporal;
CREATE DATABASE temporal_visibility;
GRANT ALL PRIVILEGES ON temporal.* TO 'your_user'@'%';
GRANT ALL PRIVILEGES ON temporal_visibility.* TO 'your_user'@'%';
```

### Troubleshooting

**Schema job fails with `Access denied ... CREATE DATABASE`.**

Grant `CREATE DATABASE` or pre-create both databases (see above). Then delete the failed job and re-upgrade:

```bash
kubectl delete job -l app.kubernetes.io/name=temporal -n michelangelo
helm upgrade michelangelo ./helm/michelangelo --reuse-values
```

**Worker logs `connection refused` to `<release>-temporal-frontend:7233`.**

Wait for Temporal to finish initializing (~60s):

```bash
kubectl --namespace michelangelo wait --for=condition=ready pod \
  -l app.kubernetes.io/name=temporal --timeout=5m
```

## Values Reference

Top-level keys. See [`values.yaml`](./values.yaml) for the full annotated schema.

| Key | Type | Default | Required | Description |
|---|---|---|---|---|
| `metadataStorage.driver` | string | `mysql` | yes | `mysql` or `postgres`. Selects the schema-init image and JDBC dialect. |
| `metadataStorage.host` | string | `""` | **yes** | Hostname of metadata storage (e.g. `my-rds.example.com`). Install fails if empty. |
| `metadataStorage.port` | int | `3306` | no | Defaults to 3306 for MySQL, 5432 for Postgres. |
| `metadataStorage.database` | string | `michelangelo` | no | Database name; created by the schema-init container if it does not exist. |
| `metadataStorage.rootPassword` | string | `""` | **yes** | Root password used by schema-init and runtime services. |
| `objectStorage.endpoint` | string | `""` | **yes** | S3-compatible endpoint (e.g. `s3.amazonaws.com`, `minio:9000`). |
| `objectStorage.secure` | bool | `true` | no | TLS for object storage. Set `false` for in-cluster MinIO. |
| `objectStorage.region` | string | `us-east-1` | no | AWS region; used by S3, GCS HMAC, and MinIO. |
| `objectStorage.bucket` | string | `michelangelo` | no | Bucket name for artifacts, models, and logs. |
| `objectStorage.accessKeyId` | string | `""` | conditional | Required unless `objectStorage.existingSecret` is set. |
| `objectStorage.secretAccessKey` | string | `""` | conditional | Required unless `objectStorage.existingSecret` is set. |
| `objectStorage.existingSecret` | string | `""` | no | Name of a pre-existing Secret with `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` keys. Takes precedence over the inline keys. |
| `workflow.engine` | string | `cadence` | yes | `cadence` or `temporal`. Mutually exclusive — selects which worker config block renders. |
| `workflow.endpoint` | string | `""` | **yes** | Workflow engine address (e.g. `cadence-frontend:7833`, `temporal-frontend:7233`). |
| `workflow.domain` | string | `default` | no | Cadence domain or Temporal namespace. |
| `images.apiserver` | string | `ghcr.io/michelangelo-ai/apiserver:main` | no | Override to pin a version or use a private registry. |
| `images.worker` | string | `ghcr.io/michelangelo-ai/worker:main` | no | |
| `images.ui` | string | `ghcr.io/michelangelo-ai/ui:main` | no | |
| `images.controllermgr` | string | `ghcr.io/michelangelo-ai/controllermgr:main` | no | |
| `images.envoy` | string | `envoyproxy/envoy:v1.29-latest` | no | |
| `images.pullPolicy` | string | `IfNotPresent` | no | |
| `imagePullSecrets` | list | `[]` | no | List of Secret names for private registries. |
| `apiserver.enabled` | bool | `true` | no | Toggle the apiserver Deployment. |
| `apiserver.port` | int | `15566` | no | gRPC port. |
| `apiserver.service.type` | string | `ClusterIP` | no | `ClusterIP`, `NodePort`, or `LoadBalancer`. |
| `apiserver.service.nodePort` | int | `null` | no | Required when `service.type=NodePort`. |
| `envoy.enabled` | bool | `true` | no | Toggle the gRPC-Web proxy. |
| `envoy.port` | int | `8081` | no | |
| `envoy.corsOrigins` | string | `""` | no | Regex for `Access-Control-Allow-Origin`. Required if the UI runs on a different host. |
| `envoy.service.type` | string | `ClusterIP` | no | |
| `envoy.service.nodePort` | int | `null` | no | |
| `ui.enabled` | bool | `true` | no | Toggle the UI Deployment. |
| `ui.apiBaseUrl` | string | `""` | conditional | Browser-reachable URL of the envoy proxy. Required when `ui.enabled=true`. |
| `ui.service.type` | string | `ClusterIP` | no | |
| `ui.service.port` | int | `80` | no | |
| `ui.service.nodePort` | int | `null` | no | |
| `worker.enabled` | bool | `true` | no | |
| `worker.replicas` | int | `1` | no | Scale horizontally for higher pipeline-run throughput. |
| `controllermgr.enabled` | bool | `true` | no | |
| `controllermgr.watchNamespace` | list | `[]` | no | Namespaces the controller watches. Empty = all namespaces (ClusterRole). Set to a list to scope down to namespaced Roles. |
| `serviceAccount.create` | bool | `true` | no | Create a ServiceAccount for the chart. |
| `serviceAccount.name` | string | `""` | no | Override the generated ServiceAccount name. |
| `podSecurityContext` | object | see values.yaml | no | Applied to every Pod. |
| `securityContext` | object | see values.yaml | no | Applied to every container. |
| `resources` | object | see values.yaml | no | Default resource requests/limits applied per service. |
| `nodeSelector` | object | `{}` | no | Applied to every Pod. |
| `tolerations` | list | `[]` | no | |
| `affinity` | object | `{}` | no | |

## Ingress

Use Ingress when you want the UI and API reachable on a real hostname. For one-off access to a ClusterIP install, prefer `kubectl port-forward`. NodePort and LoadBalancer Services still work, but Ingress gives you TLS termination, hostname-based routing, and a single load balancer across many releases.

The chart ships three independent Ingress objects — all disabled by default: `ui.ingress` (HTTP/1.1, the React app), `envoy.ingress` (gRPC-Web, the browser API path), and `apiserver.ingress` (gRPC, for external CLI/SDK clients). Most users need only the first two.

> **Heads up: set `envoy.corsOrigins` whenever you enable Ingress.**
> Without it, the UI loads but every API call fails with a CORS error.
> The value is a regex — escape literal dots:
>   --set 'envoy.corsOrigins=https://michelangelo\.example\.com'

### Phase A — UI + Envoy on one hostname (nginx, path-split)

```bash
helm upgrade --install michelangelo ./helm/michelangelo \
  --namespace michelangelo --create-namespace \
  -f values-prod.yaml \
  --set ui.ingress.enabled=true \
  --set ui.ingress.hostname=michelangelo.example.com \
  --set ui.ingress.ingressClassName=nginx \
  --set 'ui.ingress.tls[0].hosts[0]=michelangelo.example.com' \
  --set 'ui.ingress.tls[0].secretName=michelangelo-tls' \
  --set envoy.ingress.enabled=true \
  --set envoy.ingress.hostname=michelangelo.example.com \
  --set 'envoy.ingress.path=/api(/|$)(.*)' \
  --set envoy.ingress.ingressClassName=nginx \
  --set 'envoy.ingress.annotations.nginx\.ingress\.kubernetes\.io/rewrite-target=/$2' \
  --set 'envoy.ingress.annotations.nginx\.ingress\.kubernetes\.io/use-regex=true' \
  --set 'envoy.ingress.annotations.nginx\.ingress\.kubernetes\.io/proxy-read-timeout=600' \
  --set 'envoy.ingress.tls[0].hosts[0]=michelangelo.example.com' \
  --set 'envoy.ingress.tls[0].secretName=michelangelo-tls' \
  --set 'envoy.corsOrigins=https://michelangelo\.example\.com'
```

`ui.apiBaseUrl` is auto-derived from `envoy.ingress` — leave it empty.

### Phase A — Envoy on a dedicated subdomain

```bash
helm upgrade --install michelangelo ./helm/michelangelo \
  --namespace michelangelo --create-namespace \
  -f values-prod.yaml \
  --set ui.ingress.enabled=true \
  --set ui.ingress.hostname=michelangelo.example.com \
  --set envoy.ingress.enabled=true \
  --set envoy.ingress.hostname=api.michelangelo.example.com \
  --set 'envoy.corsOrigins=https://michelangelo\.example\.com'
```

### Phase A on other controllers

| Controller | Path strip pattern |
|---|---|
| nginx | `path: /api(/|$)(.*)` + `rewrite-target: /$2` + `use-regex: "true"` |
| Traefik | Requires a `Middleware` CRD with `stripPrefix` — define out-of-band and reference via `traefik.ingress.kubernetes.io/router.middlewares`. See Traefik StripPrefix docs. |
| Contour | `HTTPProxy` resource with `pathRewritePolicy.replacePrefix` instead of Ingress. |
| Emissary | `Mapping` resource with `prefix` and `rewrite` instead of Ingress. |

The dedicated-subdomain pattern (envoy on its own hostname) avoids all path stripping — use it for controller-agnostic installs.

### Phase B — apiserver gRPC (recommended: mode: grpc)

Expose the raw gRPC apiserver to external CLI/SDK clients. `mode: grpc` lets the controller terminate TLS — the Pod stays plaintext, no pod TLS Secret needed:

```bash
helm upgrade michelangelo ./helm/michelangelo --reuse-values \
  --set apiserver.ingress.enabled=true \
  --set apiserver.ingress.mode=grpc \
  --set apiserver.ingress.hostname=grpc.michelangelo.example.com \
  --set apiserver.ingress.ingressClassName=nginx \
  --set 'apiserver.ingress.tls[0].hosts[0]=grpc.michelangelo.example.com' \
  --set 'apiserver.ingress.tls[0].secretName=apiserver-grpc-tls'
```

The chart auto-injects `nginx.ingress.kubernetes.io/backend-protocol: GRPC`.

### Phase B — apiserver gRPC (end-to-end TLS: mode: passthrough)

Use when you need TLS to terminate inside the Pod (mTLS, compliance).

Two preconditions:
1. `apiserver.tls.enabled=true` (pod must terminate TLS)
2. The nginx-ingress controller must be started with `--enable-ssl-passthrough` at the cluster level (NOT a chart annotation — a controller startup flag). Without it, the annotation is silently ignored.

Verify:

```bash
kubectl -n ingress-nginx get deploy ingress-nginx-controller \
  -o jsonpath='{.spec.template.spec.containers[0].args}' | tr ',' '\n' | grep ssl-passthrough
```

```bash
kubectl create secret tls apiserver-tls --cert=server.crt --key=server.key -n michelangelo
helm upgrade michelangelo ./helm/michelangelo --reuse-values \
  --set apiserver.tls.enabled=true \
  --set apiserver.tls.secretName=apiserver-tls \
  --set apiserver.ingress.enabled=true \
  --set apiserver.ingress.mode=passthrough \
  --set apiserver.ingress.hostname=grpc.michelangelo.example.com
```

The apiserver hostname must be **different** from the UI/envoy hostname.

### apiBaseUrl auto-derive

Leave `ui.apiBaseUrl` empty when `envoy.ingress.enabled=true` and `envoy.ingress.hostname` is set — the chart derives it as `<scheme>://<hostname><path>` (https when `envoy.ingress.tls` is non-empty).

| hostname | path | tls | derived apiBaseUrl |
|---|---|---|---|
| `michelangelo.example.com` | `/` | `[]` | `http://michelangelo.example.com` |
| `michelangelo.example.com` | `/api` | non-empty | `https://michelangelo.example.com/api` |
| `api.michelangelo.example.com` | `/` | non-empty | `https://api.michelangelo.example.com` |

Set `ui.apiBaseUrl` explicitly when envoy is on NodePort/LoadBalancer, or when scheme/path differs.

Auto-derive does NOT activate when `ui.ingress` is enabled but `envoy.ingress` is disabled — a UI Ingress alone gives the browser nowhere to send gRPC-Web traffic. Set `ui.apiBaseUrl` explicitly to point at wherever envoy is exposed.

See [`values.yaml`](./values.yaml) for the full annotated schema.

## Upgrade

```bash
helm upgrade michelangelo ./helm/michelangelo --reuse-values
```

`--reuse-values` keeps your previous `--set` and `-f` overrides. Pass new flags only for the values you want to change. To pin to a specific image tag during upgrade:

```bash
helm upgrade michelangelo ./helm/michelangelo --reuse-values \
  --set images.apiserver=ghcr.io/michelangelo-ai/apiserver:v0.3.1 \
  --set images.worker=ghcr.io/michelangelo-ai/worker:v0.3.1 \
  --set images.ui=ghcr.io/michelangelo-ai/ui:v0.3.1 \
  --set images.controllermgr=ghcr.io/michelangelo-ai/controllermgr:v0.3.1
```

The `object-storage-credentials` Secret is annotated `helm.sh/resource-policy: keep` and will not be overwritten on upgrade. To rotate credentials, update the Secret in place with `kubectl edit secret object-storage-credentials` or delete and re-apply it.

## Uninstall

```bash
helm uninstall michelangelo --namespace michelangelo
```

This removes all chart-managed resources. **It does not remove**:

- The `object-storage-credentials` Secret (intentional — protects against accidental credential loss). Delete manually with `kubectl delete secret object-storage-credentials -n michelangelo` if you want it gone.
- CRDs created by the chart (Helm does not delete CRDs by default to prevent data loss). To purge, run `kubectl delete crd -l app.kubernetes.io/managed-by=Helm,app.kubernetes.io/part-of=michelangelo`.
- Any data in your metadata or object storage — those live outside the chart.

## Troubleshooting

**The apiserver Pod is stuck in `Init:0/1`.**

The `schema-init` init container is waiting for metadata storage. Check:

```bash
kubectl logs <apiserver-pod> -c schema-init
```

Common causes: wrong `metadataStorage.host`, wrong `rootPassword`, network policy blocking the connection, RDS security group not allowing the cluster's egress CIDR.

**The UI loads but shows "Failed to fetch" in the browser console.**

The browser is hitting `ui.apiBaseUrl` directly — if envoy is on a ClusterIP Service the browser cannot reach it. Either expose envoy through a NodePort/LoadBalancer/Ingress and update `ui.apiBaseUrl`, or port-forward `svc/<release>-envoy` and set `ui.apiBaseUrl=http://localhost:8081`.

**`helm install` fails with `metadataStorage.host is required`.**

You did not provide `--set metadataStorage.host=...` or `-f values-k3d.yaml`. The chart fails fast on missing required values — see the full list in the values reference table above.

**`helm install` fails with `workflow.endpoint is required`.**

Same as above for the workflow engine. If installing against k3d, pass `-f helm/michelangelo/values-k3d.yaml` which sets `workflow.endpoint=cadence:7933`.

**Worker logs `connection refused` to the workflow engine.**

The Cadence/Temporal service is not reachable at `workflow.endpoint`. Verify with:

```bash
kubectl run -it --rm debug --image=busybox --restart=Never -- nc -zv <host> <port>
```

If using Temporal, confirm `workflow.engine=temporal` (the worker renders different config blocks for each engine — wrong engine value produces silent connection failures).

**Multiple installs collide on resource names.**

Set distinct release names. All resources are prefixed with `{{ include "michelangelo.fullname" . }}` which incorporates `.Release.Name` — installs in different namespaces with the same release name will not collide on namespaced resources, but will collide on cluster-scoped ones (CRDs, ClusterRoles).

**`helm install` fails with `found in Chart.yaml, but missing in charts/ directory: cadence`.**

Run `helm dependency build ./helm/michelangelo` first. `Chart.lock` is committed, so `build` fetches the locked Cadence version. Use `helm dependency update` only when you intentionally want to re-resolve and rewrite the lock (e.g., bumping the subchart version in `Chart.yaml`).

**`<release>-cadence-schema` Job fails with `Access denied ... CREATE DATABASE`.**

The MySQL user lacks `CREATE DATABASE` privilege. Either grant it or pre-create both `cadence` and `cadence_visibility` databases manually (see [MySQL setup](#mysql-setup)). Then delete the failed job and re-upgrade:

```bash
kubectl delete job -l app.kubernetes.io/name=cadence,app.kubernetes.io/component=schema -n michelangelo
helm upgrade michelangelo ./helm/michelangelo --reuse-values
```

**`helm upgrade` fails with `Job already exists` on the cadence-schema job.**

Helm cannot mutate Jobs. Delete the old job before upgrading:

```bash
kubectl delete job -l app.kubernetes.io/name=cadence -n michelangelo
helm upgrade michelangelo ./helm/michelangelo --reuse-values
```

The schema job is idempotent — it skips versions already applied.

**Cadence frontend pod crashes with `database "cadence_visibility" does not exist`.**

Create the missing database and re-run the schema job (delete + upgrade, see above):

```sql
CREATE DATABASE cadence_visibility;
GRANT ALL PRIVILEGES ON cadence_visibility.* TO 'root'@'%';
```

**Worker logs `connection refused` to `<release>-cadence-frontend:7833` after install.**

Cadence needs ~30-60s to initialize. Wait for readiness:

```bash
kubectl --namespace michelangelo wait --for=condition=ready pod \
  -l app.kubernetes.io/name=cadence --timeout=5m
```

If pods never become ready, check the schema job logs first.

**UI loads through Ingress but the browser console shows "Failed to fetch" or CORS errors.**

`envoy.corsOrigins` does not match the browser's `Origin` header. The value is a regex — escape dots and include the full scheme. For `https://michelangelo.example.com`:

```bash
--set 'envoy.corsOrigins=https://michelangelo\.example\.com'
```

Confirm by checking the browser's network tab for a 403 from envoy with `Access-Control-Allow-Origin: null`.

**`helm install` fails with `ui.ingress.hostname is required when ui.ingress.enabled=true`.**

Set `--set ui.ingress.hostname=<your-hostname>`. Same applies to `envoy.ingress.hostname` and `apiserver.ingress.hostname`.

**`helm install` fails with `apiserver.tls.enabled must be true when apiserver.ingress.mode=passthrough`.**

Switch to `mode: grpc` (simpler, no pod TLS needed) or enable pod TLS:

```bash
kubectl create secret tls apiserver-tls --cert=server.crt --key=server.key -n michelangelo
helm upgrade ... --set apiserver.tls.enabled=true --set apiserver.tls.secretName=apiserver-tls
```

**`apiserver.ingress.mode=grpc` returns 502 or the controller never speaks HTTP/2 to the Pod.**

Your controller does not support `nginx.ingress.kubernetes.io/backend-protocol: GRPC`. For non-nginx controllers, add the equivalent in `apiserver.ingress.annotations` (Contour: `projectcontour.io/upstream-protocol.h2c`; Emissary: `getambassador.io/config` with `grpc: true`). As a last resort, use `mode: passthrough`.

**`ui.apiBaseUrl` is empty and install fails with `ui.apiBaseUrl is required`.**

Auto-derive activates only when `envoy.ingress.enabled=true` AND `envoy.ingress.hostname` is set. If envoy is on NodePort/LoadBalancer (no Ingress), or if `ui.ingress` is enabled but `envoy.ingress` is disabled, set `ui.apiBaseUrl` explicitly to point at wherever envoy is exposed.

**Ingress created but every request returns 404.**

No controller is reconciling the Ingress. Verify:

```bash
kubectl get ingressclass
kubectl describe ingress <release>-ui -n <namespace>
```

If `ADDRESS` is empty, set `ingressClassName` to a class that exists or install an Ingress controller (`helm install ingress-nginx ingress-nginx/ingress-nginx`).

## Contributing

Issues and PRs welcome at https://github.com/michelangelo-ai/michelangelo. Run `helm lint ./helm/michelangelo` and `helm template ./helm/michelangelo -f helm/michelangelo/values-k3d.yaml` locally before submitting changes to chart files.
