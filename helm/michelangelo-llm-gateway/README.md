# Michelangelo LLM Gateway Helm Chart

This chart installs LiteLLM as Michelangelo's independently operated LLM gateway data plane. It wraps the official `litellm-helm` chart instead of maintaining a second implementation of the LiteLLM Deployment and supporting resources.

The gateway is installed, upgraded, and rolled back separately from the Michelangelo control-plane chart. This keeps inference traffic, database migrations, scaling, and availability outside the control-plane release lifecycle.

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

The prerelease wrapper intentionally does not fork upstream templates. LiteLLM 1.85.1 therefore does not expose `loadBalancerSourceRanges`, `externalTrafficPolicy`, `enableServiceLinks`, or `revisionHistoryLimit`. Apply source restrictions outside the Service, and address the missing Deployment hardening upstream before declaring this chart stable.

## Prerequisites

- Kubernetes 1.27 or newer.
- Helm 3.17 or newer for installation, upgrades, and the filtered wrapper test.
- An existing PostgreSQL database supported by LiteLLM 1.85.1.
- Existing Kubernetes Secrets for the LiteLLM master key, salt key, database URL, and provider credentials.
- At least one approved provider model.

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

Use `valueFrom.secretKeyRef` for provider credentials and other sensitive environment variables. Do not put plaintext credentials in values files: Helm persists supplied values in the release Secret.

The wrapper requires the dependency to create its ServiceAccount with token automount disabled. Add Workload Identity annotations through `litellm.serviceAccount.annotations`; do not replace it with an unmanaged ServiceAccount.

The wrapper deliberately disables the upstream standalone PostgreSQL and Redis dependencies. Production data stores must be provisioned and operated separately.

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

For GCP, copy `examples/values-gcp.yaml` and replace every `REPLACE_WITH_...` placeholder. The example uses a GKE internal LoadBalancer and Workload Identity.

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

Migration hooks run before ordinary release resources. `migrationJob.serviceAccountName` must therefore name an existing ServiceAccount. Helm rollback does not reverse database schema changes; use backward-compatible migrations and maintain a tested database recovery path.

## Metrics

The upstream ServiceMonitor is disabled because it cannot configure authentication. Enable the wrapper monitor with a dedicated LiteLLM metrics credential:

```yaml
serviceMonitor:
  enabled: true
  authorization:
    secretName: litellm-metrics
    secretKey: token
```

Chart-owned LiteLLM configuration must include the `prometheus` success callback when the ServiceMonitor is enabled. Restrict metrics access through network policy and platform routing.

## NetworkPolicy

The optional NetworkPolicy is deny-by-default apart from explicitly configured ingress, egress, DNS, and the wrapper test. Define database, provider, caller, observability, and metadata-server rules before enabling it.

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
> The upstream 1.85.1 chart contains unconditional test hooks with mutable public images, and one assumes Datadog-specific values. Always select the wrapper-owned test with `--filter`; do not run this release's tests unfiltered. This limitation should be removed after upstream provides configurable, disableable tests.

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

The wrapper is versioned independently from both Michelangelo and LiteLLM. A tag such as `michelangelo-llm-gateway-v0.1.0` publishes the self-contained chart to:

```text
oci://ghcr.io/michelangelo-ai/michelangelo/charts/michelangelo-llm-gateway
```
