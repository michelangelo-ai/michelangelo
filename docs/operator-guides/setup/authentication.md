---
last_verified: 2026-10-14
---

# Authentication & Identity

This guide is for platform operators and cluster administrators configuring authentication for a Michelangelo AI deployment.

**Prerequisites**: A running Michelangelo AI control plane (see [Platform Setup](./platform-setup.md)) and `kubectl` access to the namespace the Helm chart is installed in.

## What ships by default

The stock Michelangelo AI API server (`go/cmd/apiserver`) wires `auth.AuthModule`, which reads the `apiserver.auth` config key and picks an implementation by `mode`. The default (`mode` unset or `dummy`) allows every request and installs no authentication middleware, so out of the box the API server does **not** authenticate or authorize callers. Setting `apiserver.auth.mode: k8s-rbac` turns on bearer-token authentication and Kubernetes RBAC authorization; see [Enabling Kubernetes RBAC mode](#enabling-kubernetes-rbac-mode).

The Helm chart renders no `auth` block: there is no `apiserver.auth.*` Helm value, and no `rbacEnabled` / `sessionTokenExpiry` setting. To enable authentication you supply the API server's configuration yourself.

Until you enable authentication, protect the API surface at the network layer: keep the API server, Envoy, and UI off the public internet, restrict access with Ingress and NetworkPolicy rules (see [Network & Ingress](./network.md)), and put an authenticating proxy or ingress in front of the UI if you expose it.

Authentication in Michelangelo AI is built from two independent, pluggable layers in the `go/auth` package:

1. **Transport authentication** (`go/auth/authn.go`). `auth.NewInboundMiddleware` is a YARPC inbound middleware that requires an `authorization: Bearer <token>` header, verifies it with a deployment-specific `Authenticator`, optionally restricts callers to an allowlist of service names (`AllowedServices`), and stores the verified `Principal` on the request context. It is installed only when an `Authenticator` is provided. For outbound calls, `auth.NewBearerOutboundMiddleware` attaches a token from a `TokenSource`.
2. **Handler authorization**. The generated API handlers call the `auth.Auth` interface (`UserAuthenticated`, `UserAuthorized`). `DummyAuth` allows everything and is the default. `auth.NewK8sRBACAuth` adapts a `k8s.io/apiserver` authorizer to this interface so decisions can be made with Kubernetes RBAC.

The built-in path needs **configuration only, no rebuild**: in `k8s-rbac` mode the stock API server assembles both layers itself (OIDC and/or ServiceAccount token authentication, RBAC authorization), and the stock worker attaches per-project tokens when `worker.runnerToken` is enabled (see [Service Authentication](#service-authentication-internal)). A custom binary (or image) is needed only for a different `Authenticator` or `TokenSource`.

**Custom Authenticator or TokenSource (advanced)**: The API server (`DispatcherParams.InboundMiddleware`) and the worker (`Params.OutboundMiddleware`) take these middlewares as optional `fx` dependencies, wired in the same way as described in [Notification Delivery Setup](../notifications.md#step-2-implement-delivery). Because `auth.AuthModule` and the worker module now provide those slots themselves, a custom middleware must replace that provider rather than add a second one; `fx` rejects two providers of the same type.

## OIDC token verification

`go/auth/oidc` provides an OIDC ID-token verifier (`oidc.New`) that returns a `k8s.io/apiserver` token authenticator. `auth.FromTokenAuthenticator` adapts it for `NewInboundMiddleware`. The verifier takes this configuration. In `k8s-rbac` mode, set these keys under `apiserver.auth.oidc` (see [Enabling Kubernetes RBAC mode](#enabling-kubernetes-rbac-mode)); a custom wiring can also load them and call `oidc.New` directly:

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

## Enabling Kubernetes RBAC mode

Set `apiserver.auth` in the API server's configuration.

:::warning
Switching to `k8s-rbac` rejects every caller that doesn't present a valid bearer token, including the worker. Do these in order before you set `mode`:

1. Add the ClusterRole rules and the per-project `michelangelo-runner` ServiceAccount from [Prerequisites the chart doesn't provide](#prerequisites-the-chart-doesnt-provide).
2. Create Roles and RoleBindings for your users (see [Granting Access with RBAC](#granting-access-with-rbac)).
3. Enable `worker.runnerToken` (see [Per-project worker identity](#per-project-worker-identity)).
4. Set `mode: k8s-rbac`.

Try the change in a non-production cluster first.
:::

```yaml
apiserver:
  auth:
    mode: k8s-rbac
    oidc:                      # human callers; keys as in OIDC token verification above
      issuerUrl: https://accounts.your-idp.com
      audiences:
        - michelangelo
    serviceAccounts:           # in-cluster callers, verified with TokenReview
      enabled: true
      audiences:               # optional; passed to TokenReview
        - michelangelo-apiserver
    sarCache:                  # SubjectAccessReview decision cache
      allowTTL: 10s
      denyTTL: 10s
```

| `mode` | Behavior |
|--------|----------|
| unset or `dummy` | Every request is allowed and no authentication middleware is installed. This is the default, suitable for local development or network-isolated clusters only |
| `k8s-rbac` | Every call must carry an `authorization: Bearer <token>` header. The token is verified as an OIDC ID token and/or a Kubernetes ServiceAccount token (TokenReview). Each call is then authorized with a SubjectAccessReview against the `michelangelo.api` API group |
| anything else | The API server fails to start with `unknown auth mode`. There is no silent fallback to allow-all |

In `k8s-rbac` mode:

- At least one of `oidc.issuerUrl` and `serviceAccounts.enabled: true` must be set, or the API server fails to start.
- OIDC discovery runs at startup with a 30-second timeout, so an unreachable or misconfigured issuer fails startup instead of denying every request later.
- Verified tokens are cached for 10 seconds. This isn't configurable, so a revoked token can keep working for up to 10 seconds.
- SubjectAccessReview decisions are cached separately for allows (`sarCache.allowTTL`) and denies (`sarCache.denyTTL`). Each defaults to 10 seconds; an explicit `0` disables that cache.
- Because authorization uses the `michelangelo.api` API group, the Roles in [Granting Access with RBAC](#granting-access-with-rbac) apply as written. ServiceAccount callers authenticate as `system:serviceaccount:<namespace>:<name>`.

**Rolling back**: Set `mode: dummy` (or remove the `auth` block) and restart the API server. `worker.runnerToken` can stay enabled, because `dummy` mode installs no inbound middleware and ignores the header.

### Prerequisites the chart doesn't provide

The Helm chart does not set any of this up for you:

1. **No Helm values.** The API server ConfigMap (`templates/core/apiserver-configmap.yaml`) renders no `auth` block, and the worker ConfigMap renders no `runnerToken`. There are no `values.yaml` keys for either. Supply your own configuration, for example a ConfigMap of your own or an overriding `base.yaml`.
2. **Missing RBAC rules.** The API server and worker run as one shared ServiceAccount (the chart's `serviceAccount`, named after the release by default). The chart's ClusterRole (`templates/rbac/clusterrole.yaml`) grants it none of the following, so add them yourself:

   | Rule | Needed by |
   |------|-----------|
   | `create` on `subjectaccessreviews` (`authorization.k8s.io`) | API server, `mode: k8s-rbac` |
   | `create` on `tokenreviews` (`authentication.k8s.io`) | API server, `serviceAccounts.enabled: true` |
   | `create` on `serviceaccounts/token` (core) | Worker, `worker.runnerToken.enabled: true` |

   ```yaml
   apiVersion: rbac.authorization.k8s.io/v1
   kind: ClusterRole
   metadata:
     name: michelangelo-auth
   rules:
     - apiGroups: ["authorization.k8s.io"]
       resources: ["subjectaccessreviews"]
       verbs: ["create"]
     - apiGroups: ["authentication.k8s.io"]
       resources: ["tokenreviews"]
       verbs: ["create"]
     - apiGroups: [""]
       resources: ["serviceaccounts/token"]
       verbs: ["create"]
   ---
   apiVersion: rbac.authorization.k8s.io/v1
   kind: ClusterRoleBinding
   metadata:
     name: michelangelo-auth
   subjects:
     - kind: ServiceAccount
       name: <michelangelo-service-account>   # kubectl get sa -n <release-namespace>
       namespace: <release-namespace>
   roleRef:
     kind: ClusterRole
     name: michelangelo-auth
     apiGroup: rbac.authorization.k8s.io
   ```

3. **No `michelangelo-runner` ServiceAccount.** If you enable `worker.runnerToken`, neither the chart nor the project API hook creates the per-project ServiceAccount it mints tokens for. See [Service Authentication](#service-authentication-internal).

## Multi-Factor Authentication

MFA is enforced at the IdP level, not within Michelangelo AI. Configure MFA policies in your identity provider's admin console; tokens that reach Michelangelo AI have already completed the IdP flow.

## Granting Access with RBAC

When authorization is backed by Kubernetes RBAC (`apiserver.auth.mode: k8s-rbac`, or a custom wiring of `auth.NewK8sRBACAuth`), or when users access Michelangelo AI resources directly with `kubectl`, users need a `RoleBinding` or `ClusterRoleBinding` on the `michelangelo.api` resources. Define the Roles yourself, for example (see also [Model Registry](../components/model-registry.md#step-3-configure-rbac)):

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

### Per-project worker identity

With `apiserver.auth.mode: k8s-rbac`, the worker's calls need a bearer token too. Enable `worker.runnerToken` in the worker's configuration; this requires `serviceAccounts.enabled: true` on the API server. With `k8s-rbac` and `runnerToken` disabled (and no custom `TokenSource`), the worker sends no token, so every worker call is rejected and pipeline runs and triggers fail.

```yaml
worker:
  runnerToken:
    enabled: true     # default false: worker traffic is unchanged
    audiences:        # optional; the API server's serviceAccounts.audiences must accept one
      - michelangelo-apiserver
    ttl: 1h           # optional; 0 uses the cluster default; Kubernetes enforces a 10m minimum
```

When enabled, the worker mints a short-lived token for the `michelangelo-runner` ServiceAccount in **each project's namespace** (TokenRequest API), caches it until one minute before it expires, and attaches it to the calls it makes for that project. The API server then authorizes each call against that namespace's RoleBindings instead of a platform-wide worker identity. A call that carries no project namespace fails rather than going out unauthenticated.

Before you enable it:

- Grant the chart's ServiceAccount `create` on `serviceaccounts/token` (see [Prerequisites the chart doesn't provide](#prerequisites-the-chart-doesnt-provide)).
- Create the `michelangelo-runner` ServiceAccount in **every** project namespace, plus a RoleBinding granting it what pipeline runs need on the `michelangelo.api` resources (define the Role as in the example under [Granting Access with RBAC](#granting-access-with-rbac)). Neither the chart nor the project API hook creates it:

  ```yaml
  apiVersion: v1
  kind: ServiceAccount
  metadata:
    name: michelangelo-runner
    namespace: ml-team-project
  ---
  apiVersion: rbac.authorization.k8s.io/v1
  kind: RoleBinding
  metadata:
    name: michelangelo-runner
    namespace: ml-team-project
  subjects:
    - kind: ServiceAccount
      name: michelangelo-runner
      namespace: ml-team-project
  roleRef:
    kind: Role
    name: <role-for-pipeline-runs>   # a Role you define; see Granting Access with RBAC
    apiGroup: rbac.authorization.k8s.io
  ```

A custom `TokenSource` is still possible: build an `auth.NewBearerOutboundMiddleware` from it and use it in place of the worker module's runner-token middleware.

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

## What's Next

- **Network configuration**: Set up Ingress, TLS, and Envoy CORS rules in the [Network & Ingress guide](./network.md)
- **Compliance**: Configure audit logging and data-residency controls for SOC 2, GDPR, or HIPAA in the [Compliance guide](../operations/compliance.md)
- **Monitoring**: Set up Prometheus scraping and alerting for the control plane in the [Monitoring guide](../operations/monitoring.md)
