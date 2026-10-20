# michelangelo-compute

Sets up a Kubernetes cluster to run Michelangelo compute jobs. The chart
installs the KubeRay operator and the Kubeflow Spark Operator at pinned versions
and fails rendering when a setting the control plane relies on changes.

## Install

Install one release per cluster, named `michelangelo-compute`, in namespace
`michelangelo-compute`. The operators run in the release namespace; the control
plane creates RayClusters and SparkApplications in namespace `default`. The
chart requires Kubernetes 1.29 or later.

```bash
helm repo add kuberay https://ray-project.github.io/kuberay-helm/
helm repo add spark-operator https://kubeflow.github.io/spark-operator
helm dependency build helm/michelangelo-compute
helm install michelangelo-compute helm/michelangelo-compute \
  --namespace michelangelo-compute \
  --create-namespace
```

`helm dependency build` needs both repositories added, even though `Chart.lock`
pins the versions. Helm 3.13 renders `helm template` for Kubernetes 1.28 unless
`--kube-version` is set, which the chart rejects.

## Subcharts

| Subchart | Version | Toggle (default) |
|---|---|---|
| `kuberay-operator` | 1.7.1 | `kuberay-operator.enabled` (`true`) |
| `spark-operator` (Kubeflow) | 2.5.2 | `spark-operator.enabled` (`true`) |

Values under a subchart's key are passed to the upstream chart. Helm installs
the CRDs in the subcharts' `crds/` directories on the first install only, and
never upgrades or deletes them.

The monitor toggles `kuberay-operator.metrics.serviceMonitor.enabled` and
`spark-operator.prometheus.podMonitor.create` are off by default; both need the
Prometheus Operator CRDs (`monitoring.coreos.com/v1`).

## Enforced values

While the Spark Operator is enabled, rendering fails if:

- `spark-operator.spark.serviceAccount.name` is not `spark-operator-spark`. The
  control plane sets this driver ServiceAccount on every SparkApplication.
- `spark-operator.spark.jobNamespaces` does not contain `default`. The operator
  ignores SparkApplications in unlisted namespaces.

`values.schema.json` rejects unknown top-level keys and non-boolean `enabled`
toggles.

## Development

```bash
helm dependency build helm/michelangelo-compute
bash .github/scripts/validate-compute-charts.sh
```

The script lints and renders the chart, validates the output with kubeconform
v0.8.0, and checks every enforced value. `HELM` and `KUBECONFORM` select the
binaries. CI runs it on Helm 3.13, 3.15 and 4.1
(`.github/workflows/compute-chart-ci.yml`).
