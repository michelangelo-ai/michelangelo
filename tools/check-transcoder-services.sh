#!/usr/bin/env bash
# Verifies helm/michelangelo/files/transcoder-services.json matches what
# tools/gen-transcoder-services.sh produces right now.
#
# Narrow backstop: main.yml's dirty-check job runs gen-transcoder-services.sh
# unconditionally on proto/go changes, but skips javascript/**-only
# changes — this script exists to catch a services.ts edit with no proto
# change, which that job would otherwise miss.
set -e

WORKSPACE_ROOT="${WORKSPACE_ROOT:-$(git rev-parse --show-toplevel)}"
COMMITTED_SERVICES="${WORKSPACE_ROOT}/helm/michelangelo/files/transcoder-services.json"

if ! command -v jq &> /dev/null; then
  echo "jq is NOT installed. Please install it from https://jqlang.org/download"
  exit 1
fi

REGEN_DIR=$(mktemp -d)
trap 'rm -rf "$REGEN_DIR"' EXIT

"${WORKSPACE_ROOT}/tools/gen-transcoder-services.sh" "${REGEN_DIR}"

if ! diff -u \
  <(jq -S . "${COMMITTED_SERVICES}") \
  <(jq -S . "${REGEN_DIR}/transcoder-services.json")
then
  echo "" >&2
  echo "helm/michelangelo/files/transcoder-services.json (< above, committed)" >&2
  echo "does not match the services javascript/packages/rpc/services.ts" >&2
  echo "currently references (> above, freshly generated)." >&2
  echo "" >&2
  echo "Run tools/gen-transcoder-services.sh and commit the result." >&2
  exit 1
fi

echo "transcoder-services.json is up to date."
