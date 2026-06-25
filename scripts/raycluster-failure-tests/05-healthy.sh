#!/usr/bin/env bash
# Test 5: Healthy cluster (golden path)
# Creates a normal RayCluster that should reach READY state.
#
# Expected:
#   KubeRay condition: HeadPodReady=True, RayClusterProvisioned=True
#   Michelangelo state: RAY_CLUSTER_STATE_READY
#   PodErrors: (none)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/_helpers.sh"

CLUSTER_NAME="test-healthy"

echo "============================================"
echo "TEST 5: Healthy cluster (golden path)"
echo "============================================"
echo ""
echo "Creating a normal RayCluster with valid image..."
echo "Should reach READY within ~30-60s."
echo ""

create_ray_cluster \
  "$CLUSTER_NAME" \
  "docker.io/library/examples:latest" \
  "docker.io/library/examples:latest"
