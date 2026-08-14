#!/usr/bin/env bash
# Verify helm/michelangelo/files/transcoder-services.json and descriptors.pb
# are what tools/gen-descriptors.sh would produce right now.
#
# transcoder-services.json is the services javascript/packages/rpc/services.ts
# actually references, resolved to fully-qualified names via the compiled
# proto descriptor set. descriptors.pb is that descriptor set itself, built
# via `bazel build //proto/api/v2:v2_proto` (deterministic — no BSR/network
# dependency, confirmed by running gen-descriptors.sh twice back-to-back and
# diffing byte-for-byte).
#
# This is a narrow backstop, not the primary mechanism for either file:
# - descriptors.pb regenerates unconditionally on any proto/go change via
#   the "Check transcoder descriptor artifacts are up to date" step in
#   main.yml's dirty-check job — that's the real enforcement for proto-side
#   staleness, and doesn't depend on this script at all.
# - This script/workflow exists specifically because main.yml's dirty-check
#   job doesn't run on javascript/**-only changes (main.yml ignores that
#   path), so a services.ts edit with no accompanying proto/go change would
#   otherwise slip through uncaught. tools/gen-grpc-client.sh (the
#   yarn-triggered script) deliberately does NOT regenerate these files
#   anymore — it only generates JS/Python client language bindings — so
#   there is no automatic local trigger for a services.ts-only change; this
#   CI check and a manual `tools/gen-descriptors.sh` run are how it's caught.
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
