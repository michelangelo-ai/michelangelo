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
| Workflow engine | Cadence or Temporal | Reachable at a host:port from inside the cluster. |
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

## Contributing

Issues and PRs welcome at https://github.com/michelangelo-ai/michelangelo. Run `helm lint ./helm/michelangelo` and `helm template ./helm/michelangelo -f helm/michelangelo/values-k3d.yaml` locally before submitting changes to chart files.
