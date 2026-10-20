---
last_verified: 2026-10-14
---

# Authentication & Identity

This guide is for platform operators and cluster administrators configuring authentication for a Michelangelo AI deployment.

**Prerequisites**: A running Michelangelo AI control plane (see [Platform Setup](./platform-setup.md)) and `kubectl` access to the namespace the Helm chart is installed in.

## What ships by default

The stock Michelangelo AI API server (`go/cmd/apiserver`) does **not** authenticate or authorize callers out of the box. It wires `auth.DummyAuthModule`, which allows every request, and it installs no authentication middleware unless a deployment supplies one. There is no `apiserver.auth.*` Helm value, ConfigMap key, or `rbacEnabled` / `sessionTokenExpiry` setting in the chart.

Until you wire in authentication, protect the API surface at the network layer: keep the API server, Envoy, and UI off the public internet, restrict access with Ingress and NetworkPolicy rules (see [Network & Ingress](./network.md)), and put an authenticating proxy or ingress in front of the UI if you expose it.

Authentication in Michelangelo AI is built from two independent, pluggable layers in the `go/auth` package:

1. **Transport authentication** (`go/auth/authn.go`). `auth.NewInboundMiddleware` is a YARPC inbound middleware that requires an `authorization: Bearer <token>` header, verifies it with a deployment-specific `Authenticator`, optionally restricts callers to an allowlist of service names (`AllowedServices`), and stores the verified `Principal` on the request context. It is installed only when an `Authenticator` is provided. For outbound calls, `auth.NewBearerOutboundMiddleware` attaches a token from a `TokenSource`.
2. **Handler authorization**. The generated API handlers call the `auth.Auth` interface (`UserAuthenticated`, `UserAuthorized`). `DummyAuth` allows everything and is the default. `auth.NewK8sRBACAuth` adapts a `k8s.io/apiserver` authorizer to this interface so decisions can be made with Kubernetes RBAC.

Both the API server (`DispatcherParams.InboundMiddleware`) and the worker (`Params.OutboundMiddleware`) accept these middlewares as optional `fx` dependencies, so enabling them means building a custom API server and worker binary (or image) that provides them, in the same way as described in [Notification Delivery Setup](../notifications.md#step-2-implement-delivery).

## OIDC token verification

`go/auth/oidc` provides an OIDC ID-token verifier (`oidc.New`) that returns a `k8s.io/apiserver` token authenticator. `auth.FromTokenAuthenticator` adapts it for `NewInboundMiddleware`. The verifier takes this configuration (YAML keys shown; you load and pass the config from your own wiring):

| Key | Default | Meaning |
|-----|---------|---------|
| `issuerUrl` | required | OIDC issuer. Must be `https://` (plain `http` only for loopback hosts). Discovery is fetched from `/.well-known/openid-configuration` and the token's `iss` must match exactly |
| `caBundleFile` | none | PEM file of extra CA certificates trusted for the issuer (private IdP behind an internal CA) |
| `audiences` | none | Accepted values of the token's `aud` claim |
| `usernameClaim` | `email` | Claim used as the caller's username. With `email`, `email_verified` must be true unless `allowUnverifiedEmail` is set |
| `allowUnverifiedEmail` | `false` | Opt out of the `email_verified` requirement |
| `usernamePrefix` | none | Prefix added to the username. When unset, usernames starting with `system:` are rejected |
| `groupsClaim` | `groups` | Claim carrying group memberships; a missing claim yields no groups |
| `groupsPrefix` | none | Prefix added to every group. When unset, groups starting with `system:` are rejected |
| `supportedSigningAlgs` | `RS256`, `ES256` | Allowed JWT signing algorithms |
| `clockSkewLeeway` | `30s` | Leeway for `exp` / `nbf`; must not be negative or exceed 5 minutes |

```yaml
issuerUrl: https://accounts.your-idp.com
audiences:
  - michelangelo
usernameClaim: email
groupsClaim: groups
```

Token lifetime and refresh are governed by your identity provider, not by Michelangelo AI.

## Multi-Factor Authentication

MFA is enforced at the IdP level, not within Michelangelo AI. Configure MFA policies in your identity provider's admin console; tokens that reach Michelangelo AI have already completed the IdP flow.

## Granting Access with RBAC

When authorization is backed by Kubernetes RBAC (`auth.NewK8sRBACAuth`), or when users access Michelangelo AI resources directly with `kubectl`, users need a `RoleBinding` or `ClusterRoleBinding` on the `michelangelo.api` resources. Define the Roles yourself, for example (see also [Model Registry](../components/model-registry.md#step-3-configure-rbac)):

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: michelangelo-viewer
  namespace: ml-team-project
rules:
  - apiGroups: ["michelangelo.api"]
    resources: ["*"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: alice-reader
  namespace: ml-team-project
subjects:
- kind: User
  name: alice@your-company.com   # Must match the username produced by your authenticator
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: michelangelo-viewer
  apiGroup: rbac.authorization.k8s.io
```

For group-based access, use `kind: Group` with a name matching a value in the groups claim (including any `groupsPrefix`). Use `RoleBinding` to scope access to a specific namespace. Use `ClusterRoleBinding` only for platform administrators who need cross-namespace access.

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
          kubernetes.io/metadata.name: <release-namespace>   # Control plane needs access
```

This allows traffic within the team's namespace and from the Michelangelo AI control plane, but blocks all other namespaces.

## Service Authentication (Internal)

**Worker to API server**: The chart points the worker at the in-cluster API server Service (`worker.address`, port `apiserver.port`). The worker's Go config also accepts `worker.useTLS`, which makes the worker dial the API server over TLS using the system trust store and the host in `address`. The chart does not render `useTLS`, so enabling it requires supplying your own worker configuration. Pod-level TLS on the API server is controlled by `apiserver.tls.enabled` and `apiserver.tls.secretName`.

```yaml
worker:
  address: michelangelo-apiserver.<release-namespace>.svc.cluster.local:15566
  maApiServiceName: ma-apiserver
  useTLS: true   # only if the API server is reachable over TLS at this address
```

To attach a bearer token to the worker's calls, inject an `auth.NewBearerOutboundMiddleware` built from your own `TokenSource` into the worker.

**Controller manager to compute cluster**: Uses the `ray-manager` service account token stored as a Secret in the control plane namespace. See [Register a Compute Cluster](register-a-compute-cluster-to-michelangelo-control-plane.md) for the full setup including token rotation guidance.

## Securing Storage Access

Do not allow users or services to directly access etcd or object storage (S3/MinIO) in ways that bypass the Michelangelo AI API. For S3 access:

- Supply object storage credentials through a Kubernetes Secret (`objectStorage.existingSecret`) rather than values files; see [Platform Setup](./platform-setup.md#object-store-configuration)
- Do not grant `s3:*` to individual users; use IAM policies scoped to specific buckets and prefixes
- Audit S3 bucket policies regularly to ensure no public or cross-account access is inadvertently granted

## What's Next

- **Network configuration**: Set up Ingress, TLS, and Envoy CORS rules in the [Network & Ingress guide](./network.md)
- **Compliance**: Configure audit logging and data-residency controls for SOC 2, GDPR, or HIPAA in the [Compliance guide](../operations/compliance.md)
- **Monitoring**: Set up Prometheus scraping and alerting for the control plane in the [Monitoring guide](../operations/monitoring.md)
