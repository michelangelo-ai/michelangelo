# Authentication & Identity

This guide is for platform operators and cluster administrators configuring authentication for a Michelangelo AI deployment.

**Prerequisites**: A running Michelangelo AI control plane (see [Platform Setup](./platform-setup.md)) and `kubectl` access to the `ma-system` namespace.

Authentication in Michelangelo AI operates at two levels:

- **User authentication** — end users present OIDC ID tokens (issued by your identity provider) as bearer tokens on API requests
- **Service authentication** — internal callers (the worker) present Kubernetes ServiceAccount tokens, which the API server validates through the TokenReview API

Authorization for both is delegated to Kubernetes RBAC: every request becomes a `SubjectAccessReview`, so access is granted with ordinary `RoleBinding`s on the Michelangelo resources.

## Auth modes

The API server has two modes, selected by `apiserver.auth.mode` in the Helm values:

- `dummy` (default) — every request is allowed. This is the shipped behavior; nothing changes for existing deployments.
- `k8s-rbac` — bearer tokens are required and verified, and each request is authorized through Kubernetes RBAC.

**Enable enforcement only after your clients can send tokens.** The Web UI does not acquire OIDC tokens on its own today: unless you front Envoy with an auth proxy (for example `oauth2-proxy`) that injects an `Authorization: Bearer <id-token>` header, enabling `k8s-rbac` will lock the UI out — including reads. gRPC/CLI clients must likewise attach the header to every call.

## Enabling k8s-rbac mode

Configure everything through Helm values and run `helm upgrade`; the API server picks up the new ConfigMap automatically (its Deployment is annotated with the config checksum).

```yaml
apiserver:
  auth:
    mode: k8s-rbac
    oidc:
      issuerUrl: https://accounts.your-idp.com
      audiences: ["michelangelo"]   # accepted values of the token's aud claim (any-of)
      usernameClaim: email          # JWT claim used as the RBAC username
      groupsClaim: groups           # JWT claim used for group-based RBAC
    serviceAccounts:
      enabled: true                 # accept ServiceAccount tokens (see below)
      audiences: ["michelangelo"]
    sarCache:
      allowTTL: 10s                 # authorization decision cache; 0s disables
      denyTTL: 10s
```

Notes:

- The API server fetches `<issuerUrl>/.well-known/openid-configuration` at startup and **fails fast if the issuer is unreachable or misconfigured** — check the apiserver logs if the pod does not become ready after enabling OIDC.
- Tokens are verified on every request: signature (against the issuer's JWKS), issuer, audience, and expiry. There is no separate session to configure; session lifetime is your IdP's token lifetime.
- Once enforcement is on, users without a `RoleBinding` are denied access to all resources.
- Authorization decisions are cached for `sarCache.allowTTL` / `denyTTL` (default 10s each), so a revoked `RoleBinding` can be honored for up to that window. Set a TTL to `0s` to disable that cache at the cost of one `SubjectAccessReview` per request.
- `oidc.clockSkewLeeway` (default 30s) bounds the accepted clock skew when validating token expiry and not-before claims.

## Connecting an Identity Provider (OIDC)

Michelangelo AI accepts ID tokens from any OIDC-compliant identity provider. How users *obtain* tokens is up to your front door (an auth proxy in front of Envoy, or your CLI's login tooling); the API server only validates what arrives in the `Authorization` header. Configure the issuer and accepted audiences:

### Okta

```yaml
oidc:
  issuerUrl: https://your-org.okta.com
  audiences: ["<client-id-from-okta>"]
```

### Google Workspace

```yaml
oidc:
  issuerUrl: https://accounts.google.com
  audiences: ["<client-id>.apps.googleusercontent.com"]
  usernameClaim: email
  groupsClaim: hd    # Google Workspace hosted domain
```

### Azure Active Directory

```yaml
oidc:
  issuerUrl: https://login.microsoftonline.com/<tenant-id>/v2.0
  audiences: ["<application-client-id>"]
  usernameClaim: upn        # User Principal Name (email format)
  groupsClaim: groups
```

### Keycloak

```yaml
oidc:
  issuerUrl: https://keycloak.your-domain.com/realms/<realm-name>
  audiences: ["michelangelo"]
```

## Multi-Factor Authentication

MFA is enforced at the IdP level, not within Michelangelo AI. Configure MFA policies in your identity provider's admin console; users must complete the full IdP flow — including MFA — to obtain the ID token they present to Michelangelo AI.

## Granting Access with RBAC

In `k8s-rbac` mode the chart installs two ClusterRoles covering the Michelangelo API resources:

- `michelangelo-viewer` — `get` and `list`
- `michelangelo-editor` — `get`, `list`, `create`, `update`, `delete`, `deletecollection`

Bind them per project namespace with a `RoleBinding`, or cluster-wide with a `ClusterRoleBinding`. The subject names must match what the token's claims produce: `User` names match the `usernameClaim` value, `Group` names match entries in the `groupsClaim` value.

### Grant a user read access to a project namespace

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: alice-reader
  namespace: ml-team-project
subjects:
- kind: User
  name: alice@your-company.com   # Must match the value of usernameClaim in the JWT
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: michelangelo-viewer
  apiGroup: rbac.authorization.k8s.io
```

### Grant a team edit access via group membership

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: ml-team-editors
  namespace: ml-team-project
subjects:
- kind: Group
  name: ml-team                  # Must match a value of groupsClaim in the JWT
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: michelangelo-editor
  apiGroup: rbac.authorization.k8s.io
```

Use `RoleBinding` to scope access to a specific namespace. Use `ClusterRoleBinding` only for platform administrators who need cross-namespace access — a `List` request without a namespace is authorized as a cluster-wide list and requires one.

## Multi-Tenant Namespace Isolation

Each team or project should have its own Kubernetes namespace. Use `NetworkPolicy` resources to prevent cross-namespace access to ML workloads:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-cross-namespace
  namespace: ml-team-a
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: ml-team-a
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: ma-system   # Control plane needs access
```

This allows traffic within the team's namespace and from the Michelangelo AI control plane, but blocks all other namespaces.

## Service Authentication (Internal)

**Worker → API server**: the worker's pipeline activities run as a **per-project runner identity**, not as a single platform-wide worker identity. For an activity targeting project namespace `ns`, the worker requests a short-lived token for the `michelangelo-runner` ServiceAccount in `ns` (via the Kubernetes `TokenRequest` API) and attaches it as the bearer token. The API server validates it with a `TokenReview` — the chart binds the platform ServiceAccount to the built-in `system:auth-delegator` ClusterRole for this — and authorizes it as `system:serviceaccount:<ns>:michelangelo-runner` against that namespace's `RoleBinding` only. A caller who can trigger a run in project A therefore gets at most the runner's rights in A.

To enable it:

1. List your project namespaces in `apiserver.auth.runnerProjects`; the chart creates the `michelangelo-runner` ServiceAccount and a `michelangelo-editor` RoleBinding in each.
2. Set `worker.runnerToken.enabled: true`; the chart also grants the worker its one narrow cluster-wide permission — `create` on `serviceaccounts/token`, restricted with `resourceNames: ["michelangelo-runner"]`.
3. Keep `apiserver.auth.serviceAccounts.enabled: true`, and if you set `serviceAccounts.audiences`, list one of them in `worker.runnerToken.audiences`.

With `worker.runnerToken.enabled: false` (the default) the worker sends no credential, which only works in `dummy` mode. A project namespace missing its runner ServiceAccount or RoleBinding fails pipelines in that project only, with an activity error naming the missing object.

`worker.useTLS` is transport encryption for the same connection — orthogonal to authentication. Do not set `useTLS: false` in production.

**Controller manager → compute cluster**: Uses the `ray-manager` service account token stored as a Secret in the control plane namespace. See [Register a Compute Cluster](register-a-compute-cluster-to-michelangelo-control-plane.md) for the full setup including token rotation guidance.

## Disabling Direct Storage Access

Do not allow users or services to directly access etcd or object storage (S3/MinIO) in ways that bypass the Michelangelo AI API. For S3 access:

- Set `useIam: true` in the controller manager ConfigMap — this uses IAM roles attached to pods via ServiceAccount annotations, not hardcoded credentials
- Do not grant `s3:*` to individual users; use IAM policies scoped to specific buckets and prefixes
- Audit S3 bucket policies regularly to ensure no public or cross-account access is inadvertently granted

## What's Next

- **Network configuration**: Set up Ingress, TLS, and Envoy CORS rules in the [Network & Ingress guide](./network.md)
- **Compliance**: Configure audit logging and data-residency controls for SOC 2, GDPR, or HIPAA in the [Compliance guide](../operations/compliance.md)
- **Monitoring**: Set up Prometheus scraping and alerting for the control plane in the [Monitoring guide](../operations/monitoring.md)
