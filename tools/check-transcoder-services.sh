#!/usr/bin/env bash
# Verify helm/michelangelo/files/transcoder-services.json (the
# grpc_json_transcoder allowlist consumed by the Envoy ConfigMap templates)
# lists exactly the services declared under proto/api.
#
# This is a lightweight, network-free consistency check — it parses
# `package`/`service` declarations directly out of the .proto files instead
# of invoking buf, so it can run in any CI job without buf toolchain setup
# or a dependency fetch. It exists to catch the case where proto/api changes
# (a service added, removed, or renamed) without a corresponding re-run of
# tools/gen-grpc-client.sh, which would otherwise ship a stale transcoder
# allowlist silently.
set -e

WORKSPACE_ROOT="${WORKSPACE_ROOT:-$(git rev-parse --show-toplevel)}"
SERVICES_FILE="${WORKSPACE_ROOT}/helm/michelangelo/files/transcoder-services.json"

if ! command -v jq &> /dev/null; then
  echo "jq is NOT installed. Please install it from https://jqlang.org/download"
  exit 1
fi

EXPECTED=$(mktemp)
ACTUAL=$(mktemp)
trap 'rm -f "$EXPECTED" "$ACTUAL"' EXIT

for proto_file in $(find "${WORKSPACE_ROOT}/proto/api" -name '*.proto' | sort); do
  package=$(sed -n 's/^package[[:space:]]\{1,\}\([a-zA-Z0-9_.]*\);.*/\1/p' "${proto_file}" | head -n1)
  [ -z "${package}" ] && continue
  sed -n 's/^service[[:space:]]\{1,\}\([a-zA-Z0-9_]*\).*/\1/p' "${proto_file}" | while read -r service; do
    echo "${package}.${service}"
  done
done | sort > "${EXPECTED}"

jq -r '.[]' "${SERVICES_FILE}" | sort > "${ACTUAL}"

if ! diff -u "${ACTUAL}" "${EXPECTED}"; then
  echo "" >&2
  echo "helm/michelangelo/files/transcoder-services.json (< above) is out of" >&2
  echo "sync with the services declared under proto/api (> above)." >&2
  echo "Run tools/gen-grpc-client.sh and commit the result." >&2
  exit 1
fi

echo "transcoder-services.json matches the services declared under proto/api."
