#!/usr/bin/env bash
# Verifies helm/michelangelo/files/transcoder-services.json and
# descriptors.pb match what tools/gen-descriptors.sh produces right now.
#
# Narrow backstop, not the primary enforcement: proto/go-triggered
# staleness is already caught unconditionally by main.yml's dirty-check
# job. This script exists because that job skips javascript/**-only
# changes, so a services.ts edit with no proto change needs its own check.
set -e

WORKSPACE_ROOT="${WORKSPACE_ROOT:-$(git rev-parse --show-toplevel)}"
COMMITTED_DESCRIPTORS="${WORKSPACE_ROOT}/helm/michelangelo/files/descriptors.pb"
COMMITTED_SERVICES="${WORKSPACE_ROOT}/helm/michelangelo/files/transcoder-services.json"

if ! command -v jq &> /dev/null; then
  echo "jq is NOT installed. Please install it from https://jqlang.org/download"
  exit 1
fi

REGEN_DIR=$(mktemp -d)
trap 'rm -rf "$REGEN_DIR"' EXIT

"${WORKSPACE_ROOT}/tools/gen-descriptors.sh" "${REGEN_DIR}"

FAILED=0

if ! diff -u \
  <(jq -S . "${COMMITTED_SERVICES}") \
  <(jq -S . "${REGEN_DIR}/transcoder-services.json")
then
  echo "" >&2
  echo "helm/michelangelo/files/transcoder-services.json (< above, committed)" >&2
  echo "does not match the services javascript/packages/rpc/services.ts" >&2
  echo "currently references (> above, freshly generated)." >&2
  FAILED=1
fi

if ! cmp -s "${COMMITTED_DESCRIPTORS}" "${REGEN_DIR}/descriptors.pb"; then
  echo "" >&2
  echo "helm/michelangelo/files/descriptors.pb does not match what" >&2
  echo "'bazel build //proto/api/v2:v2_proto' produces right now." >&2
  FAILED=1
fi

if [ "${FAILED}" -ne 0 ]; then
  echo "" >&2
  echo "Run tools/gen-descriptors.sh and commit the result." >&2
  exit 1
fi

echo "descriptors.pb and transcoder-services.json are up to date."
