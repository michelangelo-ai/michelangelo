#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHART_DIR="$REPO_ROOT/helm/michelangelo-llm-gateway"
DEPENDENCY="$CHART_DIR/charts/litellm-helm-1.85.1.tgz"
EXPECTED_DEPENDENCY_SHA256="40909115b2c8fb0b27e9446549f18d8105fd057e2a7d30015595bd7d3065dc9a"

if [[ ! -f "$DEPENDENCY" ]]; then
  echo "Missing $DEPENDENCY; run helm dependency build $CHART_DIR" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  dependency_sha256="$(sha256sum "$DEPENDENCY" | awk '{print $1}')"
else
  dependency_sha256="$(shasum -a 256 "$DEPENDENCY" | awk '{print $1}')"
fi
if [[ "$dependency_sha256" != "$EXPECTED_DEPENDENCY_SHA256" ]]; then
  echo "Unexpected litellm-helm dependency digest: $dependency_sha256" >&2
  exit 1
fi

render() {
  helm template litellm "$CHART_DIR" \
    --namespace litellm \
    --kube-version 1.27.0 \
    "$@" \
    >/dev/null
}

validate_profile() {
  helm lint --strict "$CHART_DIR" "$@"
  render "$@"
}

expect_failure() {
  local name="$1"
  local expected="$2"
  local normalized_output
  local output
  shift 2

  if output="$(helm template litellm "$CHART_DIR" "$@" 2>&1)"; then
    echo "Expected validation failure: $name" >&2
    exit 1
  fi
  normalized_output="${output//\//.}"
  if [[ "$output" != *"$expected"* && "$normalized_output" != *"$expected"* ]]; then
    printf 'Validation failed for the wrong reason: %s\n%s\n' "$name" "$output" >&2
    exit 1
  fi
}

assert_contains() {
  local output="$1"
  local expected="$2"

  if [[ "$output" != *"$expected"* ]]; then
    echo "Expected rendered output to contain: $expected" >&2
    exit 1
  fi
}

validate_profile
validate_profile -f "$CHART_DIR/examples/values-gcp.yaml"
validate_profile \
  --set migrationJob.hooks.helm.enabled=false \
  --set migrationJob.hooks.argocd.enabled=true
validate_profile \
  --set litellm.proxyConfigMap.create=false \
  --set litellm.proxyConfigMap.name=litellm-config \
  --set migrationJob.enabled=false \
  --set migrationJob.managedExternally=true \
  --set migrationJob.hooks.helm.enabled=false
validate_profile \
  --set networkPolicy.enabled=true \
  --set networkPolicy.migration.managedExternally=true \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.authorization.secretName=litellm-metrics
validate_profile \
  --set litellm.autoscaling.enabled=false \
  --set litellm.keda.enabled=true \
  --set 'litellm.keda.triggers[0].type=cpu' \
  --set-string 'litellm.keda.triggers[0].metadata.type=Utilization' \
  --set-string 'litellm.keda.triggers[0].metadata.value=70'
render --set-string litellm.resources.requests.cpu=1E

default_output="$(helm template litellm "$CHART_DIR" --namespace litellm)"
assert_contains "$default_output" 'ghcr.io/berriai/litellm-database:v1.85.1@sha256:fb757549de0da017a597ee4b9be0bfb55c49cde77898abe789fa3ad37d770c69'
assert_contains "$default_output" 'curlimages/curl:8.12.1@sha256:94e9e444bcba979c2ea12e27ae39bee4cd10bc7041a472c4727a558e213744e6'
assert_contains "$default_output" '--use_v2_migration_resolver'
assert_contains "$default_output" '--enforce_prisma_migration_check'

private_registry_values=(
  --set 'litellm.imagePullSecrets[0].name=registry-creds'
  --set litellm.image.repository=registry.example.com/litellm
  --set tests.image.repository=registry.example.com/curl
)
private_registry_output="$(helm template litellm "$CHART_DIR" \
  --namespace litellm \
  "${private_registry_values[@]}")"
assert_contains "$private_registry_output" 'registry.example.com/litellm:v1.85.1@sha256:'
assert_contains "$private_registry_output" 'registry.example.com/curl:8.12.1@sha256:'

pull_secret_count=0
remaining_output="$private_registry_output"
while [[ "$remaining_output" == *'- name: registry-creds'* ]]; do
  remaining_output="${remaining_output#*'- name: registry-creds'}"
  ((pull_secret_count += 1))
done
if [[ "$pull_secret_count" -ne 3 ]]; then
  echo "Expected imagePullSecrets on the Deployment, migration Job, and wrapper test" >&2
  exit 1
fi

expect_failure "dependency migration ownership" "litellm.migrationJob.enabled" \
  --set litellm.migrationJob.enabled=true
expect_failure "dependency ServiceMonitor ownership" "litellm.serviceMonitor.enabled" \
  --set litellm.serviceMonitor.enabled=true
expect_failure "database ownership" "litellm.db.deployStandalone" \
  --set litellm.db.deployStandalone=true
expect_failure "stable dependency name" "litellm.fullnameOverride" \
  --set litellm.fullnameOverride=other
expect_failure "invalid Service type" "litellm.service.type" \
  --set litellm.service.type=Banana
expect_failure "negative termination grace period" "litellm.terminationGracePeriodSeconds" \
  --set litellm.terminationGracePeriodSeconds=-1
expect_failure "invalid startup probe period" "litellm.startupProbe.periodSeconds" \
  --set litellm.startupProbe.periodSeconds=0
expect_failure "invalid liveness success threshold" "litellm.livenessProbe.successThreshold" \
  --set litellm.livenessProbe.successThreshold=2
expect_failure "invalid Deployment strategy" "litellm.strategy.type" \
  --set litellm.strategy.type=Banana
expect_failure "unavailable RollingUpdate" "maxSurge and maxUnavailable cannot both be zero" \
  --set litellm.strategy.rollingUpdate.maxSurge=0 \
  --set litellm.strategy.rollingUpdate.maxUnavailable=0
expect_failure "invalid CPU quantity" "litellm.resources.requests.cpu" \
  --set litellm.resources.requests.cpu=bananas
expect_failure "invalid CPU suffix" "litellm.resources.requests.cpu" \
  --set-string litellm.resources.requests.cpu=1K
expect_failure "Ingress without rules" "litellm.ingress.hosts" \
  --set litellm.ingress.enabled=true
expect_failure "invalid Ingress path" "litellm.ingress.hosts.0.paths.0.path" \
  --set litellm.ingress.enabled=true \
  --set 'litellm.ingress.hosts[0].host=gateway.example.com' \
  --set 'litellm.ingress.hosts[0].paths[0].path=no-leading-slash' \
  --set 'litellm.ingress.hosts[0].paths[0].pathType=Prefix'
expect_failure "invalid Ingress class" "litellm.ingress.className" \
  --set litellm.ingress.className=Bad_Class
expect_failure "invalid load balancer class" "litellm.service.loadBalancerClass" \
  --set-string 'litellm.service.loadBalancerClass=Bad Class'
expect_failure "selector label override" "app.kubernetes.io/name" \
  --set-string 'litellm.podLabels.app\.kubernetes\.io/name=other'
expect_failure "config checksum override" "cannot override chart-owned checksum/config" \
  --set-string 'litellm.podAnnotations.checksum/config=override'
expect_failure "numeric Pod annotation" "litellm.podAnnotations.foo" \
  --set litellm.podAnnotations.foo=1
expect_failure "invalid Pod security context" "litellm.podSecurityContext" \
  --set litellm.podSecurityContext=1
expect_failure "invalid container security context" "litellm.securityContext" \
  --set litellm.securityContext=1
expect_failure "numeric node selector" "litellm.nodeSelector.pool" \
  --set litellm.nodeSelector.pool=1
expect_failure "invalid toleration" "litellm.tolerations.0" \
  --set 'litellm.tolerations[0]=invalid'
expect_failure "invalid affinity" "litellm.affinity" \
  --set litellm.affinity=invalid
expect_failure "invalid command" "litellm.command" \
  --set litellm.command=litellm
expect_failure "invalid argument" "litellm.args.0" \
  --set 'litellm.args[0]=1'
expect_failure "invalid priority class" "litellm.priorityClassName" \
  --set litellm.priorityClassName=Bad_Name
expect_failure "ServiceAccount token policy" "litellm.serviceAccount.create" \
  --set litellm.serviceAccount.create=false
expect_failure "invalid ServiceAccount name" "litellm.serviceAccount.name" \
  --set litellm.serviceAccount.name=a..b
expect_failure "migration ownership" "exactly one of migrationJob.enabled" \
  --set migrationJob.managedExternally=true
expect_failure "migration hook ownership" "require exactly one" \
  --set migrationJob.hooks.argocd.enabled=true
expect_failure "reserved migration annotation" "cannot override chart-owned helm.sh/hook" \
  --set-string 'migrationJob.annotations.helm\.sh/hook=post-install'
expect_failure "migration NetworkPolicy prerequisite" "pre-existing externally managed migration NetworkPolicy" \
  --set networkPolicy.enabled=true
expect_failure "ServiceMonitor authentication" "authorization.secretName" \
  --set serviceMonitor.enabled=true
expect_failure "external ConfigMap" "proxyConfigMap.name is required" \
  --set litellm.proxyConfigMap.create=false
expect_failure "PDB availability" "set only one of litellm.pdb" \
  --set litellm.pdb.minAvailable=1
expect_failure "invalid PDB availability" "litellm.pdb.maxUnavailable" \
  --set-string litellm.pdb.maxUnavailable=bogus
expect_failure "invalid topology spread" "litellm.topologySpreadConstraints.0" \
  --set 'litellm.topologySpreadConstraints[0].maxSkew=0'
expect_failure "autoscaling bounds" "minReplicas cannot exceed" \
  --set litellm.autoscaling.minReplicas=11
expect_failure "invalid autoscaling target" "litellm.autoscaling.targetCPUUtilizationPercentage" \
  --set litellm.autoscaling.targetCPUUtilizationPercentage=0
expect_failure "KEDA without triggers" "litellm.keda.triggers" \
  --set litellm.autoscaling.enabled=false \
  --set litellm.keda.enabled=true
expect_failure "plaintext secret" "cannot contain reserved or sensitive variable API_TOKEN" \
  --set litellm.envVars.API_TOKEN=secret
expect_failure "runtime image digest" "litellm.image.tag" \
  --set litellm.image.tag=v1.85.1
expect_failure "test image digest" "tests.image.tag" \
  --set tests.image.tag=8.12.1
expect_failure "unknown wrapper value" "unknownGatewayValue" \
  --set unknownGatewayValue=true

if [[ "${SKIP_PACKAGE:-false}" != "true" ]]; then
  package_dir="$(mktemp -d)"
  trap 'rm -rf "$package_dir"' EXIT
  helm package "$CHART_DIR" --destination "$package_dir" >/dev/null
  chart_version="$(awk '$1 == "version:" {gsub(/"/, "", $2); print $2; exit}' "$CHART_DIR/Chart.yaml")"
  package="$package_dir/michelangelo-llm-gateway-$chart_version.tgz"
  package_contents="$(tar tzf "$package")"
  assert_contains "$package_contents" 'charts/litellm-helm/Chart.yaml'
  assert_contains "$package_contents" 'examples/values-gcp.yaml'
fi

echo "Validated $CHART_DIR"
