#!/usr/bin/env bash
# Usage: ./watch-kuberay.sh <cluster-name>
# Watches the KubeRay RayCluster conditions and state in the compute cluster.
set -euo pipefail

CLUSTER_NAME="${1:?Usage: $0 <cluster-name>}"

echo "Watching KubeRay RayCluster '${CLUSTER_NAME}' in namespace 'default'..."
echo "Press Ctrl-C to stop."
echo ""

while true; do
  echo "=== $(date '+%H:%M:%S') ==="

  # State + Reason
  kubectl get raycluster.ray.io "${CLUSTER_NAME}" -n default \
    -o jsonpath='{.status.state}' 2>/dev/null && echo "" || echo "(not found yet)"

  # Conditions
  kubectl get raycluster.ray.io "${CLUSTER_NAME}" -n default \
    -o jsonpath='{range .status.conditions[*]}  {.type}: {.status} | {.reason} | {.message}{"\n"}{end}' 2>/dev/null || true

  # Pods
  echo "  Pods:"
  kubectl get pods -n default -l "ray.io/cluster=${CLUSTER_NAME}" \
    --no-headers -o custom-columns='NAME:.metadata.name,STATUS:.status.phase,REASON:.status.reason,RESTARTS:.status.containerStatuses[0].restartCount' 2>/dev/null || echo "    (none)"

  echo ""
  sleep 3
done
