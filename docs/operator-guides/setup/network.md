---
last_verified: 2026-10-14
---

# Network & Ingress Configuration

This guide covers the network configuration required to deploy Michelangelo AI in a Kubernetes cluster: Envoy proxy settings (CORS, cluster hostnames), Ingress setup for the API server, Envoy, and UI through the Helm chart, TLS with cert-manager, and connectivity requirements for multi-cluster deployments.

---

## Overview

Michelangelo AI's network surface has up to three external-facing entry points, each with its own Ingress (and a distinct hostname):

| Entry Point | Helm values | Default Service Port | Purpose |
|-------------|-------------|---------|---------|
| API Server Ingress | `apiserver.ingress` | 15566 (gRPC) | gRPC API used by the `ma` CLI and SDK clients outside the cluster |
| Envoy Ingress | `envoy.ingress` | 8081 (HTTP) | JSON-over-HTTP API proxy the browser UI calls |
| UI Ingress | `ui.ingress` | 80 (HTTP) | Static UI assets served by nginx |

Traffic flow from the public internet to internal components:

```
Internet
  │
  ├─ api.your-domain.com ──► Ingress ──► michelangelo-apiserver:15566 (gRPC)
  │
  ├─ app.your-domain.com ──► Ingress ──► michelangelo-ui:80 (static UI)
  │
  └─ envoy.your-domain.com ─► Ingress ──► michelangelo-envoy:8081
                                                  │
                                                  └─► michelangelo-apiserver:15566 (gRPC, after JSON transcoding)
```

---

## Envoy Proxy Configuration

The Envoy proxy sits in front of the API server for browser clients. It runs Envoy's gRPC-JSON transcoder, which turns the UI's proto3 JSON `POST /{service}/{method}` requests into gRPC calls, and it handles CORS. The chart renders the Envoy configuration from `helm/michelangelo/templates/core/envoy-configmap.yaml`; you do not edit the ConfigMap directly.

### CORS Configuration

Add your UI origin to Envoy's allowed origins with the `envoy.corsOrigins` Helm value. It is a regular expression (Go/RE2 syntax) matched against the browser's `Origin` header, and it is required on every Ingress install because the UI and Envoy run on different origins:

```yaml
envoy:
  corsOrigins: "https://app\\.your-domain\\.com"
```

If unset, the chart allows any origin (`.*`). The rendered ConfigMap contains:

```yaml
cors:
  allow_origin_string_match:
    - safe_regex:
        regex: "https://app\\.your-domain\\.com"
  allow_methods: "GET, POST, OPTIONS"
  allow_headers: "connect-protocol-version,connect-timeout-ms,content-type,context-ttl-ms,grpc-timeout,rpc-caller,rpc-encoding,rpc-service,x-grpc-web,x-user-agent,x-user-email,x-user-name"
  expose_headers: "grpc-status,grpc-message"
  max_age: "1728000"
```

The API cluster (`michelangelo-apiserver`) is rendered with the API server Service name and `apiserver.port` (default `15566`); when `apiserver.tls.enabled` is true, Envoy connects to it over TLS.

**Values to customize per environment:**

| Helm value | Description |
|-------|-------------|
| `envoy.corsOrigins` | Regex matching your UI origin (scheme, host, and port) |
| `envoy.port` | Envoy listener and Service port (default `8081`) |
| `apiserver.port` | API server gRPC port Envoy forwards to (default `15566`) |

### Envoy TLS Termination

Terminate TLS for Envoy at the Ingress (`envoy.ingress.tls`). The chart does not expose values for TLS on the Envoy listener itself; terminating inside the Envoy pod would require a custom Envoy ConfigMap and volume mounts. See [TLS with cert-manager](#tls-with-cert-manager) below.

---

## Ingress Setup

The chart creates each Ingress from Helm values when you set `<component>.ingress.enabled: true`. Hostnames of the three Ingress resources must differ from each other.

### API Server Ingress

The API server uses gRPC (HTTP/2). Your Ingress controller must support HTTP/2 backend connections. With NGINX Ingress Controller:

```yaml
apiserver:
  ingress:
    enabled: true
    mode: grpc            # or "passthrough" (requires apiserver.tls.enabled=true)
    className: nginx
    annotations:
      nginx.ingress.kubernetes.io/ssl-redirect: "true"
      nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
      nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
    hosts:
      - host: api.your-domain.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: michelangelo-apiserver-tls
        hosts:
          - api.your-domain.com
```

In `grpc` mode the chart adds `nginx.ingress.kubernetes.io/backend-protocol: GRPC`; in `passthrough` mode it adds `nginx.ingress.kubernetes.io/ssl-passthrough: "true"` and the pod terminates TLS.

> **HTTP/2 requirement:** gRPC requires HTTP/2 end-to-end. If your Ingress controller terminates TLS but connects to the backend over HTTP/1.1, gRPC calls will fail. Ensure the `backend-protocol: GRPC` annotation (or equivalent for your controller) is present.

### UI and Envoy Ingress

The UI and the Envoy proxy each get their own Ingress. The browser loads the UI from the UI host and calls Envoy on the Envoy host (or a path on a shared host, if your controller rewrites paths):

```yaml
ui:
  ingress:
    enabled: true
    className: nginx
    hosts:
      - host: app.your-domain.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: michelangelo-ui-tls
        hosts:
          - app.your-domain.com
  apiBaseUrl: "https://envoy.your-domain.com"

envoy:
  corsOrigins: "https://app\\.your-domain\\.com"
  ingress:
    enabled: true
    className: nginx
    annotations:
      nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    hosts:
      - host: envoy.your-domain.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: michelangelo-envoy-tls
        hosts:
          - envoy.your-domain.com
```

If `ui.apiBaseUrl` is left empty, the chart derives it from the first `envoy.ingress` host and path (using `https` when `envoy.ingress.tls` is set). `envoy.corsOrigins` must match the scheme and host of `ui.ingress.hosts[0].host`.

### Domain Names to Update in Helm Values

After choosing hostnames, propagate them through Helm values:

| Helm value | Value |
|----------|-------|
| `ui.apiBaseUrl` | `https://envoy.your-domain.com` (or derived from `envoy.ingress.hosts[0]`) |
| `envoy.corsOrigins` | Regex for your UI origin, e.g. `https://app\\.your-domain\\.com` |
| `apiserver.ingress.hosts`, `ui.ingress.hosts`, `envoy.ingress.hosts` | One distinct hostname each |

The worker runs in the same release as the API server and reaches it through the in-cluster Service, so `worker.address` needs no change. Point it at the external address only if you run the worker elsewhere (and see `worker.useTLS` in [Authentication](authentication.md#service-authentication-internal)).

See [Platform Setup — Environment Overrides](platform-setup.md#environment-overrides--domain-settings) for the full list.

---

## TLS with cert-manager

Use cert-manager to automate TLS certificate provisioning. Install cert-manager if it is not already present:

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
```

### ClusterIssuer (Let's Encrypt)

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: platform-team@your-domain.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
      - http01:
          ingress:
            class: nginx
```

### Referencing the Issuer in Ingress

Add the cert-manager annotation to each Ingress through its `annotations` value (`apiserver.ingress.annotations`, `envoy.ingress.annotations`, `ui.ingress.annotations`):

```yaml
ui:
  ingress:
    annotations:
      cert-manager.io/cluster-issuer: "letsencrypt-prod"
```

cert-manager will automatically create and renew the TLS Secret referenced in `spec.tls[].secretName`.

### Using an Internal CA

For private clusters that cannot use ACME, use a `ClusterIssuer` backed by an internal CA:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: internal-ca
spec:
  ca:
    secretName: internal-ca-key-pair   # Secret containing the CA cert and key
```

---

## Multi-Cluster Network Topology

When Michelangelo AI's control plane dispatches jobs to registered compute clusters, the following connectivity is required:

:::warning No automatic failover
Michelangelo AI does not automatically fail over if the control plane API server becomes unreachable. Task pods in compute clusters cannot report results, and new jobs cannot be dispatched. Configure alerting on the controller manager's health endpoint (health probe port `controllermgr.healthPort`, `8081` by default) so on-call is paged before users are impacted — see [Monitoring](../operations/monitoring.md).
:::

```
Control Plane Cluster                    Compute Cluster
┌────────────────────────┐               ┌──────────────────────────────┐
│ Controller Manager     │──── HTTPS ───►│ Kubernetes API server        │
│ (kubeconfig for each   │               │ (port 443)                   │
│  compute cluster)      │               └──────────────────────────────┘
│                        │
│ API server             │◄──── gRPC ────┤ Task pods (report back via   │
│ (port 15566)           │               │  REGISTRY_ENDPOINT)          │
└────────────────────────┘               └──────────────────────────────┘
```

### Required Connectivity

| Direction | Source | Destination | Port | Purpose |
|-----------|--------|-------------|------|---------|
| Outbound from control plane | Controller Manager | Compute cluster K8s API | 443 | Dispatching RayCluster / SparkApplication CRDs |
| Outbound from compute | Task pods | Michelangelo AI API server | 15566 (or 443 through the API server Ingress) | Model registration and other API calls (`REGISTRY_ENDPOINT` in `michelangelo-config`) |
| Outbound from compute | Task pods | S3 / object store | 443 | Artifact reads and writes |

### NetworkPolicy for Control Plane → Compute Cluster

If your compute cluster enforces NetworkPolicy, ensure the control plane's egress IP range can reach the Kubernetes API server:

> **Managed Kubernetes (EKS, GKE, AKS):** The API server runs outside the cluster on managed platforms and is not a schedulable pod. This NetworkPolicy only applies to self-managed clusters. For managed clusters, use your cloud provider's security groups or authorized networks instead.

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-michelangelo-controller
  namespace: kube-system
spec:
  podSelector:
    matchLabels:
      component: kube-apiserver
  policyTypes:
    - Ingress
  ingress:
    - from:
        - ipBlock:
            cidr: <control-plane-egress-cidr>/32
      ports:
        - protocol: TCP
          port: 443
```

### Verifying Cross-Cluster Connectivity

From the Controller Manager pod, verify it can reach each registered compute cluster:

```bash
# Exec into the controller manager pod
kubectl exec -it deploy/michelangelo-controllermgr -n <release-namespace> -- /bin/sh

# Check connectivity to a registered compute cluster's K8s API
curl -sk https://<compute-cluster-api-server>:443/healthz
```

From a task pod in the compute cluster, verify it can reach the Michelangelo AI API server:

```bash
kubectl exec -it <task-pod> -n <compute-namespace> -- \
  env | grep REGISTRY_ENDPOINT
```

---

## Checklist

Use this checklist when deploying Michelangelo AI to a new environment:

- [ ] Ingress controller installed and supports HTTP/2 (for gRPC)
- [ ] API server Ingress created (`apiserver.ingress`, `mode: grpc`)
- [ ] UI and Envoy Ingresses created on distinct hostnames (`ui.ingress`, `envoy.ingress`)
- [ ] TLS certificates provisioned (cert-manager or manual)
- [ ] `envoy.corsOrigins` updated to match the UI origin
- [ ] `ui.apiBaseUrl` updated to the Envoy URL
- [ ] Cross-cluster connectivity verified (controller manager → compute K8s API)
- [ ] Task pod → Michelangelo AI API server connectivity verified

---

## Related

- [Platform Setup — Environment Overrides](platform-setup.md#environment-overrides--domain-settings)
- [Authentication](authentication.md)
- [Register a Compute Cluster](register-a-compute-cluster-to-michelangelo-control-plane.md)
- [Troubleshooting](../operations/troubleshooting.md)
