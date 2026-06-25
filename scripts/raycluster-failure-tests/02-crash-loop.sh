#!/usr/bin/env bash
# Test 2: CrashLoopBackOff
# Uses a valid image but overrides the command to exit immediately with error.
# After a few restarts, Kubernetes marks the pod as CrashLoopBackOff.
#
# Expected:
#   KubeRay condition: HeadPodReady=False, Reason=CrashLoopBackOff
#   Michelangelo state: RAY_CLUSTER_STATE_FAILED
#   PodErrors: reason=CrashLoopBackOff
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/_helpers.sh"

CLUSTER_NAME="test-crash-loop"

echo "============================================"
echo "TEST 2: CrashLoopBackOff"
echo "============================================"
echo ""
echo "Creating RayCluster with a command that exits immediately..."
echo "This will cause repeated restarts -> CrashLoopBackOff."
echo "Note: Takes ~2-3 minutes for CrashLoopBackOff to trigger."
echo ""

create_ray_cluster \
  "$CLUSTER_NAME" \
  "docker.io/library/examples:latest" \
  "docker.io/library/examples:latest" \
  "2Gi" \
  "2Gi" \
  '"/bin/sh", "-c", "echo crashing && exit 1"'
