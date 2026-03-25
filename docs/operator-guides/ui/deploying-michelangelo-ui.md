# Deploying Michelangelo UI

This guide covers how the Michelangelo UI fits into Michelangelo's Kubernetes deployments and how operators can adapt the configuration for their environments.

### Context
The Michelangelo UI is a React-based web application that provides a graphical interface for managing projects, pipelines, and monitoring pipeline runs in the Michelangelo ML platform.

### Audience
- Platform operators looking to understand UI deployment requirements
- Kubernetes administrators adapting the sandbox for production use
- Teams wanting to customize UI configuration for their environment

## Key Concepts

- **Sandbox Manifests**: Reference Kubernetes YAML files providing working deployment templates
- **ConfigMap Injection**: Runtime configuration mounted into containers without rebuilding images
- **gRPC-Web**: Protocol enabling browser-based gRPC communication through HTTP/1.1
- **Envoy Proxy**: Service mesh proxy handling gRPC-Web translation
- **Template Customization**: Adapting reference manifests for specific infrastructure requirements

## Setup

### Prerequisites
- Kubernetes cluster with Michelangelo API server deployed
- Access to Michelangelo UI container image from GitHub Container Registry

### UI Deployment Requirements

#### 1. Container Deployment
Deploy the UI container with runtime configuration mounted:

**Image Selection:**
Available from GitHub Container Registry at `ghcr.io/michelangelo-ai/ui`:
- **Latest development**: `:main`
- **Specific commit**: `:sha-d550892` (example)
- **Pull command**: `docker pull ghcr.io/michelangelo-ai/ui:your-chosen-tag`

**Container Requirements:**
- **Configuration**: Mount `config.json` at `/usr/share/nginx/html/config.json`
- **Port**: Container serves on port 80

The UI needs to know how to connect to your Michelangelo API server. This is configured through the ConfigMap mounted within the container.

- **API endpoint**: Point to your actual API server location

```json
{
  "apiBaseUrl": "http://your-api-server:8081"
}
```

#### 2. Envoy Proxy Setup
The UI communicates with the API server through Envoy, which requires:

**Port Configuration:**
- Envoy listens on port 8081 for gRPC-Web requests
- API server runs on port 8081 (or your configured port)
- UI makes requests to Envoy, not directly to API server

**CORS Configuration:**
Envoy must allow the UI's origin in CORS settings:
```yaml
cors:
  allow_origin_string_match:
    - exact: "http://your-ui-domain:port"
```

**Backend Routing:**
Envoy forwards UI requests to the API server. The cluster name in the route configuration must match the cluster definition:

```yaml
# In the route configuration:
route:
  cluster: michelangelo-apiserver  # This name must match below

# In the clusters section:
clusters:
  - name: michelangelo-apiserver  # Must match the route cluster name
    endpoints:
      - endpoint:
          address:
            socket_address:
              address: michelangelo-apiserver  # Your API server service name
              port_value: 8081                # Your API server port
```

**Common Issue:** Mismatched cluster names between route configuration and cluster definition will cause "no healthy upstream" errors.

#### 3. Network Connectivity
Ensure network paths are configured:
- UI container → Envoy proxy (port 8081)
- Envoy proxy → API server (port 8081)
- External access → UI container (port 80)

**External Access Details:**
The UI container serves the React application on port 80 (standard HTTP). Users need to be able to reach this port through your Kubernetes networking setup. Common approaches:
- **NodePort**: Direct access via `node-ip:30011` (sandbox default - any cluster node's IP)
- **Ingress**: Domain-based routing (e.g., `michelangelo.yourcompany.com`)
- **Port forwarding**: Development access via `kubectl port-forward`

The UI serves static files (HTML, JS, CSS) from nginx, so it's a standard web server that needs HTTP access from users' browsers.

### Reference Implementation
For a complete working example, see the [Sandbox Guide](../../getting-started/sandbox-setup.md) which includes all components configured and connected.

### Verification
Once deployed, verify the connection chain:
1. **UI loads**: Access the UI in browser
2. **Config loads**: Check browser dev tools for `/config.json`
3. **API connectivity**: Verify API calls succeed (no CORS errors in browser console)

## Troubleshooting

### Common Issues

**Envoy "no healthy upstream" errors:**
- **Cause:** Cluster name mismatch between route configuration and cluster definition
- **Fix:** Ensure the `cluster:` field in routes matches the `name:` field in clusters section
- **Check:** Envoy logs will show "no healthy upstream for cluster 'cluster-name'"

**CORS errors in browser:**
- Verify Envoy proxy CORS configuration includes your domain
- Check browser dev tools → Network tab for failed requests
- Ensure `allow_origin_string_match` includes your access URL

### FAQ

**Q: Can I use a custom domain?**
A: Yes, configure an Ingress resource and update CORS settings in Envoy.

**Q: What if my API server uses different ports?**
A: Update both the `apiBaseUrl` in config.json and the Envoy cluster configuration.
