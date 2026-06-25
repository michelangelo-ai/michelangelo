#!/usr/bin/env bash
# Usage: ./get-status.sh <cluster-name> [--watch]
# Fetches the Michelangelo RayCluster status via gRPC.
# With --watch, polls every 5s until state is READY or FAILED.
set -euo pipefail

CLUSTER_NAME="${1:?Usage: $0 <cluster-name> [--watch]}"
WATCH="${2:-}"
API="127.0.0.1:15566"

fetch_status() {
  grpcurl -plaintext -max-time 10 \
    -H 'rpc-caller: grpcurl-test' \
    -H 'rpc-service: ma-apiserver' \
    -H 'rpc-encoding: proto' \
    -d "{\"name\": \"${CLUSTER_NAME}\", \"namespace\": \"default\"}" \
    "$API" michelangelo.api.v2.RayClusterService/GetRayCluster
}

if [ "$WATCH" = "--watch" ]; then
  echo "Polling ${CLUSTER_NAME} every 5s (Ctrl-C to stop)..."
  while true; do
    echo "=== $(date '+%H:%M:%S') ==="
    OUTPUT=$(fetch_status 2>&1) || true
    # Print state, podErrors, and conditions
    echo "$OUTPUT" | grep -E '"state"|"podErrors"|"statusConditions"|"reason"|"message"|"status"' || echo "$OUTPUT" | head -5
    echo ""

    # Stop if terminal
    if echo "$OUTPUT" | grep -q '"state": "RAY_CLUSTER_STATE_FAILED"\|"state": "RAY_CLUSTER_STATE_READY"\|"state": "RAY_CLUSTER_STATE_TERMINATED"'; then
      echo "Terminal state reached."
      echo ""
      echo "Full status:"
      echo "$OUTPUT"
      break
    fi
    sleep 5
  done
else
  fetch_status
fi
