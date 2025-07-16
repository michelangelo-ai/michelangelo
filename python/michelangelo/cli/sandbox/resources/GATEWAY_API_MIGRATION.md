# Gateway API Migration Guide

This guide explains the migration from Istio-specific networking resources to the generic Kubernetes Gateway API.

## 🎯 What Changed

### Before (Istio-specific)
```yaml
# Vendor lock-in with Istio
apiVersion: networking.istio.io/v1
kind: Gateway

apiVersion: networking.istio.io/v1beta1  
kind: VirtualService
```

### After (Generic Gateway API)
```yaml
# Vendor-neutral, works with any Gateway implementation
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway

apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
```

## 🚀 Quick Start

### 1. Deploy Gateway API
```bash
# Set your kubeconfig if needed
export KUBECONFIG=/path/to/your/kubeconfig.yaml

# Run the migration script
./deploy-gateway-api.sh
```

### 2. Test the endpoints
```bash
# Get gateway IP
GATEWAY_IP=$(kubectl get svc istio-ingressgateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# Test Triton health endpoint
curl http://$GATEWAY_IP:8888/v2/health

# Test model endpoints  
curl http://$GATEWAY_IP:8888/v2/models
```

## 📋 File Structure

### New Files
- `gateway-api-setup.yaml` - Generic Gateway and GatewayClass
- `bert-cola-httproute.yaml` - HTTPRoute replacing VirtualService
- `deploy-gateway-api.sh` - Automated deployment script
- `GATEWAY_API_MIGRATION.md` - This guide

### Updated Files  
- `istio-gateway.yaml` - Marked as legacy, includes port 8888
- `bert-cola-virtual-service.yaml` - Marked as legacy

## 🌐 Port Configuration

The new Gateway exposes multiple ports:

| Port | Purpose | Listener Name |
|------|---------|---------------|
| 80   | Standard HTTP | `http` |
| 8888 | Triton Inference | `triton-http` |
| 8088 | Cadence Web UI | `cadence-web` |
| 8081 | Envoy Admin | `envoy-admin` |

## 🔀 Traffic Splitting Examples

### Basic Traffic Split
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
spec:
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /v2/models
    backendRefs:
    - name: triton-stable
      weight: 80  # 80% traffic
    - name: triton-canary  
      weight: 20  # 20% traffic
```

### Header-based Routing
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
spec:
  rules:
  # Premium users → dedicated service
  - matches:
    - headers:
      - name: "X-User-Tier"
        value: "premium"
    backendRefs:
    - name: triton-premium-service
  
  # Canary users → experimental version  
  - matches:
    - headers:
      - name: "X-Canary-User"
        value: "true"
    backendRefs:
    - name: triton-canary-service
```

## 🔧 Advanced Features

### URL Rewriting
```yaml
filters:
- type: URLRewrite
  urlRewrite:
    path:
      type: ReplacePrefixMatch
      replacePrefixMatch: /v2/models/bert-cola-13
```

### Request Headers
```yaml
filters:
- type: RequestHeaderModifier
  requestHeaderModifier:
    add:
    - name: "X-Custom-Header"
      value: "michelangelo-sandbox"
```

### Response Headers
```yaml
filters:
- type: ResponseHeaderModifier
  responseHeaderModifier:
    add:
    - name: "X-Served-By"
      value: "triton-inference-server"
```

## 🔍 Troubleshooting

### Check Gateway Status
```bash
kubectl describe gateway ma-gateway -n default
```

### Check HTTPRoute Status  
```bash
kubectl describe httproute -n default
```

### View Gateway Events
```bash
kubectl get events --field-selector involvedObject.kind=Gateway
```

### Check Istio Gateway Controller
```bash
kubectl logs -n istio-system -l app=istiod
```

## 🎛️ Migration Strategy

### Phase 1: Parallel Deployment
- Deploy new Gateway API resources alongside existing Istio resources
- Test functionality with both systems
- Verify traffic routing works correctly

### Phase 2: Traffic Validation
- Compare metrics between old and new configurations
- Validate all endpoints are accessible
- Test traffic splitting if needed

### Phase 3: Cleanup
- Remove legacy Istio Gateway and VirtualService resources
- Update documentation and deployment scripts
- Train team on new HTTPRoute syntax

## 🔗 Integration with Michelangelo

### InferenceServer CRD
The Michelangelo `InferenceServer` CRD automatically creates:
- Kubernetes `Service` 
- `VirtualService` (legacy) 
- `HTTPRoute` (new)

### Controller Manager
The controller manager supports both:
- Legacy Istio VirtualService creation
- New HTTPRoute generation (with feature flag)

### Service Discovery
Services remain the same:
- `test-inference-server-service`
- `bert-cola-deployment-service`  
- Standard Kubernetes service discovery

## 📊 Benefits

### ✅ Vendor Neutrality
- Not locked into Istio
- Can migrate to other Gateway implementations
- Future-proof against vendor changes

### ✅ Standardization  
- Official Kubernetes Gateway API
- Consistent across different environments
- Better tooling and IDE support

### ✅ Advanced Traffic Management
- Built-in traffic splitting
- Header-based routing
- URL rewriting and header modification

### ✅ Better Integration
- Native Kubernetes RBAC
- Standard resource patterns
- Improved observability

## 🔮 Next Steps

1. **Enable HTTPRoute in Controller Manager**: Update the controller to generate HTTPRoute alongside VirtualService
2. **Add Traffic Splitting**: Implement canary deployments using weight-based routing  
3. **Header-based Routing**: Route premium/experimental traffic to dedicated services
4. **Cross-namespace Routing**: Use ReferenceGrants for multi-namespace deployments
5. **TLS Configuration**: Add HTTPS support with certificate management

## 📚 References

- [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/)
- [Istio Gateway API Support](https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/)
- [HTTPRoute Specification](https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.HTTPRoute)
- [Traffic Splitting Guide](https://gateway-api.sigs.k8s.io/guides/traffic-splitting/)