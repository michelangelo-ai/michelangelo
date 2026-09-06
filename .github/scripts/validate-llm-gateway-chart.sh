#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHART_DIR="$REPO_ROOT/helm/michelangelo-llm-gateway"
DEPENDENCY="$CHART_DIR/charts/litellm-helm-1.85.1.tgz"
EXPECTED_DEPENDENCY_SHA256="40909115b2c8fb0b27e9446549f18d8105fd057e2a7d30015595bd7d3065dc9a"
KUBECONFORM_BIN="${KUBECONFORM_BIN:-kubeconform}"
CORE_SCHEMA_LOCATION='https://raw.githubusercontent.com/yannh/kubernetes-json-schema/c8f4e61c63bc529749125ac566bccc6986e08d45/{{ .NormalizedKubernetesVersion }}-standalone{{ .StrictSuffix }}/{{ .ResourceKind }}{{ .KindSuffix }}.json'
CRD_SCHEMA_LOCATION='https://raw.githubusercontent.com/datreeio/CRDs-catalog/52b0261318acc7dd0b66e032759b1f218216b980/{{ .Group }}/{{ .ResourceKind }}_{{ .ResourceAPIVersion }}.json'

if ! command -v "$KUBECONFORM_BIN" >/dev/null 2>&1; then
  echo "Missing $KUBECONFORM_BIN; install kubeconform v0.8.0" >&2
  exit 1
fi

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
    "$@" | "$KUBECONFORM_BIN" \
      -strict \
      -summary \
      -kubernetes-version 1.27.0 \
      -schema-location "$CORE_SCHEMA_LOCATION" \
      -schema-location "$CRD_SCHEMA_LOCATION"
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

assert_not_contains() {
  local output="$1"
  local unexpected="$2"

  if [[ "$output" == *"$unexpected"* ]]; then
    echo "Expected rendered output not to contain: $unexpected" >&2
    exit 1
  fi
}

validate_profile
validate_profile -f "$CHART_DIR/examples/values-gcp.yaml"
validate_profile \
  -f "$CHART_DIR/examples/values-gcp.yaml" \
  -f "$CHART_DIR/examples/values-gcp-standard.yaml"
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
  --set-json 'networkPolicy.ingress=[{"from":[{"namespaceSelector":{"matchLabels":{"kubernetes.io/metadata.name":"callers"}},"podSelector":{"matchExpressions":[{"key":"app.kubernetes.io/name","operator":"In","values":["gateway-client"]}]}}],"ports":[{"protocol":"TCP","port":"http"},{"protocol":"TCP","port":4000}]}]' \
  --set-json 'networkPolicy.egress=[{"to":[{"ipBlock":{"cidr":"10.0.0.0/8","except":["10.1.0.0/16"]}},{"ipBlock":{"cidr":"2001:db8::/32"}},{"ipBlock":{"cidr":"::ffff:192.0.2.128/128"}}],"ports":[{"protocol":"TCP","port":443,"endPort":445}]}]' \
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
assert_contains "$default_output" 'ghcr.io/berriai/litellm-non_root:v1.85.1@sha256:97ce25938fc7f38f14a4036df78ba3d57725706b6183488af9931750e395c673'
assert_contains "$default_output" 'curlimages/curl:8.12.1@sha256:94e9e444bcba979c2ea12e27ae39bee4cd10bc7041a472c4727a558e213744e6'
assert_contains "$default_output" '--use_v2_migration_resolver'
assert_contains "$default_output" '--enforce_prisma_migration_check'
assert_contains "$default_output" 'runAsNonRoot: true'
assert_contains "$default_output" 'runAsUser: 65534'
assert_contains "$default_output" 'runAsGroup: 65534'
assert_contains "$default_output" 'fsGroup: 65534'
assert_contains "$default_output" 'helm.sh/hook-delete-policy: before-hook-creation,hook-succeeded'
assert_not_contains "$default_output" 'hook-failed'

argocd_output="$(helm template litellm "$CHART_DIR" \
  --namespace litellm \
  --set migrationJob.hooks.helm.enabled=false \
  --set migrationJob.hooks.argocd.enabled=true)"
assert_contains "$argocd_output" 'argocd.argoproj.io/hook-delete-policy: BeforeHookCreation,HookSucceeded'
assert_not_contains "$argocd_output" 'HookFailed'

gcp_standard_output="$(helm template litellm "$CHART_DIR" \
  --namespace litellm \
  -f "$CHART_DIR/examples/values-gcp.yaml" \
  -f "$CHART_DIR/examples/values-gcp-standard.yaml")"
assert_contains "$gcp_standard_output" 'iam.gke.io/gke-metadata-server-enabled: "true"'

safe_proxy_config_output="$(helm template litellm "$CHART_DIR" \
  --namespace litellm \
  --set 'litellm.proxy_config.model_list[0].model_name=safe' \
  --set 'litellm.proxy_config.model_list[0].litellm_params.model=openai/gpt-4o' \
  --set-string 'litellm.proxy_config.model_list[0].litellm_params.api_key=os.environ/OPENAI_API_KEY' \
  --set-string 'litellm.proxy_config.model_list[0].litellm_params.input_cost_per_token=0.01')"
assert_contains "$safe_proxy_config_output" 'api_key: os.environ/OPENAI_API_KEY'
assert_contains "$safe_proxy_config_output" 'input_cost_per_token: "0.01"'

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
expect_failure "unknown ServiceMonitor namespace selector" "notARealField" \
  --set serviceMonitor.namespaceSelector.notARealField=true
expect_failure "external ConfigMap" "proxyConfigMap.name is required" \
  --set litellm.proxyConfigMap.create=false
expect_failure "chart-owned ConfigMap key" "litellm.proxyConfigMap.key" \
  --set litellm.proxyConfigMap.key=custom.yaml
expect_failure "unsupported Deployment priority class" "litellm.priorityClassName is unsupported" \
  --set litellm.priorityClassName=high-priority
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
expect_failure "plaintext proxy API key" "litellm.proxy_config.model_list[0].litellm_params.api_key must use os.environ/ENV_VAR" \
  --set 'litellm.proxy_config.model_list[0].model_name=unsafe' \
  --set 'litellm.proxy_config.model_list[0].litellm_params.model=openai/gpt-4o' \
  --set-string 'litellm.proxy_config.model_list[0].litellm_params.api_key=sk-visible'
expect_failure "plaintext nested credential" "litellm.proxy_config.model_list[0].litellm_params.vertex_credentials must use os.environ/ENV_VAR" \
  --set 'litellm.proxy_config.model_list[0].model_name=unsafe' \
  --set 'litellm.proxy_config.model_list[0].litellm_params.model=vertex_ai/gemini' \
  --set-string 'litellm.proxy_config.model_list[0].litellm_params.vertex_credentials={"private_key":"visible"}'
expect_failure "plaintext credential with external ConfigMap" "litellm.proxy_config.model_list[0].litellm_params.api_key must use os.environ/ENV_VAR" \
  --set litellm.proxyConfigMap.create=false \
  --set litellm.proxyConfigMap.name=external-config \
  --set 'litellm.proxy_config.model_list[0].model_name=unused-but-persisted' \
  --set 'litellm.proxy_config.model_list[0].litellm_params.model=openai/gpt-4o' \
  --set-string 'litellm.proxy_config.model_list[0].litellm_params.api_key=sk-still-persisted'
expect_failure "plaintext master key with external ConfigMap" "litellm.proxy_config.general_settings.master_key must use os.environ/ENV_VAR" \
  --set litellm.proxyConfigMap.create=false \
  --set litellm.proxyConfigMap.name=external-config \
  --set-string 'litellm.proxy_config.general_settings.master_key=sk-still-persisted'
expect_failure "plaintext authorization header" "litellm.proxy_config.model_list[0].litellm_params.extra_headers.Authorization must use os.environ/ENV_VAR" \
  --set 'litellm.proxy_config.model_list[0].model_name=unsafe' \
  --set 'litellm.proxy_config.model_list[0].litellm_params.model=openai/gpt-4o' \
  --set-string 'litellm.proxy_config.model_list[0].litellm_params.extra_headers.Authorization=Bearer visible'
expect_failure "malformed environment credential reference" "litellm.proxy_config.model_list[0].litellm_params.api_key must use os.environ/ENV_VAR" \
  --set 'litellm.proxy_config.model_list[0].model_name=unsafe' \
  --set 'litellm.proxy_config.model_list[0].litellm_params.model=openai/gpt-4o' \
  --set-string 'litellm.proxy_config.model_list[0].litellm_params.api_key=os.environ/1INVALID'
expect_failure "non-root pod policy" "litellm.podSecurityContext.runAsNonRoot" \
  --set litellm.podSecurityContext.runAsNonRoot=false
expect_failure "non-root container user" "litellm.securityContext.runAsUser" \
  --set litellm.securityContext.runAsUser=0
expect_failure "writable runtime filesystem" "litellm.securityContext.readOnlyRootFilesystem" \
  --set litellm.securityContext.readOnlyRootFilesystem=true
expect_failure "unknown NetworkPolicy rule field" "notARealNetworkPolicyField" \
  --set networkPolicy.enabled=true \
  --set networkPolicy.migration.managedExternally=true \
  --set 'networkPolicy.ingress[0].notARealNetworkPolicyField=true'
expect_failure "empty NetworkPolicy rule" "networkPolicy.ingress.0" \
  --set networkPolicy.enabled=true \
  --set networkPolicy.migration.managedExternally=true \
  --set-json 'networkPolicy.ingress=[{}]'
expect_failure "empty NetworkPolicy peer" "networkPolicy.ingress.0.from.0" \
  --set networkPolicy.enabled=true \
  --set networkPolicy.migration.managedExternally=true \
  --set-json 'networkPolicy.ingress=[{"from":[{}]}]'
expect_failure "unknown NetworkPolicy peer field" "notARealPeerField" \
  --set networkPolicy.enabled=true \
  --set networkPolicy.migration.managedExternally=true \
  --set-json 'networkPolicy.ingress=[{"from":[{"notARealPeerField":true}]}]'
expect_failure "unknown NetworkPolicy port field" "notARealPortField" \
  --set networkPolicy.enabled=true \
  --set networkPolicy.migration.managedExternally=true \
  --set-json 'networkPolicy.ingress=[{"ports":[{"notARealPortField":true}]}]'
expect_failure "invalid NetworkPolicy selector expression" "networkPolicy.ingress.0.from.0.podSelector.matchExpressions.0.values" \
  --set networkPolicy.enabled=true \
  --set networkPolicy.migration.managedExternally=true \
  --set-json 'networkPolicy.ingress=[{"from":[{"podSelector":{"matchExpressions":[{"key":"app","operator":"In","values":[]}]}}]}]'
expect_failure "mixed NetworkPolicy IP and selector peer" "networkPolicy.egress.0.to.0" \
  --set networkPolicy.enabled=true \
  --set networkPolicy.migration.managedExternally=true \
  --set-json 'networkPolicy.egress=[{"to":[{"ipBlock":{"cidr":"10.0.0.0/8"},"namespaceSelector":{}}]}]'
expect_failure "invalid IPv4 NetworkPolicy CIDR" "networkPolicy.egress.0.to.0.ipBlock.cidr" \
  --set networkPolicy.enabled=true \
  --set networkPolicy.migration.managedExternally=true \
  --set-json 'networkPolicy.egress=[{"to":[{"ipBlock":{"cidr":"999.999.999.999/999"}}]}]'
expect_failure "invalid IPv6 NetworkPolicy CIDR" "networkPolicy.egress.0.to.0.ipBlock.cidr" \
  --set networkPolicy.enabled=true \
  --set networkPolicy.migration.managedExternally=true \
  --set-json 'networkPolicy.egress=[{"to":[{"ipBlock":{"cidr":"2001:db8::1/129"}}]}]'
expect_failure "NetworkPolicy endPort bounds" "endPort cannot be less than port" \
  --set networkPolicy.enabled=true \
  --set networkPolicy.migration.managedExternally=true \
  --set-json 'networkPolicy.egress=[{"ports":[{"protocol":"TCP","port":445,"endPort":443}]}]'
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
  assert_contains "$package_contents" 'examples/values-gcp-standard.yaml'
fi

echo "Validated $CHART_DIR"
