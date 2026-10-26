#!/usr/bin/env bash
# Render checks for helm/michelangelo-compute; run after helm dependency build.
# HELM and KUBECONFORM select the binaries, so the same checks run on every
# supported Helm version.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHART_DIR="$REPO_ROOT/helm/michelangelo-compute"
HELM="${HELM:-helm}"
KUBECONFORM="${KUBECONFORM:-kubeconform}"
RELEASE=michelangelo-compute
NAMESPACE=michelangelo-compute
# The chart's kubeVersion floor, pinned because each Helm version renders with
# a different default.
KUBE_VERSION=1.29.0
# Sets the runtime settings that have no default. Every render uses it unless
# VALUES_FILE is cleared.
VALUES_FILE="$CHART_DIR/examples/values-minimal.yaml"
# Archives written to charts/ by helm dependency build, with their sha256.
DEPENDENCIES=(
  "kuberay-operator-1.7.1.tgz fd057c207a6be038f2fe1475abb87f43a8c091b77f1f563b4e8547a2404b5bc8"
  "spark-operator-2.5.2.tgz 762be5b8632ecfe12eb20fff54450ddae0427f09506422107c508a0d1d38655b"
)
CORE_SCHEMA_LOCATION='https://raw.githubusercontent.com/yannh/kubernetes-json-schema/c8f4e61c63bc529749125ac566bccc6986e08d45/{{ .NormalizedKubernetesVersion }}-standalone{{ .StrictSuffix }}/{{ .ResourceKind }}{{ .KindSuffix }}.json'
CRD_SCHEMA_LOCATION='https://raw.githubusercontent.com/datreeio/CRDs-catalog/52b0261318acc7dd0b66e032759b1f218216b980/{{ .Group }}/{{ .ResourceKind }}_{{ .ResourceAPIVersion }}.json'

if ! command -v "$HELM" >/dev/null 2>&1; then
  echo "Missing $HELM; install Helm or set HELM" >&2
  exit 1
fi

if ! command -v "$KUBECONFORM" >/dev/null 2>&1; then
  echo "Missing $KUBECONFORM; install kubeconform v0.8.0 or set KUBECONFORM" >&2
  exit 1
fi

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

for dependency in "${DEPENDENCIES[@]}"; do
  read -r archive expected_sha256 <<<"$dependency"
  if [[ ! -f "$CHART_DIR/charts/$archive" ]]; then
    echo "Missing $CHART_DIR/charts/$archive; run helm dependency build $CHART_DIR" >&2
    exit 1
  fi
  actual_sha256="$(sha256 "$CHART_DIR/charts/$archive")"
  if [[ "$actual_sha256" != "$expected_sha256" ]]; then
    echo "Unexpected $archive digest: $actual_sha256" >&2
    exit 1
  fi
done

# NOTES print after the release is applied, so they must not carry a CRD step.
if grep -qiE 'crd|kubectl apply' "$CHART_DIR/templates/NOTES.txt"; then
  echo "templates/NOTES.txt must not contain a CRD step" >&2
  exit 1
fi

# Every helm template call gets the pinned --kube-version here. Profile
# arguments also feed helm lint, which has no --kube-version on Helm 3.13, so
# they must never carry it.
template() {
  "$HELM" template "$RELEASE" "$CHART_DIR" \
    --namespace "$NAMESPACE" \
    --kube-version "$KUBE_VERSION" \
    ${VALUES_FILE:+--values "$VALUES_FILE"} \
    "$@"
}

render() {
  template "$@" | "$KUBECONFORM" \
    -strict \
    -summary \
    -kubernetes-version "$KUBE_VERSION" \
    -schema-location "$CORE_SCHEMA_LOCATION" \
    -schema-location "$CRD_SCHEMA_LOCATION"
}

validate_profile() {
  "$HELM" lint --strict "$CHART_DIR" --namespace "$NAMESPACE" --values "$VALUES_FILE" "$@"
  render "$@"
}

# expected is fail text from templates/validations.yaml or a dotted values
# path. Helm 3 and Helm 4 word schema errors differently, and Helm 4 prints
# paths as JSON pointers, so the output is also compared with / read as .
expect_failure() {
  local name="$1"
  local expected="$2"
  local output
  shift 2

  if output="$(template "$@" 2>&1)"; then
    echo "Expected rendering to fail: $name" >&2
    exit 1
  fi
  if [[ "$output" != *"$expected"* && "${output//\//.}" != *"$expected"* ]]; then
    printf 'Rendering failed for the wrong reason: %s\n%s\n' "$name" "$output" >&2
    exit 1
  fi
  echo "Rejected as expected: $name"
}

assert_contains() {
  local output="$1"
  local expected="$2"

  if [[ "$output" != *"$expected"* ]]; then
    echo "Expected rendered output to contain: $expected" >&2
    exit 1
  fi
}

assert_not_contains() {
  local output="$1"
  local unexpected="$2"

  if [[ "$output" == *"$unexpected"* ]]; then
    echo "Expected rendered output not to contain: $unexpected" >&2
    exit 1
  fi
}

# Prints "<kind> <namespace> <name>" per rendered object, with - when the
# object sets no metadata.namespace.
objects() {
  awk '
    function unquoted(value) {
      gsub(/^["'\'']|["'\'']$/, "", value)
      return value
    }
    function emit() {
      if (kind != "") print kind " " (namespace == "" ? "-" : namespace) " " name
      kind = ""
      namespace = ""
      name = ""
      in_metadata = 0
    }
    /^---/ { emit(); next }
    /^[^ #]/ { in_metadata = ($0 ~ /^metadata:/) }
    /^kind: / { kind = $2 }
    in_metadata && /^  name: / { name = unquoted($2) }
    in_metadata && /^  namespace: / { namespace = unquoted($2) }
    END { emit() }
  ' <<<"$1"
}

assert_object() {
  local objects="$1"
  local expected="$2"

  if ! grep -qxF -- "$expected" <<<"$objects"; then
    echo "Expected rendered object: $expected" >&2
    exit 1
  fi
}

# Helm installs a namespaced object without metadata.namespace in the release
# namespace.
assert_in_release_namespace() {
  local objects="$1"
  local kind="$2"
  local name="$3"

  if ! grep -qxF -e "$kind $NAMESPACE $name" -e "$kind - $name" <<<"$objects"; then
    echo "Expected $kind $name in the release namespace" >&2
    exit 1
  fi
}

assert_object_count() {
  local objects="$1"
  local kind="$2"
  local expected="$3"
  local actual

  actual="$(awk -v kind="$kind" '$1 == kind { count++ } END { print count + 0 }' <<<"$objects")"
  if [[ "$actual" -ne "$expected" ]]; then
    echo "Expected $expected $kind objects, rendered $actual" >&2
    exit 1
  fi
}

validate_profile
default_output="$(template)"
default_objects="$(objects "$default_output")"
assert_in_release_namespace "$default_objects" Deployment kuberay-operator
assert_in_release_namespace "$default_objects" Deployment michelangelo-compute-spark-operator-controller
assert_in_release_namespace "$default_objects" Deployment michelangelo-compute-spark-operator-webhook
assert_object_count "$default_objects" Deployment 3
assert_object "$default_objects" 'ServiceAccount default spark-operator-spark'
assert_object "$default_objects" 'Role default spark-operator-spark'
assert_object "$default_objects" 'RoleBinding default spark-operator-spark'
assert_in_release_namespace "$default_objects" PodDisruptionBudget michelangelo-compute-spark-operator-webhook-pdb
assert_object_count "$default_objects" ServiceMonitor 0
assert_object_count "$default_objects" PodMonitor 0

webhook_output="$(template --show-only charts/spark-operator/templates/webhook/deployment.yaml)"
assert_contains "$webhook_output" $'spec:\n  replicas: 2\n'
webhook_pdb_output="$(template --show-only charts/spark-operator/templates/webhook/poddisruptionbudget.yaml)"
assert_contains "$webhook_pdb_output" $'\n  minAvailable: 1'

# Control-plane identity. Helm prints a Role before its RoleBinding and a
# ClusterRole before its ClusterRoleBinding, so a rules block followed by ---
# is that object's complete rule list.
ray_rule='  - apiGroups:
      - ray.io
    resources:
      - rayclusters
      - rayjobs
    verbs:
      - create
      - get
      - list
      - watch
      - update
      - patch
      - delete'
pods_rule='  - apiGroups:
      - ""
    resources:
      - pods
    verbs:
      - get
      - list
      - watch'
spark_rule='  - apiGroups:
      - sparkoperator.k8s.io
    resources:
      - sparkapplications
    verbs:
      - create
      - get
      - list
      - watch
      - update
      - patch
      - delete'
localqueues_rule='  - apiGroups:
      - kueue.x-k8s.io
    resources:
      - localqueues
    verbs:
      - get
      - list
      - watch'
clusterqueues_rule='  - apiGroups:
      - kueue.x-k8s.io
    resources:
      - clusterqueues
    verbs:
      - get
      - list
      - watch'

# Asserts that every identity object and reference is named after name.
assert_identity() {
  local output="$1"
  local name="$2"
  local identity_objects

  identity_objects="$(objects "$output")"
  assert_object "$identity_objects" "ServiceAccount default $name"
  assert_object "$identity_objects" "Secret default $name-token"
  assert_object "$identity_objects" "Role default $name"
  assert_object "$identity_objects" "RoleBinding default $name"
  assert_object "$identity_objects" "ClusterRole - $name-clusterqueues"
  assert_object "$identity_objects" "ClusterRoleBinding - $name-clusterqueues"
  assert_contains "$output" $'  annotations:\n    kubernetes.io/service-account.name: '"$name"$'\ntype: kubernetes.io/service-account-token'
  assert_contains "$output" $'  kind: Role\n  name: '"$name"$'\nsubjects:\n  - kind: ServiceAccount\n    name: '"$name"$'\n    namespace: default'
  assert_contains "$output" $'  kind: ClusterRole\n  name: '"$name"$'-clusterqueues\nsubjects:\n  - kind: ServiceAccount\n    name: '"$name"$'\n    namespace: default'
}

identity_output="$(template --show-only templates/rbac/ray-manager.yaml)"
assert_identity "$identity_output" ray-manager
assert_contains "$identity_output" $'rules:\n'"$ray_rule"$'\n'"$pods_rule"$'\n'"$localqueues_rule"$'\n---'
assert_contains "$identity_output" $'rules:\n'"$clusterqueues_rule"$'\n---'

spark_grant_output="$(template --show-only templates/rbac/ray-manager.yaml --set rayManager.grants.spark=true)"
assert_contains "$spark_grant_output" $'rules:\n'"$ray_rule"$'\n'"$pods_rule"$'\n'"$spark_rule"$'\n'"$localqueues_rule"$'\n---'

no_kueue_output="$(template --show-only templates/rbac/ray-manager.yaml --set rayManager.grants.kueueRead=false)"
assert_contains "$no_kueue_output" $'rules:\n'"$ray_rule"$'\n'"$pods_rule"$'\n---'
assert_not_contains "$no_kueue_output" clusterqueues

no_token_objects="$(objects "$(template --show-only templates/rbac/ray-manager.yaml --set rayManager.tokenSecret=false)")"
assert_object "$no_token_objects" 'ServiceAccount default ray-manager'
assert_object_count "$no_token_objects" Secret 0

no_identity_objects="$(objects "$(template --set rayManager.create=false)")"
assert_not_contains "$no_identity_objects" ray-manager

custom_name_output="$(template --show-only templates/rbac/ray-manager.yaml --set rayManager.name=michelangelo-control-plane)"
assert_identity "$custom_name_output" michelangelo-control-plane
assert_not_contains "$(objects "$custom_name_output")" ray-manager

# Runtime contract. examples/values-minimal.yaml sets the three required
# settings and the Secret name, with create false.
assert_object "$default_objects" 'ConfigMap default michelangelo-config'
assert_not_contains "$default_objects" 'Secret default michelangelo-object-storage-credentials'
assert_object "$default_objects" 'Pod default michelangelo-compute-test-runtime-contract'

# runtime.config renders verbatim with sorted keys and every value a string.
runtime_config_values=(
  --set runtime.config.MA_FILE_SYSTEM_S3_SCHEME=http
  --set runtime.config.MA_RETRIES=3
  --set runtime.config.MA_TRACKING_ENABLED=true
)
validate_profile "${runtime_config_values[@]}"
config_output="$(template --show-only templates/runtime/configmap.yaml "${runtime_config_values[@]}")"
assert_contains "$config_output" 'data:
  AWS_ENDPOINT_URL: "https://object-storage.example.com"
  MA_FILE_SYSTEM: "s3://michelangelo"
  MA_FILE_SYSTEM_S3_SCHEME: "http"
  MA_RETRIES: "3"
  MA_TRACKING_ENABLED: "true"
  REGISTRY_ENDPOINT: "michelangelo-apiserver.example.com:15566"'

credentials_values=(
  --set runtime.objectStorageCredentials.create=true
  --set runtime.objectStorageCredentials.accessKeyId=test-access-key-id
  --set runtime.objectStorageCredentials.secretAccessKey=test-secret-access-key
)
validate_profile "${credentials_values[@]}"
credentials_output="$(template --show-only templates/runtime/secret.yaml "${credentials_values[@]}")"
assert_object "$(objects "$credentials_output")" 'Secret default michelangelo-object-storage-credentials'
assert_contains "$credentials_output" $'  annotations:\n    helm.sh/resource-policy: keep\n'
assert_contains "$credentials_output" 'type: Opaque
stringData:
  AWS_ACCESS_KEY_ID: "test-access-key-id"
  AWS_SECRET_ACCESS_KEY: "test-secret-access-key"'

test_pod_output="$(template --show-only templates/tests/test-runtime-contract.yaml)"
assert_contains "$test_pod_output" $'  annotations:\n    helm.sh/hook: test\n    helm.sh/hook-delete-policy: before-hook-creation,hook-succeeded\n'
assert_contains "$test_pod_output" $'\n  automountServiceAccountToken: false\n'
# busybox runs as root, so runAsNonRoot needs a non-root user to start.
assert_contains "$test_pod_output" $'\n    runAsNonRoot: true\n    runAsUser: 65534\n    runAsGroup: 65534\n'
assert_contains "$test_pod_output" $'\n      image: "busybox:1.36"\n'
assert_contains "$test_pod_output" $'      envFrom:\n        - configMapRef:\n            name: michelangelo-config\n'
assert_contains "$test_pod_output" $'      secret:\n        secretName: "michelangelo-object-storage-credentials"\n        optional: true'
test_image_output="$(template --show-only templates/tests/test-runtime-contract.yaml --set tests.image=registry.example.com/busybox:1.36)"
assert_contains "$test_image_output" $'\n      image: "registry.example.com/busybox:1.36"\n'

# The Spark invariants no longer apply once the subchart is disabled.
spark_disabled_values=(
  --set spark-operator.enabled=false
  --set spark-operator.spark.serviceAccount.name=other
  --set-json 'spark-operator.spark.jobNamespaces=[]'
)
validate_profile "${spark_disabled_values[@]}"
spark_disabled_output="$(template "${spark_disabled_values[@]}")"
spark_disabled_objects="$(objects "$spark_disabled_output")"
assert_in_release_namespace "$spark_disabled_objects" Deployment kuberay-operator
assert_object_count "$spark_disabled_objects" Deployment 1
assert_not_contains "$spark_disabled_objects" spark

operators_disabled_values=(
  --set kuberay-operator.enabled=false
  --set spark-operator.enabled=false
)
validate_profile "${operators_disabled_values[@]}"
operators_disabled_output="$(template "${operators_disabled_values[@]}")"
operators_disabled_objects="$(objects "$operators_disabled_output")"
assert_object_count "$operators_disabled_objects" Deployment 0

# Spark's PodMonitor template fails unless the API is served, and helm lint has
# no --api-versions flag, so this profile is only rendered.
monitoring_values=(
  --set kuberay-operator.metrics.serviceMonitor.enabled=true
  --set spark-operator.prometheus.podMonitor.create=true
  --api-versions monitoring.coreos.com/v1/PodMonitor
)
render "${monitoring_values[@]}"
monitoring_output="$(template "${monitoring_values[@]}")"
monitoring_objects="$(objects "$monitoring_output")"
assert_in_release_namespace "$monitoring_objects" ServiceMonitor kuberay-operator
assert_in_release_namespace "$monitoring_objects" PodMonitor michelangelo-compute-spark-operator-podmonitor

expect_failure "Spark driver ServiceAccount" \
  "spark-operator.spark.serviceAccount.name must be spark-operator-spark" \
  --set spark-operator.spark.serviceAccount.name=other
expect_failure "Spark job namespaces without default" \
  "spark-operator.spark.jobNamespaces must contain default" \
  --set-json 'spark-operator.spark.jobNamespaces=["spark-jobs"]'
expect_failure "Spark job namespaces set to all namespaces" \
  "spark-operator.spark.jobNamespaces must contain default" \
  --set-json 'spark-operator.spark.jobNamespaces=[""]'
expect_failure "unknown top-level value" "unknownComputeValue" \
  --set unknownComputeValue=true
expect_failure "unknown rayManager value" "unknownIdentityValue" \
  --set rayManager.unknownIdentityValue=true
expect_failure "non-boolean subchart toggle" "spark-operator.enabled" \
  --set-string spark-operator.enabled=false
expect_failure "Kubernetes older than the chart's kubeVersion" ">=1.29.0-0" \
  --kube-version 1.28.0

# Without examples/values-minimal.yaml, the defaults leave the runtime settings
# unset.
VALUES_FILE='' expect_failure "chart defaults alone" "runtime.config.MA_FILE_SYSTEM must be set"
expect_failure "runtime.config without MA_FILE_SYSTEM" "runtime.config.MA_FILE_SYSTEM must be set" \
  --set-json 'runtime.config={"AWS_ENDPOINT_URL":"https://object-storage.example.com","REGISTRY_ENDPOINT":"michelangelo-apiserver.example.com:15566"}'
expect_failure "runtime.config without AWS_ENDPOINT_URL" "runtime.config.AWS_ENDPOINT_URL must be set" \
  --set-json 'runtime.config={"MA_FILE_SYSTEM":"s3://michelangelo","REGISTRY_ENDPOINT":"michelangelo-apiserver.example.com:15566"}'
expect_failure "runtime.config without REGISTRY_ENDPOINT" "runtime.config.REGISTRY_ENDPOINT must be set" \
  --set-json 'runtime.config={"MA_FILE_SYSTEM":"s3://michelangelo","AWS_ENDPOINT_URL":"https://object-storage.example.com"}'
expect_failure "empty runtime.config.REGISTRY_ENDPOINT" "runtime.config.REGISTRY_ENDPOINT must be set" \
  --set runtime.config.REGISTRY_ENDPOINT=
expect_failure "runtime.config with AWS_ACCESS_KEY_ID" "runtime.config.AWS_ACCESS_KEY_ID must not be set" \
  --set runtime.config.AWS_ACCESS_KEY_ID=test-access-key-id
expect_failure "runtime.config with AWS_SECRET_ACCESS_KEY" "runtime.config.AWS_SECRET_ACCESS_KEY must not be set" \
  --set runtime.config.AWS_SECRET_ACCESS_KEY=test-secret-access-key
expect_failure "non-scalar runtime.config value" "runtime.config.MA_NON_SCALAR" \
  --set-json 'runtime.config.MA_NON_SCALAR={"nested":"value"}'
expect_failure "credentials Secret without a name" "runtime.objectStorageCredentials.name must be set" \
  --set runtime.objectStorageCredentials.name=
expect_failure "created credentials Secret without secretAccessKey" \
  "runtime.objectStorageCredentials.secretAccessKey must be set when" \
  --set runtime.objectStorageCredentials.create=true \
  --set runtime.objectStorageCredentials.accessKeyId=test-access-key-id
expect_failure "external credentials Secret with accessKeyId" \
  "runtime.objectStorageCredentials.accessKeyId must be empty when" \
  --set runtime.objectStorageCredentials.accessKeyId=test-access-key-id

if [[ "${SKIP_PACKAGE:-false}" != "true" ]]; then
  package_dir="$(mktemp -d)"
  trap 'rm -rf "$package_dir"' EXIT
  "$HELM" package "$CHART_DIR" --destination "$package_dir" >/dev/null
  chart_version="$(awk '$1 == "version:" {gsub(/"/, "", $2); print $2; exit}' "$CHART_DIR/Chart.yaml")"
  package_contents="$(tar tzf "$package_dir/michelangelo-compute-$chart_version.tgz")"
  assert_contains "$package_contents" 'michelangelo-compute/values.schema.json'
  assert_contains "$package_contents" 'michelangelo-compute/examples/values-minimal.yaml'
  assert_contains "$package_contents" 'michelangelo-compute/charts/kuberay-operator/Chart.yaml'
  assert_contains "$package_contents" 'michelangelo-compute/charts/kuberay-operator/crds/ray.io_rayclusters.yaml'
  assert_contains "$package_contents" 'michelangelo-compute/charts/spark-operator/Chart.yaml'
  assert_contains "$package_contents" 'michelangelo-compute/charts/spark-operator/crds/sparkoperator.k8s.io_sparkapplications.yaml'
fi

echo "Validated $CHART_DIR"
