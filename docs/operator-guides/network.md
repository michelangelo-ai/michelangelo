# Network & Ingress Configuration

This guide covers the network configuration required to deploy Michelangelo in a Kubernetes cluster: Envoy proxy settings (CORS, cluster hostnames), Ingress setup for the API server and UI, TLS with cert-manager, and connectivity requirements for multi-cluster deployments.

---

## Overview

Michelangelo's network surface has two external-facing entry points:

| Entry Point | Default Port | Purpose |
|-------------|-------------|---------|
| API Server Ingress | 443 (HTTPS) | gRPC API used by the `ma` CLI, workers, and SDK |
| UI + Envoy Ingress | 443 (HTTPS) | Browser-facing UI and REST/gRPC-Web proxy |

Traffic flow from the public internet to internal components:

```
Internet
  │
  ├─ api.your-domain.com ──► Ingress ──► michelangelo-apiserver:15566 (gRPC)
  │
  └─ app.your-domain.com ──► Ingress ──► michelangelo-envoy:8081
                                                  │
                                                  └─► michelangelo-apiserver:15566 (gRPC-Web)
```

---

## Envoy Proxy Configuration

The Envoy proxy sits in front of the API server for browser clients. It handles HTTP/1.1 → gRPC transcoding and CORS.

### CORS Configuration

Add your UI domain to Envoy's CORS allowed origins. This is required for the browser-based UI to call the API. In the Envoy ConfigMap:

```yaml
static_resources:
  listeners:
    - address:
        socket_address: { address: 0.0.0.0, port_value: 8081 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                route_config:
                  virtual_hosts:
                    - name: local_service
                      domains: ["*"]
                      cors:
                        allow_origin_string_match:
                          - safe_regex:
                              regex: "https://app\\.your-domain\\.com"
                        allow_methods: "GET, POST, OPTIONS"
                        allow_headers: "content-type, context-ttl-ms, grpc-timeout, rpc-caller, rpc-encoding, rpc-service, x-grpc-web, x-user-agent"
                        expose_headers: "grpc-status, grpc-message"
                        max_age: "1728000"
                      routes:
                        - match: { prefix: "/" }
                          route:
                            cluster: michelangelo-apiserver
                            max_grpc_timeout: 0s
                http_filters:
                  - name: envoy.filters.http.grpc_web
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.grpc_web.v3.GrpcWeb
                  - name: envoy.filters.http.cors
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.cors.v3.Cors
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

  clusters:
    - name: michelangelo-apiserver
      connect_timeout: 30s
      type: LOGICAL_DNS
      http2_protocol_options: {}
      load_assignment:
        cluster_name: michelangelo-apiserver
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: michelangelo-apiserver   # Kubernetes service name
                      port_value: 15566
```

**Fields to customize per environment:**

| Field | Description |
|-------|-------------|
| `allow_origin_string_match.regex` | Replace with your UI domain regex |
| `socket_address.address` | API server Kubernetes service name (default: `michelangelo-apiserver`) |
| `socket_address.port_value` | API server port (default: `15566`) |

### Envoy TLS Termination

If you terminate TLS at the Envoy pod (rather than at the Ingress), add a `transport_socket` to the listener:

```yaml
transport_socket:
  name: envoy.transport_sockets.tls
  typed_config:
    "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
    common_tls_context:
      tls_certificates:
        - certificate_chain:
            filename: /etc/ssl/certs/tls.crt
          private_key:
            filename: /etc/ssl/certs/tls.key
```

Mount the certificate from a Kubernetes Secret:

```yaml
volumes:
  - name: tls-cert
    secret:
      secretName: michelangelo-envoy-tls
volumeMounts:
  - name: tls-cert
    mountPath: /etc/ssl/certs
    readOnly: true
```

In most deployments, TLS is terminated at the Ingress layer instead — see [TLS with cert-manager](#tls-with-cert-manager) below.

---

## Ingress Setup

### API Server Ingress

The API server uses gRPC (HTTP/2). Your Ingress controller must support HTTP/2 backend connections. With NGINX Ingress Controller:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: michelangelo-apiserver
  namespace: michelangelo
  annotations:
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/backend-protocol: "GRPC"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - api.your-domain.com
      secretName: michelangelo-apiserver-tls
  rules:
    - host: api.your-domain.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: michelangelo-apiserver
                port:
                  number: 15566
```

> **HTTP/2 requirement:** gRPC requires HTTP/2 end-to-end. If your Ingress controller terminates TLS but connects to the backend over HTTP/1.1, gRPC calls will fail. Ensure `backend-protocol: GRPC` (or equivalent) is set.

### UI + Envoy Ingress

The UI and gRPC-Web proxy share a single Ingress:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: michelangelo-ui
  namespace: michelangelo
  annotations:
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - app.your-domain.com
      secretName: michelangelo-ui-tls
  rules:
    - host: app.your-domain.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: michelangelo-envoy
                port:
                  number: 8081
```

### Domain Names to Update in Overlays

After setting hostnames in Ingress resources, propagate them through ConfigMaps:

| Location | Field | Value |
|----------|-------|-------|
| Worker ConfigMap | `worker.address` | `api.your-domain.com:443` |
| UI Public Config | `apiBaseUrl` | `https://app.your-domain.com` |
| Envoy CORS config | `allow_origin_string_match.regex` | Your UI domain |

See [Platform Setup — Environment Overrides](index.md#environment-overrides--domain-settings) for the full list.

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

Add the cert-manager annotation to your Ingress resources:

```yaml
metadata:
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

When Michelangelo's control plane dispatches jobs to registered compute clusters, the following connectivity is required:

```
Control Plane Cluster                    Compute Cluster
┌────────────────────────┐               ┌──────────────────────────────┐
│ Controller Manager     │──── HTTPS ───►│ Kubernetes API server        │
│ (kubeconfig for each   │               │ (port 443)                   │
│  compute cluster)      │               └──────────────────────────────┘
│                        │
│ Worker                 │◄──── gRPC ────┤ Task pods (report back       │
│ (port 15566)           │               │  via worker.address)         │
└────────────────────────┘               └──────────────────────────────┘
```

### Required Connectivity

| Direction | Source | Destination | Port | Purpose |
|-----------|--------|-------------|------|---------|
| Outbound from control plane | Controller Manager | Compute cluster K8s API | 443 | Dispatching RayCluster / SparkApplication CRDs |
| Outbound from compute | Task pods | Michelangelo API server | 443 | Worker connectivity for result reporting |
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
kubectl exec -it deploy/michelangelo-controllermgr -n michelangelo -- /bin/sh

# Check connectivity to a registered compute cluster's K8s API
curl -sk https://<compute-cluster-api-server>:443/healthz
```

From a task pod in the compute cluster, verify it can reach the Michelangelo API server:

```bash
kubectl exec -it <task-pod> -n <compute-namespace> -- \
  curl -sk https://api.your-domain.com/healthz
```

---

## Checklist

Use this checklist when deploying Michelangelo to a new environment:

- [ ] Ingress controller installed and supports HTTP/2 (for gRPC)
- [ ] API server Ingress created with `backend-protocol: GRPC`
- [ ] UI + Envoy Ingress created
- [ ] TLS certificates provisioned (cert-manager or manual)
- [ ] Envoy CORS `allow_origin` updated to match UI domain
- [ ] Worker ConfigMap `worker.address` updated to `api.your-domain.com:443`
- [ ] UI `config.json` `apiBaseUrl` updated to UI domain
- [ ] Cross-cluster connectivity verified (controller manager → compute K8s API)
- [ ] Task pod → Michelangelo API server connectivity verified

---

## Related

- [Platform Setup — Environment Overrides](index.md#environment-overrides--domain-settings)
- [Authentication](authentication.md)
- [Register a Compute Cluster](jobs/register-a-compute-cluster-to-michelangelo-control-plane.md)
- [Troubleshooting](troubleshooting.md)
