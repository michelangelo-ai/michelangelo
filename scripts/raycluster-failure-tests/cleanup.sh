#!/usr/bin/env bash
# Usage: ./cleanup.sh <cluster-name>
# Deletes a Michelangelo RayCluster and its KubeRay counterpart.
set -euo pipefail

CLUSTER_NAME="${1:?Usage: $0 <cluster-name>}"
API="127.0.0.1:15566"

echo "Deleting Michelangelo RayCluster '${CLUSTER_NAME}'..."
grpcurl -plaintext -max-time 10 \
  -H 'rpc-caller: grpcurl-test' \
  -H 'rpc-service: ma-apiserver' \
  -H 'rpc-encoding: proto' \
  -d "{\"name\": \"${CLUSTER_NAME}\", \"namespace\": \"default\"}" \
  "$API" michelangelo.api.v2.RayClusterService/DeleteRayCluster 2>&1 || true

echo "Deleting KubeRay RayCluster (if orphaned)..."
kubectl delete raycluster.ray.io "${CLUSTER_NAME}" -n default --ignore-not-found 2>/dev/null || true

echo "Done."
