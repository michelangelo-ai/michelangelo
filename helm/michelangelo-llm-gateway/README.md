# Michelangelo LLM Gateway Helm Chart

This chart installs LiteLLM as Michelangelo's independently operated LLM gateway data plane. It wraps the official `litellm-helm` chart instead of maintaining a second implementation of the LiteLLM Deployment and supporting resources.

The gateway and control-plane charts are published with the same Michelangelo version so operators and integration tests have one compatibility coordinate. The gateway is still installed, upgraded, and rolled back as a separate Helm release, keeping inference traffic, database migrations, scaling, and availability outside the control-plane runtime lifecycle.

## Ownership

The pinned upstream chart owns:

- The LiteLLM Deployment, Service, ServiceAccount, and configuration mount.
- Health probes, Ingress, HPA or KEDA, PodDisruptionBudget, and topology spread.
- Optional environment Secret and ConfigMap injection.

This wrapper owns only behavior that the upstream chart cannot express safely:

- The qualified v2 database migration Job.
- An optional NetworkPolicy.
- An optional authenticated ServiceMonitor.
- Strict validation of the Michelangelo runtime contract.
- A digest-pinned authenticated connection test.

Cloud SQL, Redis, Secret Manager, External Secrets Operator, private DNS, TLS, provider infrastructure, dashboards, and alerts remain external.

The prerelease wrapper intentionally does not fork upstream templates. LiteLLM 1.85.1 therefore does not expose `loadBalancerSourceRanges`, `externalTrafficPolicy`, `enableServiceLinks`, `revisionHistoryLimit`, workload `priorityClassName`, HPA behavior, or safe sidecar injection. Apply source restrictions outside the Service and address required upstream gaps before declaring this chart stable.

## Prerequisites

- Kubernetes 1.27 or newer.
- Helm 3.17 or newer for installation, upgrades, and the filtered wrapper test.
- kubeconform 0.8.0 for strict Kubernetes and operator-CRD validation.
- An existing PostgreSQL database supported by LiteLLM 1.85.1.
- Existing Kubernetes Secrets for the LiteLLM master key, salt key, database URL, and provider credentials.
- At least one approved provider model.
- GKE Workload Identity Federation enabled on the cluster and eligible node pools when using Vertex AI.

## Build the dependency

The source chart locks `litellm-helm` to version 1.85.1 through `Chart.lock`. The validation script pins the downloaded chart content by SHA-256. Authenticate to GHCR, build the dependency, and validate it before installing from a checkout:

```bash
helm registry login ghcr.io
helm dependency build helm/michelangelo-llm-gateway
bash .github/scripts/validate-llm-gateway-chart.sh
```

Published OCI packages already contain the dependency.

## Secret contract

The defaults read three values from the existing `litellm-secrets` Secret:

| Wrapper value | Container variable |
| --- | --- |
| `litellm.masterkeySecretName` / `litellm.masterkeySecretKey` | `PROXY_MASTER_KEY` |
| `litellm.extraEnvVars[name=LITELLM_SALT_KEY]` | `LITELLM_SALT_KEY` |
| `litellm.extraEnvVars[name=DATABASE_URL]` | `DATABASE_URL` |

Use `valueFrom.secretKeyRef` for provider credentials and other sensitive environment variables. Provider credential fields in `litellm.proxy_config` must use `os.environ/ENV_VAR`; the chart rejects plaintext values recursively. Do not put plaintext credentials anywhere in values files: Helm persists supplied values in the release Secret.

The wrapper requires the dependency to create its ServiceAccount with token automount disabled. Add Workload Identity annotations through `litellm.serviceAccount.annotations`; do not replace it with an unmanaged ServiceAccount.

The wrapper deliberately disables the upstream standalone PostgreSQL and Redis dependencies. Production data stores must be provisioned and operated separately.

## Runtime security context

The chart pins LiteLLM's official non-root image and runs both the proxy and migration Job as UID and GID `65534`. The pod and container security contexts are part of the wrapper contract: non-root execution, `RuntimeDefault` seccomp, disabled privilege escalation, and dropped Linux capabilities cannot be overridden.

The image needs writable application cache and migration paths, so `readOnlyRootFilesystem` remains disabled. The chart otherwise satisfies the container-level Restricted Pod Security requirements; cluster policy and any injected platform resources must still be validated in the target environment.

## Configuration

By default, the upstream chart creates a ConfigMap from `litellm.proxy_config`. To use an externally managed ConfigMap:

```yaml
litellm:
  proxyConfigMap:
    create: false
    name: litellm-config
    key: config.yaml
```

Externally managed configuration must preserve these settings:

```yaml
general_settings:
  master_key: os.environ/PROXY_MASTER_KEY
  disable_prisma_schema_update: true
litellm_settings:
  cache: false
```

When the wrapper owns the ConfigMap, chart validation enforces those settings.

## Install

Validate an environment values file before installation:

```bash
helm lint --strict helm/michelangelo-llm-gateway \
  -f path/to/environment-values.yaml

helm template litellm helm/michelangelo-llm-gateway \
  --namespace litellm \
  -f path/to/environment-values.yaml
```

Install the gateway as its own release and namespace:

```bash
helm upgrade --install litellm helm/michelangelo-llm-gateway \
  --namespace litellm \
  --create-namespace \
  --wait \
  --timeout 15m \
  -f path/to/environment-values.yaml
```

For GCP, copy `examples/values-gcp.yaml` and replace every `REPLACE_WITH_...` placeholder. The common example uses a GKE internal LoadBalancer and a Kubernetes ServiceAccount annotated for Workload Identity Federation.

On Standard GKE, enable the GKE metadata server on every eligible node pool and apply the Standard placement overlay as a second values file:

```bash
helm upgrade --install litellm helm/michelangelo-llm-gateway \
  --namespace litellm \
  --create-namespace \
  -f helm/michelangelo-llm-gateway/examples/values-gcp.yaml \
  -f helm/michelangelo-llm-gateway/examples/values-gcp-standard.yaml
```

Grant the annotated Google Service Account `roles/iam.workloadIdentityUser` to `serviceAccount:PROJECT_ID.svc.id.goog[litellm/litellm]`, then grant that Google Service Account only the required Vertex AI permissions. Autopilot already enables the metadata server on every node and must not use the Standard-only node selector.

## Database migrations

The upstream migration Job is disabled. The wrapper runs the pinned LiteLLM image with the v2 resolver and fails the release when migration verification fails.

The default is a Helm `pre-install,pre-upgrade` hook. For Argo CD:

```yaml
migrationJob:
  hooks:
    helm:
      enabled: false
      weight: 0
    argocd:
      enabled: true
```

For an external migration controller:

```yaml
migrationJob:
  enabled: false
  managedExternally: true
  hooks:
    helm:
      enabled: false
      weight: 0
    argocd:
      enabled: false
```

Migration hooks run before ordinary release resources. `migrationJob.serviceAccountName` must therefore name an existing ServiceAccount. Failed migration Jobs are retained for diagnosis until their 24-hour TTL expires or the next hook attempt replaces them. Helm rollback does not reverse database schema changes; use backward-compatible migrations and maintain a tested database recovery path.

## Metrics

The upstream ServiceMonitor is disabled because it cannot configure authentication. The wrapper ServiceMonitor references an existing Secret in the release namespace; it does not create or authorize the credential.

After LiteLLM and its database are available, create a dedicated virtual key through the Management API using the master key:

```bash
curl --fail --show-error --silent \
  --request POST "${LITELLM_URL}/key/generate" \
  --header "Authorization: Bearer ${PROXY_MASTER_KEY}" \
  --header 'Content-Type: application/json' \
  --data '{"key_alias":"prometheus-scrape","allowed_routes":["/metrics"]}'
```

Store the returned `key` through the approved Secret Manager and External Secrets workflow as `litellm-metrics/token`, then enable the monitor:

```yaml
serviceMonitor:
  enabled: true
  authorization:
    secretName: litellm-metrics
    secretKey: token
```

Chart-owned LiteLLM configuration must include the `prometheus` success callback when the ServiceMonitor is enabled. An arbitrary bearer token is not accepted. Rotate by provisioning a new LiteLLM key, updating the Secret, confirming a successful scrape, and then deleting the old key. The master key is a bootstrap-only fallback. Restrict metrics access through network policy and platform routing.

## NetworkPolicy

The optional NetworkPolicy is deny-by-default apart from explicitly configured ingress, egress, DNS, and the wrapper test. Rules are structurally validated and rendered literally; template expressions and empty allow-all rules are rejected. Define database, provider, caller, observability, and metadata-server rules before enabling it.

Migration hooks run before this NetworkPolicy exists. Provision a matching migration policy externally, then acknowledge it with:

```yaml
networkPolicy:
  enabled: true
  migration:
    managedExternally: true
```

## Test the release with Helm 3.17+

The wrapper test authenticates to `/v1/models`, supports private-registry pull secrets, and pins its image by digest:

```bash
helm test litellm \
  --namespace litellm \
  --filter name=litellm-michelangelo-llm-gateway-test-connection \
  --logs
```

> [!WARNING]
> The upstream 1.85.1 chart contains unconditional test hooks with mutable public images, and one assumes Datadog-specific values. Unfiltered tests are unsupported. Always select the wrapper-owned test with `--filter`; it is digest-pinned and inherits `litellm.imagePullSecrets`. Failed wrapper tests remain available until the next test run so `--logs` and cluster events remain useful. This limitation can only be removed after upstream provides configurable, disableable tests or the wrapper deliberately owns a patched dependency.

## Qualification status

CI validates schemas, rendering, packaging, the pinned dependency, and image tag-to-digest and platform metadata. It does not replace live qualification. Before production, record successful GKE, Cloud SQL, External Secrets, Workload Identity, provider traffic, budget enforcement, metrics, telemetry, migration, upgrade, rollback, disruption, and uninstall tests for the exact chart and values release.

## Upgrade and uninstall

For every LiteLLM upgrade:

1. Pin the upstream chart, application version, image tag, and digest together.
2. Review release notes and database migrations.
3. Render every supported environment profile.
4. Test migrations, readiness, streaming cancellation, budgets, disruption, and rollback in staging.
5. Confirm logs, traces, spend records, and callbacks exclude prompts, completions, authorization headers, and credentials.

Uninstalling the release removes chart-managed Kubernetes resources. It does not remove external databases, Secrets, ConfigMaps, DNS records, provider infrastructure, or reverse database migrations.

```bash
helm uninstall litellm --namespace litellm --wait
```

## Release

The wrapper is published alongside the Michelangelo control-plane chart from the same `vX.Y.Z` release tag. Both packages use `X.Y.Z` as their chart version, while this chart's `appVersion` identifies the pinned LiteLLM runtime. Shared artifact versioning does not combine their Helm releases or runtime lifecycles.

Future integration tests should install both OCI charts at the same version, then exercise independent gateway and control-plane upgrades explicitly. The self-contained gateway chart is published to:

```text
oci://ghcr.io/michelangelo-ai/michelangelo/charts/michelangelo-llm-gateway
```
