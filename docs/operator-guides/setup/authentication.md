---
last_verified: 2026-10-15
sidebar_position: 2
---

# Authentication & Identity

This guide is for platform operators and cluster administrators configuring authentication for a Michelangelo AI deployment.

**Prerequisites**: A running Michelangelo AI control plane (see [Platform Setup](./platform-setup.md)) and `kubectl` access to the namespace the Helm chart is installed in.

## What ships by default

The stock Michelangelo AI API server (`go/cmd/apiserver`) wires `auth.AuthModule`, which reads the `apiserver.auth` config key and picks an implementation by `mode`. The default (`mode` unset or `dummy`) allows every request and installs no authentication middleware, so out of the box the API server does **not** authenticate or authorize callers. Setting `apiserver.auth.mode: k8s-rbac` (a Helm value; see [Enabling k8s-rbac mode](#enabling-k8s-rbac-mode)) turns on:

- **User authentication** — end users present OIDC ID tokens (issued by your identity provider) as bearer tokens on API requests
- **Service authentication** — internal callers (the worker) present Kubernetes ServiceAccount tokens, which the API server validates through the TokenReview API

Authorization for both is delegated to Kubernetes RBAC: every request becomes a `SubjectAccessReview`, so access is granted with ordinary `RoleBinding`s on the Michelangelo resources.

Until you enable it, protect the API surface at the network layer: keep the API server, Envoy, and UI off the public internet, restrict access with Ingress and NetworkPolicy rules (see [Network & Ingress](./network.md)), and put an authenticating proxy or ingress in front of the UI if you expose it.

Under the hood, this is built from two independent, pluggable layers in the `go/auth` package:

1. **Transport authentication** (`go/auth/authn.go`). `auth.NewInboundMiddleware` is a YARPC inbound middleware that requires an `authorization: Bearer <token>` header, verifies it with an `Authenticator` (the OIDC verifier below, and/or ServiceAccount TokenReview), optionally restricts callers to an allowlist of service names, and stores the verified `Principal` on the request context. For outbound calls, `auth.NewBearerOutboundMiddleware` attaches a token from a `TokenSource`.
2. **Handler authorization**. The generated API handlers call the `auth.Auth` interface (`UserAuthenticated`, `UserAuthorized`). `auth.NewK8sRBACAuth` adapts a `k8s.io/apiserver` authorizer to this interface so decisions are made with Kubernetes RBAC.

The built-in path needs **configuration only, no rebuild**: in `k8s-rbac` mode the stock API server assembles both layers itself (OIDC and/or ServiceAccount token authentication, RBAC authorization), and the stock worker attaches per-project tokens when `worker.runnerToken` is enabled (see [Service Authentication](#service-authentication-internal)). A custom binary (or image) is needed only for a different `Authenticator` or `TokenSource`.

**Custom Authenticator or TokenSource (advanced)**: The API server (`DispatcherParams.InboundMiddleware`) and the worker (`Params.OutboundMiddleware`) take these middlewares as optional `fx` dependencies, wired in the same way as described in [Notification Delivery Setup](../notifications.md#step-2-implement-delivery). `auth.AuthModule` provides the API server's slot itself, so a custom inbound middleware must replace that provider rather than add a second one; `fx` rejects two providers of the same type. The worker is different: its slot is filled by the opt-in `RunnerTokenModule` (included by the stock `go/cmd/worker`), not by the worker module. A deployment that supplies its own outbound middleware leaves `RunnerTokenModule` out and keeps `worker.runnerToken.enabled: false`; see [Service Authentication](#service-authentication-internal).

## Auth modes

The API server has two modes, selected by `apiserver.auth.mode` in the Helm values:

- `dummy` (default) — every request is allowed; neither layer above is installed. This is the shipped behavior; nothing changes for existing deployments.
- `k8s-rbac` — both layers are wired in: bearer tokens are required and verified, and each request is authorized through Kubernetes RBAC.

**Enable enforcement only after your clients can send tokens.** The Web UI does not acquire OIDC tokens on its own today: unless you front Envoy with an auth proxy (for example `oauth2-proxy`) that injects an `Authorization: Bearer <id-token>` header, enabling `k8s-rbac` will lock the UI out — including reads. gRPC/CLI clients must likewise attach the header to every call.

**Transport security.** Bearer tokens are sent as plain `Authorization` headers with no additional encryption of their own, so `k8s-rbac` mode must only run behind TLS: terminate TLS at (or in front of) Envoy so the token never travels over an unencrypted connection. Do not enable `k8s-rbac` on a deployment that serves plain HTTP.

**Rolling back.** Set `apiserver.auth.mode: dummy` **and** `worker.runnerToken.enabled: false`, then run `helm upgrade`: the chart refuses to render with runner tokens enabled outside `k8s-rbac` mode. The API server returns to the permissive default on its next rollout.

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
- Tokens are verified on every request: signature (against the issuer's JWKS, restricted to `oidc.supportedSigningAlgs` -- RS256 and ES256 by default), issuer, audience, and expiry. There is no separate session to configure; session lifetime is your IdP's token lifetime.
- Once enforcement is on, users without a `RoleBinding` are denied access to all resources.
- Authorization decisions are cached for `sarCache.allowTTL` / `denyTTL` (default 10s each), so a revoked `RoleBinding` can be honored for up to that window. Set a TTL to `0s` to disable that cache at the cost of one `SubjectAccessReview` per request.
- `oidc.clockSkewLeeway` (default 30s, maximum 5m) bounds the accepted clock skew when validating token expiry and not-before claims.
- `issuerUrl` must be `https://`; plain `http://` is only accepted for a loopback address, which is only ever useful for local testing against a throwaway issuer.

Also in `k8s-rbac` mode:

- At least one of `oidc.issuerUrl` and `serviceAccounts.enabled: true` must be set (the chart's `validations.yaml` and the API server both enforce it).
- Verified tokens are cached for 10 seconds. This isn't configurable, so a revoked token can keep working for up to 10 seconds.
- ServiceAccount callers authenticate as `system:serviceaccount:<namespace>:<name>`.

**Order of operations.** Switching to `k8s-rbac` rejects every caller that doesn't present a valid bearer token, including the worker. Before you set `mode`:

1. List your project namespaces in `apiserver.auth.runnerProjects` (the chart creates the `michelangelo-runner` ServiceAccount and RoleBinding in each).
2. Create RoleBindings for your users (see [Granting Access with RBAC](#granting-access-with-rbac)).
3. Enable `worker.runnerToken` (see [Service Authentication](#service-authentication-internal)).
4. Set `mode: k8s-rbac`.

Try the change in a non-production cluster first.

### OIDC configuration reference

`go/auth/oidc` provides an OIDC ID-token verifier (`oidc.New`) that returns a `k8s.io/apiserver` token authenticator. `auth.FromTokenAuthenticator` adapts it for `NewInboundMiddleware`. The verifier takes this configuration. In `k8s-rbac` mode, set these keys under `apiserver.auth.oidc` in the Helm values (see [Enabling k8s-rbac mode](#enabling-k8s-rbac-mode)); a custom wiring can also load them and call `oidc.New` directly:

| Key | Default | Meaning |
|-----|---------|---------|
| `issuerUrl` | required | OIDC issuer. Must be `https://` (plain `http` only for loopback hosts). Discovery is fetched from `/.well-known/openid-configuration` and the token's `iss` must match exactly |
| `caBundleFile` | none | PEM file of extra CA certificates trusted for the issuer (private IdP behind an internal CA) — see below |
| `audiences` | none | Accepted values of the token's `aud` claim |
| `usernameClaim` | `email` | Claim used as the caller's username. With `email`, `email_verified` must be true unless `allowUnverifiedEmail` is set |
| `allowUnverifiedEmail` | `false` | Opt out of the `email_verified` requirement |
| `usernamePrefix` | none | Prefix added to the username. When unset, usernames starting with `system:` are rejected |
| `groupsClaim` | `groups` | Claim carrying group memberships; a missing claim yields no groups |
| `groupsPrefix` | none | Prefix added to every group. When unset, groups starting with `system:` are rejected |
| `supportedSigningAlgs` | `RS256`, `ES256` | Allowed JWT signing algorithms |
| `clockSkewLeeway` | `30s` | Leeway for `exp` / `nbf`; must not be negative or exceed 5 minutes |

### Choosing a usernameClaim: `email` vs `sub`

The default, `email`, is convenient because it's human-readable in `RoleBinding` subjects and audit logs, but it's only as trustworthy as the IdP's `email_verified` claim -- so the apiserver requires that claim to be `true` whenever `usernameClaim` is `email`, and denies the request otherwise. If your IdP doesn't reliably set `email_verified` (some SSO/SAML-to-OIDC bridges don't), set `oidc.allowUnverifiedEmail: true` to opt out -- but only after confirming your IdP cannot be tricked into issuing a token for an email address the holder doesn't control.

Set `usernameClaim: sub` instead for an identity that's opaque (usually a GUID or similar, not human-readable) but stable and IdP-assigned regardless of email verification -- a reasonable default if your audit tooling can resolve `sub` values back to people some other way, or if your IdP's email verification story is unclear.

Either way, pick a claim that is both unique per person and verified by the IdP (not user-editable) -- its value is used directly as the RBAC username in every `SubjectAccessReview` and `RoleBinding` subject, so a claim two people could share, or one a user could change, undermines per-user authorization.

### Preventing claim collisions with Kubernetes built-in identities

Kubernetes reserves the `system:` prefix for its own built-in identities (`system:masters`, `system:serviceaccount:...`, etc.). If your IdP could ever issue a username or group claim value starting with `system:` -- unlikely with `email`, more plausible with `sub` or a custom claim -- the apiserver rejects that token outright rather than letting it collide with a built-in identity in an RBAC decision. Set `oidc.usernamePrefix` / `oidc.groupsPrefix` (for example `"oidc:"`) to avoid ever hitting that rejection: every username/group is prefixed before it reaches RBAC, so your `RoleBinding` subjects need the same prefix (e.g. `oidc:alice@example.com`).

### Connecting to a private IdP (internal CA)

The apiserver's OIDC discovery and JWKS fetches use the system root CA pool. A private IdP behind an internal CA (common for self-hosted Dex or Keycloak) needs that CA trusted explicitly -- today that means setting `CABundleFile` in the apiserver's auth config directly (a PEM file path), since the chart does not yet have a values-driven way to mount a custom CA bundle into the apiserver pod; this is tracked as a follow-up to this chart's support for OIDC. Until then, `oidc.issuerUrl` must be reachable via a CA the system pool already trusts (e.g. a public IdP, or an internal one issued by your organization's CA if it's in the base image's trust store).

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
```

Google ID tokens carry no group membership claim. Do not set `groupsClaim` to `hd` (the hosted-domain string): it is the same value for every account in the domain, so a `Group` RoleBinding keyed on it would match the entire workspace, not a specific team. Grant access with per-user `RoleBinding` subjects instead (see [Granting Access with RBAC](#granting-access-with-rbac)), or manage groups outside Google Workspace (for example Okta or Azure AD) if group-based RBAC is required.

### Azure Active Directory

```yaml
oidc:
  issuerUrl: https://login.microsoftonline.com/<tenant-id>/v2.0
  audiences: ["<application-client-id>"]
  usernameClaim: preferred_username   # or email, if your tenant issues it
  groupsClaim: groups
```

`upn` is not included in Azure AD v2.0 ID tokens by default; `preferred_username` or `email` is reliably present instead. Also note that `groups` yields the members' object IDs (GUIDs), not display names — `Group` RoleBinding subjects must match those IDs exactly.

### Keycloak

```yaml
oidc:
  issuerUrl: https://keycloak.your-domain.com/realms/<realm-name>
  audiences: ["michelangelo"]
```

Keycloak is commonly self-hosted behind an internal CA; see [Connecting to a private IdP](#connecting-to-a-private-idp-internal-ca) above if so. If you've set `usernameClaim: sub` (Keycloak's `sub` is a stable GUID per user, unaffected by email verification), consider also setting `usernamePrefix` so the resulting RBAC username stays unambiguous.

## Multi-Factor Authentication

MFA is enforced at the IdP level, not within Michelangelo AI. Configure MFA policies in your identity provider's admin console; users must complete the full IdP flow — including MFA — to obtain the ID token they present to Michelangelo AI.

## Granting Access with RBAC

In `k8s-rbac` mode the chart installs two ClusterRoles covering the Michelangelo API resources:

- `michelangelo-viewer` — `get` and `list`
- `michelangelo-editor` — `get`, `list`, `create`, `update`, `delete`, `deletecollection`

Bind them per project namespace with a `RoleBinding`, or cluster-wide with a `ClusterRoleBinding`. The subject names must match what the token's claims produce: `User` names match the `usernameClaim` value, `Group` names match entries in the `groupsClaim` value (including any `groupsPrefix`).

Users who access Michelangelo AI resources directly with `kubectl` need the same bindings (see also [Model Registry](../components/model-registry.md#step-3-configure-rbac)).

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
          kubernetes.io/metadata.name: <release-namespace>   # Control plane needs access
```

This allows traffic within the team's namespace and from the Michelangelo AI control plane, but blocks all other namespaces.

## Service Authentication (Internal)

**Worker → API server**: the worker's pipeline activities run as a **per-project runner identity**, not as a single platform-wide worker identity. For an activity targeting project namespace `ns`, the worker requests a short-lived token for the `michelangelo-runner` ServiceAccount in `ns` (via the Kubernetes `TokenRequest` API) and attaches it as the bearer token. The API server validates it with a `TokenReview` — the chart binds the platform ServiceAccount to the built-in `system:auth-delegator` ClusterRole for this — and authorizes it as `system:serviceaccount:<ns>:michelangelo-runner` against that namespace's `RoleBinding` only. A caller who can trigger a run in project A therefore gets at most the runner's rights in A.

To enable it:

1. List your project namespaces in `apiserver.auth.runnerProjects`; the chart creates the `michelangelo-runner` ServiceAccount and a `michelangelo-editor` RoleBinding in each.
2. Set `worker.runnerToken.enabled: true`; the chart also grants the worker permission to mint runner tokens: `create` on `serviceaccounts/token`, restricted with `resourceNames: ["michelangelo-runner"]` and bound only in each `runnerProjects` namespace (not cluster-wide). With this on, the stock worker wires `auth.NewBearerOutboundMiddleware` itself (through `RunnerTokenModule`; see [Custom worker binaries](#service-authentication-internal) below) -- there's nothing further to inject.
3. Keep `apiserver.auth.serviceAccounts.enabled: true`, and if you set `serviceAccounts.audiences`, list one of them in `worker.runnerToken.audiences`.

With `worker.runnerToken.enabled: false` (the default) the worker sends no credential, which only works in `dummy` mode. A project namespace missing its runner ServiceAccount or RoleBinding fails pipelines in that project only, with an activity error naming the missing object.

This is a separate concern from transport encryption: `worker.useTLS` makes the worker dial the API server over TLS using the system trust store (pair it with `apiserver.tls.enabled` / `apiserver.tls.secretName` on the API server side). Do not set `useTLS: false` in production, independent of whether `k8s-rbac` mode is on.

The worker caches each minted token until one minute before it expires, and a call that carries no project namespace fails rather than going out unauthenticated. With `k8s-rbac` and `runnerToken` disabled (and no custom `TokenSource`), every worker call is rejected, so pipeline runs and triggers fail.

The worker's configuration, which the chart renders from the `worker.runnerToken.*` Helm values:

```yaml
worker:
  runnerToken:
    enabled: true     # default false: worker traffic is unchanged
    audiences:        # optional; the API server's serviceAccounts.audiences must accept one
      - michelangelo-apiserver
    ttl: 1h           # optional; 0 uses the cluster default; Kubernetes enforces a 10m minimum
```

**Custom worker binaries.** Runner tokens are opt-in at the binary level as well as in config. The stock worker (`go/cmd/worker`) includes `workermod.RunnerTokenModule`, which provides the runner-token outbound middleware, so the chart works as is. A deployment with its own worker `main` must include `RunnerTokenModule` to use `worker.runnerToken`: setting `enabled: true` without it fails worker startup with `worker.runnerToken.enabled is true but worker.RunnerTokenModule is not installed`, rather than silently sending no token. Conversely, a deployment that injects its own `middleware.UnaryOutbound` (for example an identity-mesh token) must leave `RunnerTokenModule` out, because the module and a custom middleware compete for the same `fx` slot; it keeps `worker.runnerToken.enabled: false`. To use a custom `TokenSource` instead, build an `auth.NewBearerOutboundMiddleware` from it and provide that in place of the module.

### Controller manager to compute cluster

Uses the `ray-manager` service account token stored as a Secret in the control plane namespace. See [Register a Compute Cluster](register-a-compute-cluster-to-michelangelo-control-plane.md) for the full setup including token rotation guidance.

## Securing Storage Access

Do not allow users or services to directly access etcd or object storage (S3/MinIO) in ways that bypass the Michelangelo AI API. For S3 access:

- Supply object storage credentials through a Kubernetes Secret (`objectStorage.existingSecret`) rather than values files; see [Platform Setup](./platform-setup.md#object-store-configuration)
- Do not grant `s3:*` to individual users; use IAM policies scoped to specific buckets and prefixes
- Audit S3 bucket policies regularly to ensure no public or cross-account access is inadvertently granted

## Troubleshooting

| Symptom | Likely cause |
|---------|--------------|
| API server fails to start with `auth: unknown auth mode` | Typo in `apiserver.auth.mode`. Use `dummy` or `k8s-rbac` |
| API server fails at startup during OIDC discovery | The issuer in `oidc.issuerUrl` is unreachable or misconfigured; discovery times out after 30 seconds |
| `missing authorization header`, `authorization header must use the Bearer scheme`, or `empty bearer token` | The caller sent no token or a malformed one. For worker calls, check that `worker.runnerToken` is enabled |
| `token verification failed` or `authentication failed` | Issuer or audience mismatch, or an expired token |
| `<user> cannot <verb> <kind> in namespace "<namespace>"` | The caller has no Role or RoleBinding granting that verb on the `michelangelo.api` resource in that namespace |
| `runnertoken: outbound call has no project namespace on the context` | A worker call carried no project namespace, so no runner token could be minted |
| Worker fails to start with `worker.runnerToken.enabled is true but worker.RunnerTokenModule is not installed` | A custom worker `main` enables `runnerToken` without including `workermod.RunnerTokenModule`. Add the module, or set `runnerToken.enabled: false` if you provide your own outbound middleware |

## What's Next

- **Keeping the UI working under `k8s-rbac`**: Front it with an auth proxy — see [UI Authentication](./ui-authentication.md)
- **Network configuration**: Set up Ingress, TLS, and Envoy CORS rules in the [Network & Ingress guide](./network.md)
- **Compliance**: Configure audit logging and data-residency controls for SOC 2, GDPR, or HIPAA in the [Compliance guide](../operations/compliance.md)
- **Monitoring**: Set up Prometheus scraping and alerting for the control plane in the [Monitoring guide](../operations/monitoring.md)
