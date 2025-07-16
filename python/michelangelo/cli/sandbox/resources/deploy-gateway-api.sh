#!/bin/bash
set -e

# Michelangelo Sandbox - Gateway API Migration Script
# This script migrates from Istio-specific resources to generic Kubernetes Gateway API

echo "🚀 Starting Gateway API migration for Michelangelo Sandbox..."

# Check if kubectl is configured
if ! kubectl cluster-info &> /dev/null; then
    echo "❌ kubectl is not configured or cluster is not accessible"
    exit 1
fi

# Set kubeconfig if provided
if [ ! -z "$KUBECONFIG_PATH" ]; then
    export KUBECONFIG="$KUBECONFIG_PATH"
    echo "📁 Using kubeconfig: $KUBECONFIG_PATH"
fi

echo "📦 Installing Kubernetes Gateway API CRDs..."
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.1.0/standard-install.yaml

echo "⏳ Waiting for Gateway API CRDs to be ready..."
kubectl wait --for condition=established --timeout=60s crd/gatewayclasses.gateway.networking.k8s.io
kubectl wait --for condition=established --timeout=60s crd/gateways.gateway.networking.k8s.io
kubectl wait --for condition=established --timeout=60s crd/httproutes.gateway.networking.k8s.io

echo "🔧 Deploying Gateway API setup..."
kubectl apply -f gateway-api-setup.yaml

echo "⏳ Waiting for Gateway to be ready..."
kubectl wait --for=condition=Programmed --timeout=300s gateway/ma-gateway -n default

echo "🛣️  Deploying HTTPRoutes..."
kubectl apply -f bert-cola-httproute.yaml

echo "✅ Verifying deployment..."
echo "📊 Gateway status:"
kubectl get gateway ma-gateway -n default -o wide

echo "📊 HTTPRoute status:"
kubectl get httproute -n default -o wide

echo "📊 Gateway API resources:"
kubectl get gatewayclasses,gateways,httproutes -A

echo "🔍 Testing gateway connectivity..."
GATEWAY_IP=$(kubectl get svc istio-ingressgateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
if [ -z "$GATEWAY_IP" ]; then
    GATEWAY_IP=$(kubectl get svc istio-ingressgateway -n istio-system -o jsonpath='{.spec.clusterIP}')
fi

echo "🌐 Gateway IP: $GATEWAY_IP"

# Test endpoints if gateway IP is available
if [ ! -z "$GATEWAY_IP" ]; then
    echo "🧪 Testing health endpoint..."
    if curl -s --max-time 5 "http://$GATEWAY_IP:8888/v2/health" > /dev/null; then
        echo "✅ Health endpoint is accessible"
    else
        echo "⚠️  Health endpoint test failed (this is expected if services aren't running)"
    fi
else
    echo "⚠️  Could not determine gateway IP, skipping connectivity tests"
fi

echo ""
echo "🎉 Gateway API migration completed successfully!"
echo ""
echo "📋 Next steps:"
echo "1. Test your applications with the new HTTPRoute endpoints"
echo "2. Update any hardcoded references from VirtualService to HTTPRoute"
echo "3. Remove legacy Istio Gateway/VirtualService resources when ready"
echo ""
echo "🔗 Available endpoints:"
echo "  - HTTP: http://$GATEWAY_IP:80"
echo "  - Triton: http://$GATEWAY_IP:8888"
echo "  - Health: http://$GATEWAY_IP:8888/v2/health"
echo ""
echo "📚 For traffic splitting examples, see bert-cola-httproute.yaml"