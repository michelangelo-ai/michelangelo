---
sidebar_position: 2
sidebar_label: "Helm Chart"
---

# Install Michelangelo with Helm

The `michelangelo` Helm chart installs the Michelangelo control plane on any Kubernetes cluster — production, staging, or a local development cluster. After installation you can manage the platform with standard `helm install`, `helm upgrade`, and `helm uninstall` commands.

This guide walks you through prerequisites, a minimal install, verification, customization, and upgrade or removal. The final sections explain the chart's design so you know what you are getting.

## Who this is for

This guide is for **platform engineers and infrastructure operators** who want to run Michelangelo on a Kubernetes cluster they manage. By the end, you will have:

- A running Michelangelo control plane on your cluster
- A clear understanding of what the chart owns and what you must provide
- The commands to upgrade, customize, and remove the release

If you are looking for a fully scripted local sandbox instead, see [Dev Environment Setup](../contributing/dev-environment.md).

## Prerequisites

Before you install, make sure you have:

| Requirement | Version | Notes |
|-------------|---------|-------|
| Kubernetes cluster | 1.27 or newer | Chart's `kubeVersion` constraint is `>=1.27.0-0` |
| `kubectl` | Compatible with your cluster | Configured with cluster admin or equivalent permissions |
| `helm` | 3.8 or newer | Required for OCI dependencies and the `lookup` template function |

You also need the following infrastructure reachable from the cluster **before** you install the chart. The chart expects you to point it at running services — it does not create them (unless you opt into a bundled subchart).

| Component | What the chart needs | Examples |
|-----------|---------------------|----------|
| Metadata storage | A reachable MySQL or PostgreSQL endpoint, plus credentials (root password OR an existing Kubernetes Secret) | RDS, Cloud SQL, an in-cluster pod |
| Object storage | An S3-compatible endpoint and access keys (or an existing Secret) | S3, GCS, MinIO |
| Workflow engine | A Cadence or Temporal frontend address — or enable a bundled subchart (see [Self-contained install](#self-contained-install-with-cadence-or-temporal)) | Managed Cadence/Temporal, in-cluster install |

## Quick start

A minimal install pointing at existing infrastructure:

```bash
helm install michelangelo ./helm/michelangelo \
  --set metadataStorage.host=mysql.example.com \
  --set metadataStorage.rootPassword=changeme \
  --set objectStorage.endpoint=s3.amazonaws.com \
  --set objectStorage.accessKeyId=AKID \
  --set objectStorage.secretAccessKey=SECRET \
  --set workflow.endpoint=cadence-frontend.example.com:7833
```

If any required value is missing, `helm install` exits before creating any resources and tells you which value to set. See the [values reference](#values-reference) for the full list.

### Verify the install

```bash
kubectl get deployments -l app.kubernetes.io/instance=michelangelo
```

You should see five Deployments, each with `READY 1/1`:

```
NAME                          READY   UP-TO-DATE   AVAILABLE
michelangelo-apiserver        1/1     1            1
michelangelo-controllermgr    1/1     1            1
michelangelo-envoy            1/1     1            1
michelangelo-ui               1/1     1            1
michelangelo-worker           1/1     1            1
```

Run the chart's built-in connectivity test:

```bash
helm test michelangelo
```

### Reach the UI

For a default `ClusterIP` install, port-forward to your laptop:

```bash
kubectl port-forward svc/michelangelo-ui 8080:80
```

Then open http://localhost:8080 in your browser. For production, see [Expose the UI and API](#expose-the-ui-and-api) below.

## Customization

### Disable services you do not need

```bash
helm install michelangelo ./helm/michelangelo \
  --set ui.enabled=false \
  --set envoy.enabled=false \
  ...
```

### Restrict the controller to specific namespaces

By default `controllermgr` watches all namespaces (it gets a `ClusterRole`). Set `controllermgr.watchNamespace` to a list to switch to namespaced `Role` + `RoleBinding`:

```yaml
controllermgr:
  watchNamespace:
    - team-ml
    - team-ranking
```

### Use a values file

Copy `helm/michelangelo/values.yaml`, edit it, and pass with `-f`:

```bash
helm install michelangelo ./helm/michelangelo -f my-values.yaml
```

### Use existing Secrets (GitOps-friendly)

Instead of passing credentials through `--set`, point the chart at Secrets you manage out-of-band:

```yaml
metadataStorage:
  existingSecret: my-mysql-secret    # must contain key: rootPassword
objectStorage:
  existingSecret: my-s3-secret       # must contain keys: accessKeyId, secretAccessKey
```

### Expose the UI and API

The chart includes per-service Ingress templates (`apiserver.ingress`, `envoy.ingress`, `ui.ingress`). Enable and configure them to expose the UI and API outside the cluster:

```yaml
ui:
  ingress:
    enabled: true
    className: nginx
    hosts:
      - host: michelangelo.example.com
        paths: ["/"]
    tls:
      - secretName: michelangelo-ui-tls
        hosts: [michelangelo.example.com]

envoy:
  ingress:
    enabled: true
    # ... mirror UI config for the gRPC-Web endpoint

ui:
  apiBaseUrl: https://michelangelo.example.com/api    # match your Envoy/Ingress URL
```

### Enable TLS on the API server

The apiserver gRPC port supports TLS via `apiserver.tls.*`:

```yaml
apiserver:
  tls:
    enabled: true
    secretName: michelangelo-apiserver-tls
```

### Local development on k3d

A ready-made overrides file for [k3d](https://k3d.io) clusters switches services to `NodePort`, enables the bundled Cadence subchart, and points at in-cluster infrastructure:

```bash
helm install michelangelo ./helm/michelangelo -f helm/michelangelo/values-k3d.yaml
```

See [Dev Environment Setup](../contributing/dev-environment.md) for the full local workflow.

## Self-contained install with Cadence or Temporal

If you do not have a workflow engine available, the chart can install one for you. Both **Cadence** and **Temporal** are declared as optional subcharts and are disabled by default. Enable exactly one — `templates/validations.yaml` rejects installs that enable both.

### Cadence subchart

```bash
helm dependency update helm/michelangelo

helm install michelangelo ./helm/michelangelo \
  --set cadence.enabled=true \
  --set workflow.engine=cadence \
  --set workflow.endpoint=michelangelo-cadence-frontend:7833 \
  ...   # other required infrastructure values
```

Cadence subchart values are namespaced under `cadence:`. A common setup shares MySQL with the control plane but uses a separate `cadence` database:

```yaml
cadence:
  enabled: true
  persistence:
    defaultStore: mysql
    mysql:
      driver: mysql
      host: mysql.example.com
      port: 3306
      database: cadence
      user: root
      password: changeme
  web:
    enabled: true
```

### Temporal subchart

```bash
helm install michelangelo ./helm/michelangelo \
  --set temporal.enabled=true \
  --set workflow.engine=temporal \
  --set workflow.endpoint=michelangelo-temporal-frontend:7233 \
  ...
```

Pass Temporal subchart values under the `temporal:` key. See the [official Temporal Helm chart](https://github.com/temporalio/helm-charts) for the full surface.

## Upgrade

```bash
helm upgrade michelangelo ./helm/michelangelo --reuse-values
```

To change a single value:

```bash
helm upgrade michelangelo ./helm/michelangelo --reuse-values \
  --set ui.enabled=true
```

Review the chart's `CHANGELOG.md` before upgrading across a minor version.

## Uninstall

```bash
helm uninstall michelangelo
```

This removes all Deployments, Services, ConfigMaps, RBAC, and CRDs created by the chart. **Two Secrets are intentionally retained**: `metadata-storage-secret` and `object-storage-secret`. They are annotated `helm.sh/resource-policy: keep` so an `uninstall`/`install` cycle does not destroy externally-injected credentials.

Delete them manually for a fully clean slate:

```bash
kubectl delete secret metadata-storage-secret object-storage-secret
```

## Troubleshooting

| Symptom | Likely cause | What to try |
|---------|--------------|-------------|
| `helm install` fails with `<value> is required` | A required value is unset | Add it with `--set` or in your values file |
| `helm install` fails with a validation error | Both `cadence.enabled=true` and `temporal.enabled=true` set | Pick one workflow engine |
| `apiserver` Pod stuck in `Init:0/2` | `wait-for-metadata-storage` cannot reach DB | `kubectl run -it --rm mysql-test --image=mysql:8.0 -- mysqladmin ping -h <host>` |
| `apiserver` Pod stuck in `Init:1/2` | `schema-init` cannot apply SQL schema | `kubectl logs <pod> -c schema-init` — usually a credentials issue |
| `worker` Pod `CrashLoopBackOff` | Cannot reach workflow engine | Verify `workflow.endpoint` resolves and the port is open from the cluster |
| `helm test` fails | API server unreachable from inside cluster | `kubectl logs <test-pod>`; check apiserver Service exists |
| UI shows network errors in browser | `ui.apiBaseUrl` does not match how you exposed the API | Set `ui.apiBaseUrl` to the URL the browser uses to reach the API |

For deeper diagnostics see [Troubleshooting](operations/troubleshooting.md).

## Values reference

Most commonly set values. See `helm/michelangelo/values.yaml` for the complete list.

| Key | Required | Default | Description |
|-----|----------|---------|-------------|
| `metadataStorage.host` | yes | — | Hostname of MySQL or PostgreSQL |
| `metadataStorage.port` | no | `3306` | Database port |
| `metadataStorage.rootPassword` | yes¹ | — | Root password for schema initialization |
| `metadataStorage.existingSecret` | no | — | Name of a Secret containing `rootPassword` |
| `objectStorage.endpoint` | yes | — | S3-compatible endpoint |
| `objectStorage.accessKeyId` | yes¹ | — | S3 access key ID |
| `objectStorage.secretAccessKey` | yes¹ | — | S3 secret access key |
| `objectStorage.existingSecret` | no | — | Secret containing `accessKeyId`/`secretAccessKey` |
| `workflow.engine` | no | `cadence` | `cadence` or `temporal` |
| `workflow.endpoint` | yes | — | `host:port` of workflow frontend |
| `apiserver.tls.enabled` | no | `false` | Enable TLS on the apiserver gRPC port |
| `ui.apiBaseUrl` | no | `/api` | URL the UI uses to call the API |
| `ui.enabled` / `envoy.enabled` / etc. | no | `true` | Per-service install toggle |
| `<service>.ingress.enabled` | no | `false` | Per-service Ingress toggle |
| `controllermgr.watchNamespace` | no | `[]` (all) | Namespaces the controller manager watches |
| `cadence.enabled` | no | `false` | Install bundled Cadence subchart |
| `temporal.enabled` | no | `false` | Install bundled Temporal subchart |

¹ Required unless `existingSecret` is set.

## What the chart installs

The `michelangelo` chart owns the **control plane only**. Three tiers, with clear ownership boundaries:

- **Infrastructure tier** — stateful, long-lived. You provide it (or opt into a subchart): MySQL/PostgreSQL, S3-compatible storage, and a Cadence or Temporal service.
- **Control plane tier** — stateless, frequently redeployed. The chart installs all five services as Deployments:
  - `michelangelo-apiserver` — gRPC API server (port 15566)
  - `michelangelo-envoy` — gRPC-Web proxy (port 8081)
  - `michelangelo-ui` — React frontend (port 80)
  - `michelangelo-worker` — Cadence/Temporal workflow client
  - `michelangelo-controllermgr` — Kubernetes controller manager
- **Observability tier** — optional. Bring your own Prometheus and Grafana, or see [Monitoring & Observability](operations/monitoring.md).

### Chart layout

```
helm/michelangelo/
├── Chart.yaml              # includes optional cadence + temporal dependencies
├── README.md
├── values.yaml             # production defaults (ClusterIP, empty addresses, subcharts off)
├── values-k3d.yaml         # k3d overrides (NodePorts, in-cluster infra, cadence enabled)
├── files/schema/
│   └── mysql-init-schema.sql      # CRD schema applied at first install
├── crds/                          # placeholder — CRDs self-register at apiserver startup
└── templates/
    ├── _helpers.tpl
    ├── NOTES.txt                  # post-install instructions
    ├── validations.yaml           # chart-level guardrails (e.g. cadence/temporal exclusivity)
    ├── rbac/                      # ServiceAccount, ClusterRole, ClusterRoleBinding
    ├── tests/
    │   └── test-connection.yaml   # helm test hook
    └── core/                      # 20 templates for the 5 services
        ├── apiserver-{deployment,service,configmap,ingress,schema-init-configmap}.yaml
        ├── envoy-{deployment,service,configmap,ingress}.yaml
        ├── ui-{deployment,service,configmap,ingress}.yaml
        ├── worker-{deployment,configmap}.yaml
        ├── controllermgr-{deployment,service,configmap}.yaml
        ├── metadata-storage-secret.yaml    # resource-policy: keep
        └── object-storage-secret.yaml      # resource-policy: keep
```

## Design notes

### All control plane workloads are Deployments

Every service runs as a Deployment, not a bare Pod, so you get self-healing and rolling updates.

### Schema initialization runs as init containers

Two init containers on the `apiserver` Pod: `wait-for-metadata-storage` polls the DB until reachable, then `schema-init` applies the CRD schema idempotently. This removes the ordering race that exists when the schema is applied by a separate Job.

### Credential Secrets are retained on uninstall

`metadata-storage-secret` and `object-storage-secret` carry `helm.sh/resource-policy: keep`, so externally-injected credentials survive an uninstall/reinstall.

### Required values fail fast

Required values use Helm's `required` template function — `helm install` fails before any resource is created.

### Chart-level validations

`templates/validations.yaml` enforces chart-wide invariants (notably, that `cadence.enabled` and `temporal.enabled` are not both true). Validation errors surface during `helm install`/`upgrade`.

### Least-privilege RBAC

The chart installs a scoped `ClusterRole` covering only what `controllermgr` and `apiserver` need: CRD lifecycle, Michelangelo CRs, KubeRay/Spark CRs, namespaces (create/update/patch/delete), pods/services, configmaps/secrets, and leader-election leases. There is no `cluster-admin` grant.

### Pod security defaults

All control plane Pods run with `runAsNonRoot: true`, `runAsUser: 65534`, and `drop: [ALL]` capabilities by default. Override per-service if a custom image needs different settings.

### Per-service `enabled` toggle

Each service has an `enabled` flag, and templates wrap the Deployment, Service, ConfigMap, and Ingress accordingly.

### Envoy backend is release-scoped

The Envoy ConfigMap references `{{ include "michelangelo.fullname" . }}-apiserver`, so multiple releases in different namespaces do not collide.

### KubeRay log-collector sidecar

When enabled via `controllermgr.jobs.k8sengine.mapper.logPersistence`, controllermgr injects a `kuberayCollector` sidecar into Ray jobs to persist logs to object storage. See `values.yaml` comments for the available knobs.

## Next steps

- [Platform Setup](setup/platform-setup.md) — configure the components installed by this chart through their ConfigMaps
- [Network & Ingress](setup/network.md) — TLS and multi-cluster connectivity
- [Authentication](setup/authentication.md) — connect an identity provider and configure RBAC
- [Register a Compute Cluster](setup/register-a-compute-cluster-to-michelangelo-control-plane.md) — connect a cluster for Ray and Spark job dispatch
- [Monitoring & Observability](operations/monitoring.md) — scrape metrics and configure dashboards
