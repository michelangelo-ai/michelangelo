#!/usr/bin/env bash
# Build the proto FileDescriptorSet + grpc_json_transcoder services allowlist
# consumed by the Envoy ConfigMap templates, from proto/api.
#
# The allowlist is deliberately NOT "every service that compiles" — Envoy's
# grpc_json_transcoder exposes whatever is on it over plain JSON/HTTP, so an
# internal-only Go service would become web-reachable the moment its proto
# compiles if we did that. It's the intersection of:
#   - the services javascript/packages/rpc/services.ts actually instantiates
#     (i.e. what the browser client can call), and
#   - the compiled descriptor set (used only to resolve each of those names
#     to its fully-qualified proto service name).
# A new Go-only service does not appear here until something in services.ts
# starts referencing it.
#
# Shared by gen-grpc-client.sh (writes the committed chart files) and CI
# (writes to a scratch dir and diffs against the committed files, so a
# services.ts change that isn't followed by a re-run of gen-grpc-client.sh
# fails the PR instead of silently shipping a stale transcoder config).
set -e
set -x

WORKSPACE_ROOT="${WORKSPACE_ROOT:-$(git rev-parse --show-toplevel)}"
OUT_DIR="${1:-${WORKSPACE_ROOT}/helm/michelangelo/files}"
SERVICES_TS="${WORKSPACE_ROOT}/javascript/packages/rpc/services.ts"

if ! command -v buf &> /dev/null; then
  echo "Buf is NOT installed. Please install it from https://docs.buf.build/installation"
  exit 1
fi

if ! command -v jq &> /dev/null; then
  echo "jq is NOT installed. Please install it from https://jqlang.org/download"
  exit 1
fi

if [ ! -f "${SERVICES_TS}" ]; then
  echo "${SERVICES_TS} not found — can't determine which services the JS client references."
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
buf build "${TMP_DIR}" --exclude-source-info -o "${TMP_DIR}/descriptors.json"

# Service identifiers services.ts imports from generated `*_svc_pb` modules,
# e.g. `import { DeploymentService } from './gen/michelangelo/api/v2/deployment_svc_pb';`
grep -oE "^import \{ [A-Za-z_][A-Za-z0-9_]* \} from '\./gen/[^']+_svc_pb';" "${SERVICES_TS}" \
  | sed -E "s/^import \{ ([A-Za-z_][A-Za-z0-9_]*) \}.*/\1/" \
  | sort -u > "${TMP_DIR}/referenced-services.txt"

if [ ! -s "${TMP_DIR}/referenced-services.txt" ]; then
  echo "No '*Service' imports found in ${SERVICES_TS} — refusing to write an empty allowlist."
  echo "(If services.ts genuinely imports no services, this check needs updating.)"
  exit 1
fi

REFERENCED_JSON=$(jq -R -s -c 'split("\n") | map(select(length > 0))' "${TMP_DIR}/referenced-services.txt")

# Resolve each referenced name to its fully-qualified proto service name.
jq -r --argjson refs "${REFERENCED_JSON}" '
  [.file[] | select(.service != null) | .package as $pkg | .service[]
    | select(.name as $n | $refs | index($n) != null) | "\($pkg).\(.name)"]
  | sort
' "${TMP_DIR}/descriptors.json" > "${OUT_DIR}/transcoder-services.json"

# Fail loudly if an import in services.ts didn't resolve to any proto
# service — almost certainly a typo/rename, not an intentional omission.
RESOLVED_COUNT=$(jq 'length' "${OUT_DIR}/transcoder-services.json")
REFERENCED_COUNT=$(wc -l < "${TMP_DIR}/referenced-services.txt" | tr -d ' ')
if [ "${RESOLVED_COUNT}" -ne "${REFERENCED_COUNT}" ]; then
  echo "services.ts references ${REFERENCED_COUNT} service(s) but only ${RESOLVED_COUNT}" \
    "resolved against the compiled descriptor set:"
  comm -23 "${TMP_DIR}/referenced-services.txt" <(jq -r '.[] | split(".") | last' "${OUT_DIR}/transcoder-services.json" | sort)
  exit 1
fi
