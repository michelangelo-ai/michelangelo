#!/usr/bin/env bash
# Test 4: RunContainerError / Bad entrypoint
# Uses a command that references a binary that doesn't exist in the image.
# Kubernetes reports RunContainerError or CrashLoopBackOff.
#
# Expected:
#   KubeRay condition: HeadPodReady=False, Reason=CrashLoopBackOff or RunContainerError
#   Michelangelo state: RAY_CLUSTER_STATE_FAILED
#   PodErrors: reason=CrashLoopBackOff or RunContainerError
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/_helpers.sh"

CLUSTER_NAME="test-bad-command"

echo "============================================"
echo "TEST 4: RunContainerError (bad entrypoint)"
echo "============================================"
echo ""
echo "Creating RayCluster with a non-existent binary as entrypoint..."
echo ""

create_ray_cluster \
  "$CLUSTER_NAME" \
  "docker.io/library/examples:latest" \
  "docker.io/library/examples:latest" \
  "2Gi" \
  "2Gi" \
  '"/this/binary/does/not/exist"'
