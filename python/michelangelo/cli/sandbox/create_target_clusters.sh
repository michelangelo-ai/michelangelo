#!/bin/bash
set -e

echo "Creating cluster-1 with non-overlapping CIDRs..."
k3d cluster create cluster-1 \
  --network k3d-michelangelo-sandbox \
  --k3s-arg "--cluster-cidr=10.44.0.0/16@server:*" \
  --k3s-arg "--service-cidr=10.45.0.0/16@server:*"

echo "Creating cluster-2 with non-overlapping CIDRs..."
k3d cluster create cluster-2 \
  --network k3d-michelangelo-sandbox \
  --k3s-arg "--cluster-cidr=10.46.0.0/16@server:*" \
  --k3s-arg "--service-cidr=10.47.0.0/16@server:*"

echo "✅ Target clusters created successfully!"
echo ""
echo "Cluster CIDRs:"
echo "  Control plane: Pod=10.42.0.0/16, Service=10.43.0.0/16"
echo "  cluster-1:     Pod=10.44.0.0/16, Service=10.45.0.0/16"
echo "  cluster-2:     Pod=10.46.0.0/16, Service=10.47.0.0/16"
