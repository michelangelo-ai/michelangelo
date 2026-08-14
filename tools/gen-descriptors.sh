#!/usr/bin/env bash
# Build the proto FileDescriptorSet + grpc_json_transcoder services allowlist
# consumed by the Envoy ConfigMap templates, from proto/api.
#
# Shared by gen-grpc-client.sh (writes the committed chart files) and CI
# (writes to a scratch dir and diffs against the committed files, so a proto
# change that isn't followed by a re-run of gen-grpc-client.sh fails the PR
# instead of silently shipping a stale transcoder config).
set -e
set -x

WORKSPACE_ROOT="${WORKSPACE_ROOT:-$(git rev-parse --show-toplevel)}"
OUT_DIR="${1:-${WORKSPACE_ROOT}/helm/michelangelo/files}"

if ! command -v buf &> /dev/null; then
  echo "Buf is NOT installed. Please install it from https://docs.buf.build/installation"
  exit 1
fi

if ! command -v jq &> /dev/null; then
  echo "jq is NOT installed. Please install it from https://jqlang.org/download"
  exit 1
fi

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

mkdir -p "${TMP_DIR}/michelangelo"
cp -r "${WORKSPACE_ROOT}/proto/api" "${TMP_DIR}/michelangelo"

cat << EOF > "${TMP_DIR}/buf.yaml"
version: v2
deps:
  - buf.build/coscene-io/kubernetes-apis
lint:
  use:
    - STANDARD
breaking:
  use:
    - FILE
EOF

buf dep update "${TMP_DIR}"

mkdir -p "${OUT_DIR}"

# FileDescriptorSet so Envoy's grpc_json_transcoder filter can transcode
# JSON<->binary proto without any Go-side jsonpb involvement.
buf build "${TMP_DIR}" --exclude-source-info -o "${OUT_DIR}/descriptors.pb"

# The transcoder's services allowlist, derived from the same descriptor set
# so it can never drift from the services the client is built against.
buf build "${TMP_DIR}" --exclude-source-info -o "${TMP_DIR}/descriptors.json"
jq -r '
  [.file[] | select(.service != null) | .package as $pkg | .service[] | "\($pkg).\(.name)"]
  | sort
' "${TMP_DIR}/descriptors.json" > "${OUT_DIR}/transcoder-services.json"
