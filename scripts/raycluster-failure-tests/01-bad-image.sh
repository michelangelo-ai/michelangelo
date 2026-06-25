#!/usr/bin/env bash
# Test 1: ImagePullBackOff
# Uses a non-existent image to trigger ErrImagePull / ImagePullBackOff.
#
# Expected:
#   KubeRay condition: HeadPodReady=False, Reason=ImagePullBackOff
#   Michelangelo state: RAY_CLUSTER_STATE_FAILED
#   PodErrors: reason=ImagePullBackOff or ErrImagePull
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/_helpers.sh"

CLUSTER_NAME="test-bad-image"

echo "============================================"
echo "TEST 1: ImagePullBackOff"
echo "============================================"
echo ""
echo "Creating RayCluster with non-existent image..."
echo "Head image:   this-image-does-not-exist:v999"
echo "Worker image: this-image-does-not-exist:v999"
echo ""

create_ray_cluster \
  "$CLUSTER_NAME" \
  "this-image-does-not-exist:v999" \
  "this-image-does-not-exist:v999"
