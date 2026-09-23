#!/usr/bin/env bash
# Builds the grpc_json_transcoder services allowlist consumed by the Envoy
# ConfigMap template.
#
# transcoder-services.json is deliberately NOT "every service that
# compiles": Envoy's grpc_json_transcoder exposes whatever is on it over
# plain JSON/HTTP, so it's scoped to exactly what
# javascript/packages/rpc/services.ts imports.
#
# Each import's fully-qualified proto service name is derived directly from
# its import path rather than resolved against a compiled descriptor set:
# gen-grpc-client.sh's generated directory layout mirrors the proto package
# by construction (buf's PACKAGE_DIRECTORY_MATCH convention), so
# './gen/michelangelo/api/v2/deployment_svc_pb' importing `DeploymentService`
# means the service lives at package `michelangelo.api.v2`, giving the FQN
# `michelangelo.api.v2.DeploymentService`. This needs neither buf nor
# descriptors.pb. If that convention is ever violated, the wrong FQN doesn't
# fail silently — Envoy validates each entry against descriptors.pb at
# startup and refuses to come up.
set -e
set -x

WORKSPACE_ROOT="${WORKSPACE_ROOT:-$(git rev-parse --show-toplevel)}"
OUT_DIR="${1:-${WORKSPACE_ROOT}/helm/michelangelo/files}"
SERVICES_TS="${WORKSPACE_ROOT}/javascript/packages/rpc/services.ts"

if ! command -v jq &> /dev/null; then
  echo "jq is NOT installed. Please install it from https://jqlang.org/download"
  exit 1
fi

if [ ! -f "${SERVICES_TS}" ]; then
  echo "${SERVICES_TS} not found — can't determine which services the JS client references."
  exit 1
fi

mkdir -p "${OUT_DIR}"

# Each import services.ts pulls from a generated `*_svc_pb` module carries
# both the service name and its package path, e.g.
# `import { DeploymentService } from './gen/michelangelo/api/v2/deployment_svc_pb';`
# resolves to `michelangelo.api.v2.DeploymentService`.
FQNS=$(grep -oE "^import \{ [A-Za-z_][A-Za-z0-9_]* \} from '\./gen/[^']+_svc_pb';" "${SERVICES_TS}" \
  | sed -E "s|^import \{ ([A-Za-z_][A-Za-z0-9_]*) \} from '\./gen/(.+)/[^/]+_svc_pb';|\2.\1|" \
  | tr '/' '.' \
  | sort -u)

if [ -z "${FQNS}" ]; then
  echo "No '*Service' imports found in ${SERVICES_TS} — refusing to write an empty allowlist."
  echo "(If services.ts genuinely imports no services, this check needs updating.)"
  exit 1
fi

jq -R -s 'split("\n") | map(select(length > 0)) | sort' <<< "${FQNS}" \
  > "${OUT_DIR}/transcoder-services.json"
