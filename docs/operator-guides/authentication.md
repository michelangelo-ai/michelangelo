# Authentication & Identity

This guide is for platform operators and cluster administrators configuring authentication for a Michelangelo deployment.

**Prerequisites**: A running Michelangelo control plane (see [Platform Setup](./platform-setup.md)) and `kubectl` access to the `ma-system` namespace.

Authentication in Michelangelo operates at two levels:

- **User authentication** — end users authenticate to the Michelangelo API and UI via an identity provider (IdP)
- **Service authentication** — internal services (worker, controller manager) authenticate to each other using Kubernetes service account tokens

This guide covers configuring both, plus RBAC authorization and multi-tenant isolation.

## Enabling RBAC

RBAC is disabled by default. Enable it in the API server ConfigMap overlay before connecting an identity provider:

```yaml
apiserver:
  auth:
    rbacEnabled: true
```

Apply the overlay and restart the API server:

```bash
kubectl rollout restart deployment/michelangelo-apiserver -n ma-system
```

Once RBAC is enabled, users without a RoleBinding will be denied access to all resources.

## Connecting an Identity Provider (OIDC)

Michelangelo supports any OIDC-compliant identity provider. Configure it in the API server ConfigMap:

```yaml
apiserver:
  auth:
    rbacEnabled: true
    oidc:
      issuerUrl: https://accounts.your-idp.com
      clientId: michelangelo
      usernameClaim: email      # JWT claim used as the Michelangelo username
      groupsClaim: groups       # JWT claim used for group-based RBAC
```

### Okta

1. In the Okta admin console, create an application of type **Web**
2. Set the **Sign-in redirect URI** to `https://michelangelo-envoy.your-domain/callback`
3. Copy the **Client ID** and **Okta domain** into the config:
   ```yaml
   oidc:
     issuerUrl: https://your-org.okta.com
     clientId: <client-id-from-okta>
   ```

### Google Workspace

1. In Google Cloud Console, create an **OAuth 2.0 Client ID** of type Web application
2. Add your Michelangelo Envoy URL as an authorized redirect URI
3. Set the issuer URL:
   ```yaml
   oidc:
     issuerUrl: https://accounts.google.com
     clientId: <client-id>.apps.googleusercontent.com
     usernameClaim: email
     groupsClaim: hd    # Google Workspace hosted domain
   ```

### Azure Active Directory

1. Register a new application in the Azure portal
2. Set the redirect URI to your Michelangelo Envoy callback URL
3. Note the **Application (client) ID** and **Directory (tenant) ID**:
   ```yaml
   oidc:
     issuerUrl: https://login.microsoftonline.com/<tenant-id>/v2.0
     clientId: <application-client-id>
     usernameClaim: upn        # User Principal Name (email format)
     groupsClaim: groups
   ```

### Keycloak

1. Create a realm and a Client with Client Protocol `openid-connect`
2. Set the redirect URI and note the client ID:
   ```yaml
   oidc:
     issuerUrl: https://keycloak.your-domain.com/realms/<realm-name>
     clientId: michelangelo
   ```

## Session Token Configuration

Control how long a user's session remains valid:

```yaml
apiserver:
  auth:
    sessionTokenExpiry: 8h    # Valid time units: h, m, s
```

8 hours is a reasonable default for a standard workday. Shorter expiry increases security but requires more frequent re-authentication.

## Multi-Factor Authentication

MFA is enforced at the IdP level, not within Michelangelo. Configure MFA policies in your identity provider's admin console. Michelangelo requires users to complete the full IdP authentication flow — including MFA — before issuing a session token.

## Granting Access with RBAC

After RBAC is enabled, users need a `RoleBinding` or `ClusterRoleBinding` to access Michelangelo resources.

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
  name: viewer
  apiGroup: rbac.authorization.k8s.io
```

### Grant a team admin access via group membership

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: ml-team-admins
  namespace: ml-team-project
subjects:
- kind: Group
  name: ml-team                  # Must match the value of groupsClaim in the JWT
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: editor
  apiGroup: rbac.authorization.k8s.io
```

Use `RoleBinding` to scope access to a specific namespace. Use `ClusterRoleBinding` only for platform administrators who need cross-namespace access.

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

This allows traffic within the team's namespace and from the Michelangelo control plane, but blocks all other namespaces.

## Service Authentication (Internal)

Michelangelo services authenticate to each other using Kubernetes service account tokens.

**Worker → API server**: Configured via `worker.useTLS: true` in the worker ConfigMap. The worker uses its Kubernetes pod service account token. Do not set `useTLS: false` in production.

```yaml
worker:
  address: michelangelo-apiserver.ma-system.svc.cluster.local:15566
  maApiServiceName: ma-apiserver
  useTLS: true
```

**Controller manager → compute cluster**: Uses the `ray-manager` service account token stored as a Secret in the control plane namespace. See [Register a Compute Cluster](jobs/register-a-compute-cluster-to-michelangelo-control-plane.md) for the full setup including token rotation guidance.

## Disabling Direct Storage Access

Do not allow users or services to directly access etcd or object storage (S3/MinIO) in ways that bypass the Michelangelo API. For S3 access:

- Set `useIam: true` in the controller manager ConfigMap — this uses IAM roles attached to pods via ServiceAccount annotations, not hardcoded credentials
- Do not grant `s3:*` to individual users; use IAM policies scoped to specific buckets and prefixes
- Audit S3 bucket policies regularly to ensure no public or cross-account access is inadvertently granted

## What's next?

- **Network configuration**: Set up Ingress, TLS, and Envoy CORS rules in the [Network & Ingress guide](./network.md)
- **Compliance**: Configure audit logging and data-residency controls for SOC 2, GDPR, or HIPAA in the [Compliance guide](./compliance.md)
- **Monitoring**: Set up Prometheus scraping and alerting for the control plane in the [Monitoring guide](./monitoring.md)
