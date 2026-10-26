# Install the Compute Cluster Chart

The `michelangelo-compute` Helm chart prepares a Kubernetes cluster to run the jobs that the Michelangelo AI control plane dispatches to it. It installs the KubeRay operator and the Kubeflow Spark Operator at pinned versions, the `ray-manager` identity the control plane authenticates as, and the runtime objects every job pod reads. Rendering fails when a setting the control plane relies on changes.

Install one release per compute cluster, named `michelangelo-compute`, in namespace `michelangelo-compute`. Then [register the cluster](register-a-compute-cluster-to-michelangelo-control-plane.md) with the control plane. The control plane has its own chart, covered in [Install Michelangelo AI with Helm](../helm-chart.md). The local sandbox installs this chart for you; see [Sandbox Setup](../../getting-started/sandbox-setup.md).

## Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| Kubernetes cluster | 1.29 or newer | The chart's `kubeVersion` constraint is `>=1.29.0-0` |
| `helm` | 3.13 or newer | CI tests the chart on Helm 3.13, 3.15 and 4.1 |
| `kubectl` | Compatible with your cluster | Cluster admin or equivalent: the chart creates CRDs, ClusterRoles and objects in `default` |

You also need:

- **A control plane on the release that ships this chart, or a later one.** Its controllermgr attaches the object-storage credentials Secret to task pods, and the chart's ConfigMap carries no credentials. See [Upgrade order](#upgrade-order).
- **Three endpoints reachable from this cluster:** the storage root, the object-storage endpoint and the model-registry endpoint (the control plane's API server).
- **The object-storage credentials Secret** in namespace `default`, unless the chart creates it. See [Object-storage credentials](#object-storage-credentials).

The commands on this page use these variables:

```bash
COMPUTE_CLUSTER=michelangelo-compute-0          # The name the cluster is registered under
COMPUTE_CONTEXT=my-compute-cluster-context      # Your kubectl context for the compute cluster
CONTROL_PLANE_CONTEXT=my-control-plane-context  # Your kubectl context for the control plane
CHART=oci://ghcr.io/michelangelo-ai/michelangelo/charts/michelangelo-compute
VERSION=<chart-version>                         # The Michelangelo release to install
```

## What the chart installs

| Component | What you get |
|-----------|--------------|
| KubeRay operator | Upstream chart `kuberay-operator` 1.7.1, in the release namespace |
| Spark Operator | Upstream Kubeflow chart `spark-operator` 2.5.2, in the release namespace, watching SparkApplications in `default` |
| Control-plane identity | ServiceAccount `ray-manager` and its token Secret `ray-manager-token`, Role and RoleBinding `ray-manager` in `default`, and ClusterRole and ClusterRoleBinding `ray-manager-clusterqueues` |
| Runtime contract | ConfigMap `michelangelo-config` in `default`, and the credentials Secret when the chart creates it |
| `helm test` | Pod `michelangelo-compute-test-runtime-contract` in `default`, which checks the runtime contract |

The control plane creates every RayCluster and SparkApplication in namespace `default`. Until federated Spark dispatch ships, Spark jobs run on the control plane's own cluster, so the Spark Operator on a dedicated compute cluster stays idle.

Helm creates the operators' CRDs, which the subcharts ship in `crds/`, at the first install only, and never upgrades or deletes them. See [Apply the operator CRDs first](#apply-the-operator-crds-first).

### Control-plane identity

| Object | API group | Resources | Verbs | Enabled by |
|---|---|---|---|---|
| Role `ray-manager` in `default` | `ray.io` | `rayclusters`, `rayjobs` | create, get, list, watch, update, patch, delete | Always |
| Same Role | core (`""`) | `pods` | get, list, watch | Always (the control plane's pod watch) |
| Same Role | `sparkoperator.k8s.io` | `sparkapplications` | Same as `ray.io` | `rayManager.grants.spark` (federated Spark dispatch) |
| Same Role | `kueue.x-k8s.io` | `localqueues` | get, list, watch | `rayManager.grants.kueueRead` |
| ClusterRole `ray-manager-clusterqueues` | `kueue.x-k8s.io` | `clusterqueues` | get, list, watch | `rayManager.grants.kueueRead` |

A binding of the same name grants each to the ServiceAccount `ray-manager` in `default`. Every call the control plane makes to a compute cluster targets `default`, and its health probe (`/healthz`) needs no grant. The cluster-scoped names differ from the `ray-manager` ClusterRole and `ray-manager-binding` that earlier versions of the registration guide built, so both sets coexist while a cluster migrates.

:::warning What a leaked token can do
Its holder can create RayClusters (and, with `grants.spark`, SparkApplications) in `default`. That means running arbitrary images there, under any ServiceAccount in that namespace, mounting any Secret in it, the object-storage credentials included. It reaches nothing outside `default` except read access to ClusterQueues. `default` on a compute cluster should therefore hold only Michelangelo workloads and their runtime objects.
:::

### Runtime contract

Every pod the control plane creates on a compute cluster reads two objects in `default`:

- **`michelangelo-config`,** rendered verbatim from `runtime.config`: the storage root, the object-storage endpoint and the model-registry endpoint, each reachable from this cluster, plus any other settings.
- **The object-storage credentials Secret** (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`), named by controllermgr's `logPersistence.credentialsSecret`. One identity serves the log collector sidecar and the task containers, so it needs access to both the log bucket and the storage root.

controllermgr attaches the Secret whole to the Ray head and worker containers and the Spark driver and executor, whatever authored the job and whether or not log persistence is on. It is an optional `envFrom` source ahead of the ConfigMap: *optional*, so clusters without the Secret keep working; *ahead*, so a ConfigMap that still carries credentials keeps precedence and hand-built clusters see no change.

One coupling remains: controllermgr has a single `logPersistence.storageEndpoint`, which every compute cluster must reach. See [Required Connectivity](network.md#required-connectivity).

## Configure values

Every release must set the values in the chart's `examples/values-minimal.yaml`. Save them as `values-compute.yaml` and replace the placeholders:

```yaml
runtime:
  config:
    MA_FILE_SYSTEM: s3://michelangelo
    AWS_ENDPOINT_URL: https://object-storage.example.com
    REGISTRY_ENDPOINT: michelangelo-apiserver.example.com:15566
  objectStorageCredentials:
    # controllermgr's logPersistence.credentialsSecret. Create it in namespace
    # default with keys AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY.
    name: michelangelo-object-storage-credentials
```

`values.schema.json` rejects unknown top-level keys, unknown `rayManager` keys, non-boolean `enabled` toggles, and `runtime.config` values that are not strings, numbers or booleans. `templates/validations.yaml` fails rendering, before Helm creates anything, when a rule below is broken; each message starts with the values path to fix.

### Runtime config

`runtime.config` is rendered verbatim into ConfigMap `michelangelo-config`, each value as a string. Quote any value whose exact spelling matters.

| Key | Rule |
|-----|------|
| `MA_FILE_SYSTEM` | Required. The storage root, for example `s3://michelangelo` |
| `AWS_ENDPOINT_URL` | Required. The object-storage endpoint |
| `REGISTRY_ENDPOINT` | Required. The model-registry endpoint: the control plane's API server, for example `michelangelo-apiserver.example.com:15566` |
| `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` | Rejected. They belong in the [credentials Secret](#object-storage-credentials) |
| Any other key | Passed through, for example the [experiment-tracking settings](../experiment-tracking.md) |

The chart rejects only the two credential keys, because it cannot tell other secrets from settings. Tracking credentials still pass, and can move to the Secret, which reaches the same containers.

### Object-storage credentials

`runtime.objectStorageCredentials` names the Secret and says who provides it:

| Key | Default | Description |
|-----|---------|-------------|
| `name` | `""` | Required. Must equal controllermgr's `logPersistence.credentialsSecret`, which defaults to `<release>-object-storage-credentials` (`michelangelo-object-storage-credentials` for a control-plane release named `michelangelo`) |
| `create` | `false` | `false`, the production default, expects the Secret from your secret management before the first job, and keeps keys out of Helm values. `true` renders the Secret from the two keys below, for the sandbox and development |
| `accessKeyId`, `secretAccessKey` | `""` | Both set with `create: true`, both empty with `create: false` |

A rendered Secret carries `helm.sh/resource-policy: keep`, so neither uninstalling nor switching to `create: false` deletes credentials that running pods read. Where the control plane runs on the same cluster and already provides this Secret in `default` (the sandbox, or `helm/michelangelo` installed in `default`), `create` stays `false`, because two releases cannot own one object.

With `create: false`, the Secret must exist before the first job. If you create it by hand:

```bash
kubectl --context "${COMPUTE_CONTEXT}" -n default create secret generic michelangelo-object-storage-credentials \
  --from-literal=AWS_ACCESS_KEY_ID=<access-key-id> \
  --from-literal=AWS_SECRET_ACCESS_KEY=<secret-access-key>
```

### Identity

| Key | Default | Description |
|-----|---------|-------------|
| `rayManager.create` | `true` | Render the identity objects. Deleting the ServiceAccount (`false`, or an uninstall) invalidates the token the control plane registered |
| `rayManager.name` | `ray-manager` | Names the ServiceAccount, Role and RoleBinding. The token Secret is `<name>-token`; the ClusterRole and ClusterRoleBinding are `<name>-clusterqueues` |
| `rayManager.tokenSecret` | `true` | Render `<name>-token` (type `kubernetes.io/service-account-token`), which registration reads |
| `rayManager.grants.spark` | `false` | Manage SparkApplications in `default` (federated Spark dispatch) |
| `rayManager.grants.kueueRead` | `true` | Read LocalQueues in `default` and ClusterQueues |

### Test image

`tests.image` (default `busybox:1.36`) is the image of the `helm test` pod. It needs `sh` and `nc`. Override it for air-gapped registries.

### Operators

| Subchart | Version | Toggle (default) |
|---|---|---|
| `kuberay-operator` | 1.7.1 | `kuberay-operator.enabled` (`true`) |
| `spark-operator` (Kubeflow) | 2.5.2 | `spark-operator.enabled` (`true`) |

Values under a subchart's key are passed to the pinned upstream chart. Set a toggle to `false` to keep a standalone operator instead (see [Migrating existing clusters](#migrating-existing-clusters)); that component's pin and enforced settings then no longer apply.

While the Spark Operator is enabled, rendering fails unless:

- `spark-operator.spark.serviceAccount.name` is `spark-operator-spark`. The control plane sets this driver ServiceAccount on every SparkApplication.
- `spark-operator.spark.jobNamespaces` contains `default`. The operator ignores SparkApplications in unlisted namespaces.

The Spark Operator's webhook fails closed, so the chart runs two replicas of it with a disruption budget. The monitor toggles `kuberay-operator.metrics.serviceMonitor.enabled` and `spark-operator.prometheus.podMonitor.create` are off by default; both need the Prometheus Operator CRDs (`monitoring.coreos.com/v1`).

## Install

If KubeRay or the Spark Operator already runs on this cluster, or the cluster was registered by hand, read [Migrating existing clusters](#migrating-existing-clusters) first.

```bash
helm upgrade --install michelangelo-compute "$CHART" --version "$VERSION" \
  --kube-context "${COMPUTE_CONTEXT}" \
  --namespace michelangelo-compute --create-namespace \
  --values values-compute.yaml --wait
```

`--wait` returns once the operators are ready. The command is idempotent; re-run it to change values.

To install from a checkout of the repository instead, build the chart's dependencies and pass its directory as `CHART`, without `--version`. `helm dependency build` needs both upstream repositories added, even though `Chart.lock` pins the versions:

```bash
helm repo add kuberay https://ray-project.github.io/kuberay-helm/
helm repo add spark-operator https://kubeflow.github.io/spark-operator
helm dependency build helm/michelangelo-compute
CHART=./helm/michelangelo-compute
```

:::note
Helm 3.13 renders `helm template` for Kubernetes 1.28 unless `--kube-version` is set, and the chart rejects 1.28. When you render offline, pass your cluster's version, for example `--kube-version 1.29.0`.
:::

## Verify with helm test

```bash
helm test michelangelo-compute --kube-context "${COMPUTE_CONTEXT}" --namespace michelangelo-compute
```

The test pod runs in `default`, where job pods run, and checks the runtime contract:

- The credentials Secret named by `runtime.objectStorageCredentials.name` exists and sets `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`. The test never prints the keys.
- `AWS_ENDPOINT_URL` and `REGISTRY_ENDPOINT` from `michelangelo-config` are reachable. The test opens a TCP connection to each host and port; `http` and `https` URLs without a port use 80 and 443.

Each check prints `ok:` or `FAIL:` in the pod's log, and `helm test` fails if any check fails. The test checks reachability only: it does not sign in to the object store.

Run it without `--logs`. Helm reads test pods from the release namespace, not from `default`, so `--logs` fails even when the test passes. A passing test pod is deleted. A failed one stays in `default` until the next run, so read its checks with `kubectl --context "${COMPUTE_CONTEXT}" -n default logs michelangelo-compute-test-runtime-contract`.

## Apply the operator CRDs first

KubeRay and the Spark Operator ship their CRDs in `crds/`, which Helm creates at first install, if absent, and never upgrades or rolls back. Wherever those CRDs may be older than the chart's, apply them first: at a first install over an existing operator, and before an upgrade to a release that bumps either pin. Otherwise the new operator runs on the old schema, and the API server silently drops every field it lacks. `helm show chart "$CHART" --version "$VERSION"` lists the pinned versions under `dependencies`.

One procedure covers both operators. Use the one for your deployment tool.

### Helm CLI

Apply server-side, because five of these CRDs exceed client-side apply's 256 KiB annotation limit, and with `--force-conflicts`, because Helm or a GitOps tool created them:

```bash
helm show crds "$CHART" --version "$VERSION" \
  | kubectl --context "${COMPUTE_CONTEXT}" apply --server-side --force-conflicts -f -
```

`helm show crds` prints every subchart's CRDs, whatever your values. If you keep one operator standalone, filter its CRDs out, so that you do not change the schema it runs on. Set `SKIP_GROUP` to its API group, `ray.io` for KubeRay or `sparkoperator.k8s.io` for the Spark Operator:

```bash
SKIP_GROUP=sparkoperator.k8s.io
# Drop every CRD whose spec.group is SKIP_GROUP.
helm show crds "$CHART" --version "$VERSION" \
  | awk -v skip="$SKIP_GROUP" '
      /^---/ { head = ""; known = 0 }
      !known { head = head $0 "\n"
               if ($1 == "group:") { known = 1; drop = ($2 == skip); if (!drop) printf "%s", head }
               next }
      !drop' \
  | kubectl --context "${COMPUTE_CONTEXT}" apply --server-side --force-conflicts -f -
```

If you keep both operators standalone, skip this step.

### Flux

Set `install.crds` and `upgrade.crds` to `CreateReplace` on the HelmRelease:

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: michelangelo-compute
  namespace: michelangelo-compute
spec:
  # chart, interval and values as for any other release
  install:
    crds: CreateReplace
  upgrade:
    crds: CreateReplace
```

### Argo CD

Argo CD applies `crds/` on every sync, and needs `ServerSideApply=true` for these CRDs:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: michelangelo-compute
spec:
  # source, destination and values as for any other application
  syncPolicy:
    syncOptions:
      - ServerSideApply=true
```

## Upgrade

[Apply the operator CRDs first](#apply-the-operator-crds-first) if the new release bumps either pin. Then re-run the install command with the new `VERSION`:

```bash
helm upgrade --install michelangelo-compute "$CHART" --version "$VERSION" \
  --kube-context "${COMPUTE_CONTEXT}" \
  --namespace michelangelo-compute --values values-compute.yaml --wait
```

An upgrade restarts the operators' controllers, which resume reconciling existing RayClusters and SparkApplications. Their pods keep running.

### Upgrade order

- **Moving to the chart, the control plane upgrades first,** to the release that ships this chart. The chart's ConfigMap carries no credentials, so task pods must already read the Secret, and the new control plane changes nothing on hand-built clusters. controllermgr must attach the credentials Secret before any compute cluster moves to the chart.
- **Afterwards, compute clusters upgrade first.** Chart changes that a control-plane release relies on, such as grants, ship in the same platform release and are additive, so a compute cluster may run one release ahead of its control plane, never behind.

### Rollback

Rollback is a `helm rollback` of the affected release:

```bash
helm rollback michelangelo-compute --kube-context "${COMPUTE_CONTEXT}" --namespace michelangelo-compute
```

- The control plane rolls back past the release that ships this chart only while no compute cluster runs the chart, because older task pods read credentials only from the ConfigMap.
- `helm rollback` leaves the KubeRay and Spark Operator CRDs at the newer schema, so pin bumps are reviewed for CRD backward compatibility.

## Rotate the ray-manager token

The chart renders Secret `ray-manager-token`, which registration reads (see [Register a Compute Cluster](register-a-compute-cluster-to-michelangelo-control-plane.md)). The token does not expire. Anything that deletes the ServiceAccount (an uninstall, or `rayManager.create=false`) invalidates the registered token. To rotate it:

1. Delete the Secret and re-run `helm upgrade`. The chart recreates the Secret with a new token, and the old one stops validating at once:

   ```bash
   kubectl --context "${COMPUTE_CONTEXT}" -n default delete secret ray-manager-token
   helm upgrade --install michelangelo-compute "$CHART" --version "$VERSION" \
     --kube-context "${COMPUTE_CONTEXT}" \
     --namespace michelangelo-compute --values values-compute.yaml --wait
   ```

2. Re-register: read the new token and replace the control plane's `cluster-<name>-client-token` Secret, as in steps 1 and 4 of [Register a Compute Cluster](register-a-compute-cluster-to-michelangelo-control-plane.md).
3. Restart controllermgr so that it uses the new token:

   ```bash
   kubectl --context "${CONTROL_PLANE_CONTEXT}" -n <release-namespace> rollout restart deployment/michelangelo-controllermgr
   ```

Jobs dispatched to the cluster between steps 1 and 3 can fail. Once RFC-20260731 items 0e and 0f ship, the control plane picks up the re-registered token without a restart and retries the creates rejected in between, so step 3 goes away and a rotation completed within ten minutes fails no job.

## Migrating existing clusters

Helm installs over an existing object only if it carries the release's ownership metadata: the `meta.helm.sh/release-name` and `meta.helm.sh/release-namespace` annotations and the `app.kubernetes.io/managed-by: Helm` label. Otherwise the install aborts. Adding that metadata is how an object is *adopted* below. The commands add it with `kubectl`, so they work with Helm 3.13.

### Operator releases

Taking over objects that another release owns would leave two releases claiming them, and uninstalling either would delete them. A standalone install therefore moves in one of two ways, per component:

- **Replace.** Uninstall the standalone KubeRay or Spark release, then install the chart, [applying its CRDs first](#apply-the-operator-crds-first), because Helm skips `crds/` that already exist. The CRDs, which Helm never deletes, keep existing RayClusters and SparkApplications alive, and the new operator resumes reconciling them.
- **Keep.** Set `<subchart>.enabled=false` and keep the standalone release. The chart still provides the identity and the runtime objects, but that component's pin and enforced configuration no longer apply.

```bash
# Find the standalone releases
helm list --all-namespaces --kube-context "${COMPUTE_CONTEXT}"

# Replace, for example the KubeRay release that earlier guides installed
helm uninstall kuberay-operator --kube-context "${COMPUTE_CONTEXT}" --namespace ray-system
```

### Hand-built objects

Objects from an earlier version of [Register a Compute Cluster](register-a-compute-cluster-to-michelangelo-control-plane.md) or an older sandbox are handled at the first install, which the control plane's upgrade must precede ([Upgrade order](#upgrade-order)):

| Object | At the first install |
|---|---|
| ServiceAccount `ray-manager`, Secret `ray-manager-token` | Adopted. Recreating either invalidates the registered token. |
| Role and RoleBinding `ray-manager`, ClusterRole and ClusterRoleBinding `ray-manager-clusterqueues` (Kueue read grants built by hand, RFC-20260731 0d) | Adopted. The chart's rules replace theirs. |
| ClusterRole `ray-manager`, ClusterRoleBinding `ray-manager-binding` | Deleted after the install, so dispatch never loses permission. Deleting them is what narrows `ray-manager`. |
| ConfigMap `michelangelo-config` | Deleted just before the install, once its other keys are in `runtime.config`. Adopting it would keep the credential keys, because Helm leaves keys it never rendered. Pods that start in between wait until the chart recreates it. |
| Secret `aws-credentials` | Left alone: the chart does not render it, and nothing Michelangelo runs on a compute cluster reads it. |

In order:

1. Upgrade the control plane ([Upgrade order](#upgrade-order)).
2. Make sure the [credentials Secret](#object-storage-credentials) exists in `default`, or set `create: true`.
3. Copy every key of the hand-built ConfigMap except `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` into `runtime.config`:

   ```bash
   kubectl --context "${COMPUTE_CONTEXT}" -n default get configmap michelangelo-config -o yaml
   ```

4. Adopt the identity objects that exist:

   ```bash
   for obj in serviceaccount/ray-manager secret/ray-manager-token role/ray-manager rolebinding/ray-manager \
       clusterrole/ray-manager-clusterqueues clusterrolebinding/ray-manager-clusterqueues; do
     kubectl --context "${COMPUTE_CONTEXT}" -n default get "$obj" >/dev/null 2>&1 || continue
     kubectl --context "${COMPUTE_CONTEXT}" -n default annotate "$obj" --overwrite \
       meta.helm.sh/release-name=michelangelo-compute meta.helm.sh/release-namespace=michelangelo-compute
     kubectl --context "${COMPUTE_CONTEXT}" -n default label "$obj" --overwrite app.kubernetes.io/managed-by=Helm
   done
   ```

5. Replace or keep each standalone operator ([Operator releases](#operator-releases)), and [apply the operator CRDs first](#apply-the-operator-crds-first).
6. Delete the hand-built ConfigMap and install right away:

   ```bash
   kubectl --context "${COMPUTE_CONTEXT}" -n default delete configmap michelangelo-config
   helm upgrade --install michelangelo-compute "$CHART" --version "$VERSION" \
     --kube-context "${COMPUTE_CONTEXT}" \
     --namespace michelangelo-compute --create-namespace \
     --values values-compute.yaml --wait
   ```

7. Delete the legacy ClusterRole and its binding, which narrows `ray-manager` to `default`:

   ```bash
   kubectl --context "${COMPUTE_CONTEXT}" delete clusterrolebinding ray-manager-binding
   kubectl --context "${COMPUTE_CONTEXT}" delete clusterrole ray-manager
   ```

8. [Verify with helm test](#verify-with-helm-test).

A cluster registered with a token from `kubectl create token` instead of `ray-manager-token` keeps working until that token expires. Re-register it with the token from `ray-manager-token`, as in steps 1 and 4 of [Register a Compute Cluster](register-a-compute-cluster-to-michelangelo-control-plane.md).

## Uninstall

1. Deregister the cluster first, because uninstalling deletes `ray-manager` and so invalidates the registered token. On the control plane, delete its `Cluster` resource and the two Secrets that registration created:

   ```bash
   kubectl --context "${CONTROL_PLANE_CONTEXT}" -n ma-system delete cluster "${COMPUTE_CLUSTER}"
   kubectl --context "${CONTROL_PLANE_CONTEXT}" -n default delete secret \
     "cluster-${COMPUTE_CLUSTER}-client-token" "cluster-${COMPUTE_CLUSTER}-ca-data"
   ```

2. Uninstall the release:

   ```bash
   helm uninstall michelangelo-compute --kube-context "${COMPUTE_CONTEXT}" --namespace michelangelo-compute
   ```

KubeRay's and the Spark Operator's CRDs survive, along with their objects, because Helm never deletes `crds/`; no operator reconciles those objects after the uninstall. A credentials Secret that the chart created survives too.

## Related

- [Register a Compute Cluster](register-a-compute-cluster-to-michelangelo-control-plane.md)
- [Install Michelangelo AI with Helm](../helm-chart.md)
- [Network & Ingress: Multi-Cluster Network Topology](network.md#multi-cluster-network-topology)
- [Troubleshooting: Ray pods not starting](../operations/troubleshooting.md#ray-pods-not-starting-on-the-compute-cluster)
