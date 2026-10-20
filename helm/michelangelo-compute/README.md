# michelangelo-compute

Sets up a Kubernetes cluster to run Michelangelo compute jobs. The chart
installs the KubeRay operator, the Kubeflow Spark Operator and, optionally,
Kueue at pinned versions, and fails rendering when a setting the control plane
relies on changes.

## Install

Install one release per cluster, named `michelangelo-compute`, in namespace
`michelangelo-compute`. The operators run in the release namespace; the control
plane creates RayClusters and SparkApplications in namespace `default`. The
chart requires Kubernetes 1.29 or later.

Every release must set the values in `examples/values-minimal.yaml`: the
storage root, the object-storage endpoint and the model-registry endpoint, each
reachable from this cluster, and the name of the object-storage credentials
Secret. Copy that file to `values-compute.yaml` and replace its placeholders.

```bash
helm repo add kuberay https://ray-project.github.io/kuberay-helm/
helm repo add spark-operator https://kubeflow.github.io/spark-operator
helm dependency build helm/michelangelo-compute
helm install michelangelo-compute helm/michelangelo-compute \
  --namespace michelangelo-compute \
  --create-namespace \
  --values values-compute.yaml
helm test michelangelo-compute --namespace michelangelo-compute
```

`helm test` checks from namespace `default` that the credentials Secret sets
both keys and that both endpoints are reachable. Its pod runs outside the
release namespace, so `helm test --logs` cannot read it. A failed test pod
stays in `default` until the next run:
`kubectl --namespace default logs michelangelo-compute-test-runtime-contract`.

`helm dependency build` needs both repositories added, even though `Chart.lock`
pins the versions; it pulls Kueue from `oci://registry.k8s.io/kueue/charts`,
which needs none. Helm 3.13 renders `helm template` for Kubernetes 1.28 unless
`--kube-version` is set, which the chart rejects.

## Subcharts

| Subchart | Version | Toggle (default) |
|---|---|---|
| `kuberay-operator` | 1.7.1 | `kuberay-operator.enabled` (`true`) |
| `spark-operator` (Kubeflow) | 2.5.2 | `spark-operator.enabled` (`true`) |
| `kueue` | 0.19.9 | `kueue.enabled` (`false`) |

Values under a subchart's key are passed to the upstream chart. Helm installs
the CRDs in the subcharts' `crds/` directories on the first install only, and
never upgrades or deletes them. Kueue renders its CRDs from templates instead:
they upgrade with the release, and uninstalling or setting `kueue.enabled` to
`false` deletes them together with every Kueue object.

The monitor toggles `kuberay-operator.metrics.serviceMonitor.enabled`,
`spark-operator.prometheus.podMonitor.create` and `kueue.enablePrometheus` are
off by default; all three need the Prometheus Operator CRDs
(`monitoring.coreos.com/v1`).

## Kueue

Kueue is opt-in per cluster: set `kueue.enabled: true`
(`examples/values-kueue.yaml`) together with `scheduler_type` on the cluster's
Cluster CR. On a cluster that is not marked, Kueue adds a fail-closed webhook
and buys nothing. `manageJobsWithoutQueueName` keeps upstream's `false`: Kueue
leaves jobs without a queue name alone, so a cluster that is not yet marked
admits everything at once.

The subchart replaces its Configuration wholesale, so
`kueue.managerConfig.controllerManagerConfigYaml` carries Kueue's full default
with four deltas:

- `managedJobsNamespaceSelector` matches only namespace `default`, where the
  control plane creates its jobs. Every job webhook takes its namespace
  selector from this field.
- `integrations.frameworks` is `[ray.io/raycluster]`, the only kind the control
  plane labels. With `pod`, `deployment` and `statefulset` off, their webhooks
  are `Ignore`, so Kueue stays out of every other Pod create.
- `waitForPodsReady` sets `timeout: 1h`, a backstop above the control plane's
  provisioning budget; `recoveryTimeout: 0s`, so Kueue never evicts a running
  cluster that lost a pod; and `requeuingStrategy.backoffLimitCount: 2`, so
  Kueue deactivates a workload after its third failed start.
- `resources.excludeResourcePrefixes` is `[ephemeral-storage]`: Ray pods
  request ephemeral storage whenever a task sets a disk size, and node-local
  disk is not rationed by quota. Any other resource a ClusterQueue does not
  cover still blocks admission.

Every Kueue replica serves its webhooks, so the chart runs two with a
PodDisruptionBudget.

To admit SparkApplications through Kueue, append
`sparkoperator.k8s.io/sparkapplication` to `integrations.frameworks` and enable
its feature gate, without which Kueue does not start:

```yaml
kueue:
  controllerManager:
    featureGates:
      - name: SparkApplicationIntegration
        enabled: true
```

CI fails if the Configuration differs from the pinned subchart's default
anywhere outside the deltas, which catches upstream changes at every Kueue
bump. With `kueue.enabled: false`, an existing Kueue can be used if it serves
`v1beta2` (0.15 or later) and carries the same deltas.

## Enforced values

Rendering always fails if:

- `runtime.config` lacks `MA_FILE_SYSTEM`, `AWS_ENDPOINT_URL` or
  `REGISTRY_ENDPOINT`, or sets `AWS_ACCESS_KEY_ID` or `AWS_SECRET_ACCESS_KEY`.
  The credentials belong in the Secret.
- `runtime.objectStorageCredentials.name` is empty. It must match
  controllermgr's `logPersistence.credentialsSecret`.
- `runtime.objectStorageCredentials.accessKeyId` and `secretAccessKey` are not
  both set with `create: true`, or not both empty with `create: false`.

While the Spark Operator is enabled, rendering fails if:

- `spark-operator.spark.serviceAccount.name` is not `spark-operator-spark`. The
  control plane sets this driver ServiceAccount on every SparkApplication.
- `spark-operator.spark.jobNamespaces` does not contain `default`. The operator
  ignores SparkApplications in unlisted namespaces.

While Kueue is enabled, rendering fails if:

- `kueue.managerConfig.controllerManagerConfigYaml` changes one of the four
  deltas, spelled as above, or lists a framework other than
  `ray.io/raycluster` followed by `sparkoperator.k8s.io/sparkapplication`.
- The Spark framework is listed without `kueue.controllerManager.featureGates`
  enabling `SparkApplicationIntegration`.

`values.schema.json` rejects unknown top-level keys, non-boolean `enabled`
toggles, and `runtime.config` values that are not strings, numbers or booleans.

## Development

```bash
helm dependency build helm/michelangelo-compute
bash .github/scripts/validate-compute-charts.sh
```

The script lints and renders the chart, validates the output with kubeconform
v0.8.0, checks every enforced value, and compares the Kueue Configuration with
the pinned subchart's default using yq v4. `HELM`, `KUBECONFORM` and `YQ`
select the binaries. CI runs it on Helm 3.13, 3.15 and 4.1
(`.github/workflows/compute-chart-ci.yml`).
