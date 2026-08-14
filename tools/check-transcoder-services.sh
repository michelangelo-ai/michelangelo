#!/usr/bin/env bash
# Verify helm/michelangelo/files/transcoder-services.json (the
# grpc_json_transcoder allowlist consumed by the Envoy ConfigMap templates)
# is what tools/gen-descriptors.sh would produce from the current
# proto/api sources right now.
#
# This is a backstop, not the primary mechanism: the normal way this file
# gets regenerated is automatic — `yarn generate` runs
# tools/gen-grpc-client.sh (which calls gen-descriptors.sh) as a `prebuild`
# and `setup` hook in javascript/package.json, so anyone running `yarn
# build` or `yarn setup` after a proto change already regenerates it without
# knowing the file exists. This script exists for the cases that bypass that
# path — a Go-only proto change where nobody ran a JS command, a hand-edited
# generated file, or CI on a PR that never touches javascript/.
#
# Deliberately does NOT diff descriptors.pb byte-for-byte: rebuilding it
# pulls buf.build/coscene-io/kubernetes-apis, an unpinned BSR dependency
# that can resolve to a newer revision between runs with no proto/api change
# on our side, making a raw binary diff flaky. The decoded service list is
# stable across that churn, so only transcoder-services.json is compared.
set -e

WORKSPACE_ROOT="${WORKSPACE_ROOT:-$(git rev-parse --show-toplevel)}"
COMMITTED_FILE="${WORKSPACE_ROOT}/helm/michelangelo/files/transcoder-services.json"

if ! command -v jq &> /dev/null; then
  echo "jq is NOT installed. Please install it from https://jqlang.org/download"
  exit 1
fi

REGEN_DIR=$(mktemp -d)
trap 'rm -rf "$REGEN_DIR"' EXIT

"${WORKSPACE_ROOT}/tools/gen-descriptors.sh" "${REGEN_DIR}"

if ! diff -u \
  <(jq -S . "${COMMITTED_FILE}") \
  <(jq -S . "${REGEN_DIR}/transcoder-services.json")
then
  echo "" >&2
  echo "helm/michelangelo/files/transcoder-services.json (< above, committed)" >&2
  echo "does not match what tools/gen-descriptors.sh produces from the" >&2
  echo "current proto/api sources (> above, freshly generated)." >&2
  echo "Run tools/gen-grpc-client.sh (or 'yarn generate' in javascript/) and" >&2
  echo "commit the result." >&2
  exit 1
fi

echo "transcoder-services.json matches the current proto/api sources."
