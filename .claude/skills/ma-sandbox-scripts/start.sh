#!/usr/bin/env bash
# Resume a stopped sandbox cluster (containers exited, Docker up) without
# touching its state. No-op if the cluster is already running.
# Exit codes: 0 resumed/running, 1 cluster does not exist (needs `ma sandbox create`).

set -euo pipefail

CLUSTER=michelangelo-sandbox

if ! k3d cluster list --no-headers 2>/dev/null | awk '{print $1}' | grep -qx "$CLUSTER"; then
  echo "Cluster $CLUSTER does not exist — create it with 'ma sandbox create' (see /ma-sandbox-setup)." >&2
  exit 1
fi

SERVERS=$(k3d cluster list --no-headers | awk -v c="$CLUSTER" '$1==c {print $2}')
if [ "${SERVERS%%/*}" = "0" ]; then
  echo "Starting $CLUSTER..."
  k3d cluster start "$CLUSTER"
else
  echo "$CLUSTER is already running."
fi

kubectl wait --for=condition=ready node --all --timeout=120s
