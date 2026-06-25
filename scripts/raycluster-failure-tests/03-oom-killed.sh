#!/usr/bin/env bash
# Test 3: OOMKilled
# Requests a tiny memory limit and runs a process that allocates beyond it.
# Kubernetes kills the container with OOMKilled.
#
# Expected:
#   KubeRay condition: HeadPodReady=False, Reason=CrashLoopBackOff (after OOM restart)
#   Michelangelo state: RAY_CLUSTER_STATE_FAILED
#   PodErrors: reason=CrashLoopBackOff or OOMKilled
#
# Note: KubeRay reports OOM as CrashLoopBackOff on the condition since the pod
# restarts after being OOM-killed. The underlying termination reason (OOMKilled)
# is visible on the pod's container status but not always on the RayCluster condition.
# Our code handles both reasons as terminal.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/_helpers.sh"

CLUSTER_NAME="test-oom-killed"

echo "============================================"
echo "TEST 3: OOMKilled"
echo "============================================"
echo ""
echo "Creating RayCluster with 64Mi memory limit and a memory hog command..."
echo "Note: Takes ~2-3 minutes for OOM + CrashLoopBackOff cycle."
echo ""

create_ray_cluster \
  "$CLUSTER_NAME" \
  "docker.io/library/examples:latest" \
  "docker.io/library/examples:latest" \
  "64Mi" \
  "2Gi" \
  '"/bin/sh", "-c", "python3 -c \"x = bytearray(256 * 1024 * 1024)\""'
