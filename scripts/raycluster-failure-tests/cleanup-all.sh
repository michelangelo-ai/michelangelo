#!/usr/bin/env bash
# Cleans up all failure-test RayClusters.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

for name in \
  test-bad-image \
  test-crash-loop \
  test-oom-killed \
  test-bad-command \
  test-healthy; do
  echo "--- Cleaning up ${name} ---"
  "$SCRIPT_DIR/cleanup.sh" "$name" || true
done

echo ""
echo "All test clusters cleaned up."
