---
last_verified: 2026-10-14
---

# Deploying Michelangelo AI UI

This guide covers how the Michelangelo AI UI fits into Michelangelo AI's Kubernetes deployments and how operators can adapt the Helm chart configuration for their environments.

### Context
The Michelangelo AI UI is a React-based web application that provides a graphical interface for managing projects, pipelines, and monitoring pipeline runs in the Michelangelo AI ML platform.

### Audience
- Platform operators looking to understand UI deployment requirements
- Kubernetes administrators adapting the sandbox for production use
- Teams wanting to customize UI configuration for their environment

## Key Concepts

- **Helm Chart**: The `michelangelo` chart (`helm/michelangelo`) deploys the UI, Envoy, and API server together. See the [Helm Chart guide](../helm-chart.md)
- **ConfigMap Injection**: Runtime configuration mounted into containers without rebuilding images
- **proto3 JSON**: The UI sends and receives protobuf's JSON mapping as plain HTTP `POST /{service}/{method}` requests
- **Envoy Proxy**: Proxy running the gRPC-JSON transcoder, which converts those JSON requests into gRPC calls to the API server
- **Template Customization**: Adapting chart values for specific infrastructure requirements

## Setup

### Prerequisites
- Kubernetes cluster with Michelangelo AI API server deployed
- Access to Michelangelo AI UI container image from GitHub Container Registry

### UI Deployment Requirements

#### 1. Container Deployment
The chart deploys the UI container (`ui.enabled: true` by default) with runtime configuration mounted:

**Image Selection:**
Available from GitHub Container Registry at `ghcr.io/michelangelo-ai/ui`:
- **Chart default**: the chart's `appVersion` (`images.ui.tag: ""`)
- **Latest development**: `:main` (what `values-k3d.yaml` uses)
- **Specific commit**: `:sha-<short-sha>`
- **Pull command**: `docker pull ghcr.io/michelangelo-ai/ui:your-chosen-tag`

Override with `images.ui.repository` and `images.ui.tag` in your Helm values.

**Container Requirements:**
- **Configuration**: `config.json` mounted at `/usr/share/nginx/html/config.json` (the chart does this from a ConfigMap)
- **Port**: Container serves on port 80

The UI needs to know how to connect to your Michelangelo AI API server. This is configured through `ui.apiBaseUrl`, which the chart renders into the `config.json` ConfigMap:

- **API endpoint**: Browser-reachable URL of the Envoy proxy (not the API server itself)

```yaml
ui:
  apiBaseUrl: "https://michelangelo.example.com/api"
```

The resulting `config.json` is:

```json
{
  "apiBaseUrl": "https://michelangelo.example.com/api"
}
```

`ui.apiBaseUrl` is required when `ui.enabled=true`. If you leave it empty and enable `envoy.ingress` with at least one host, the chart derives it from the first Envoy ingress host and path.

#### 2. Envoy Proxy Setup
The UI communicates with the API server through Envoy (`envoy.enabled: true` by default), which requires:

**Port Configuration:**
- Envoy listens on port 8081 (`envoy.port`)
- The API server listens on its gRPC port, 15566 by default (`apiserver.port`)
- The UI makes requests to Envoy, not directly to the API server

**CORS Configuration:**
Envoy must allow the UI's origin. Set `envoy.corsOrigins` to a regular expression matching the scheme, host, and port of the UI (escape literal dots). It is required on every Ingress install:
```yaml
envoy:
  corsOrigins: "https://michelangelo\\.example\\.com"
```
For local development use `"http://localhost:[0-9]+"`. If unset, the chart allows any origin (`.*`).

**Transcoding and Backend Routing:**
The chart renders an Envoy config (`helm/michelangelo/templates/core/envoy-configmap.yaml`) with a single route to the `michelangelo-apiserver` cluster, whose address and port are filled in from the API server Service and `apiserver.port`. The `envoy.filters.http.grpc_json_transcoder` filter handles the allowlisted services listed in `helm/michelangelo/files/transcoder-services.json` and the descriptor set mounted from `helm/michelangelo/files/descriptors.pb`. Its `print_options` set `always_print_enums_as_ints: false` and `always_print_primitive_fields: true`, which the UI relies on, so keep them if you maintain your own Envoy config.

If you maintain your own Envoy config rather than using the chart, the route `cluster:` name must match the cluster definition `name:`:

```yaml
# In the route configuration:
route:
  cluster: michelangelo-apiserver  # This name must match below

# In the clusters section:
clusters:
  - name: michelangelo-apiserver  # Must match the route cluster name
    load_assignment:
      cluster_name: michelangelo-apiserver
      endpoints:
        - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: michelangelo-apiserver  # Your API server service name
                    port_value: 15566                # Your API server gRPC port
```

**Common Issue:** Mismatched cluster names between route configuration and cluster definition will cause "no healthy upstream" errors.

#### 3. Network Connectivity
Ensure network paths are configured:
- Browser → Envoy proxy (port 8081, or your Envoy ingress)
- Envoy proxy → API server (port 15566)
- External access → UI container (Service port 80 by default, `ui.service.port`)

**External Access Details:**
The UI container serves the React application on port 80 (standard HTTP). Users need to be able to reach this port through your Kubernetes networking setup. Common approaches:
- **NodePort**: Set `ui.service.type: NodePort` and `ui.service.nodePort` (sandbox default is `30011`, with Envoy on `30010`)
- **Ingress**: Domain-based routing via `ui.ingress` and `envoy.ingress` (e.g., `michelangelo.yourcompany.com`)
- **Port forwarding**: Development access via `kubectl port-forward`

The UI serves static files (HTML, JS, CSS) from nginx, so it's a standard web server that needs HTTP access from users' browsers.

### Reference Implementation
For a complete working example, see the [Sandbox Guide](../../getting-started/sandbox-setup.md), which installs the chart with `helm/michelangelo/values-k3d.yaml` and has all components configured and connected.

### Verification
Once deployed, verify the connection chain:
1. **UI loads**: Access the UI in browser
2. **Config loads**: Check browser dev tools for `/config.json`
3. **API connectivity**: Verify API calls succeed (no CORS errors in browser console)

## Troubleshooting

### Common Issues

**Envoy "no healthy upstream" errors:**
- **Cause:** Cluster name mismatch between route configuration and cluster definition, or the API server Service/port is unreachable from Envoy
- **Fix:** Ensure the `cluster:` field in routes matches the `name:` field in clusters section, and that the cluster address and port match the API server Service and `apiserver.port`
- **Check:** Envoy logs will show "no healthy upstream for cluster 'cluster-name'"

**CORS errors in browser:**
- Verify `envoy.corsOrigins` matches your UI's origin (scheme, host, and port)
- Check browser dev tools → Network tab for failed requests
- Remember the value is a regular expression, so escape literal dots as `\.`

### FAQ

**Q: Can I use a custom domain?**
A: Yes, configure `ui.ingress` and `envoy.ingress`, and update `envoy.corsOrigins` to match.

**Q: What if my API server uses different ports?**
A: Set `apiserver.port`; the chart updates the Envoy cluster to match. Update `ui.apiBaseUrl` only if you change how Envoy is exposed (`envoy.port` or the Envoy ingress).
