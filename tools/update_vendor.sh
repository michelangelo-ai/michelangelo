#!/usr/bin/env bash

# update_vendor.sh
#
# This script ensures vendored Go code is updated for IDEs and development tools to use:
# 1. Deletes go/vendor directory, if it exists.
# 2. Runs `go mod vendor` to create a new go/vendor directory.
# 3. Generates go code for all protobuf files via `bazel build //proto/...`.
# 4. Copies generated .go files from Bazel output into the correct paths under go/vendor.

set -euo pipefail

GO_DIR="${WORKSPACE_ROOT}/go"
VENDOR_DIR="${GO_DIR}/vendor"

IMPORT_BASE="github.com/michelangelo-ai/michelangelo"

echo "==> Generating go/vendor directory..."
(cd "${GO_DIR}" && go mod vendor)

echo "==> Querying Bazel go_proto_library targets..."
TARGETS=$(bazel query 'kind("go_proto_library", //proto/...)')

if [ -z "$TARGETS" ]; then
    echo "    No go_proto_library targets found under //proto/"
    exit 1
fi
echo "$TARGETS" | sed 's/^/      - /'

echo "==> Copying generated .go files to vendor/..."

for target in $TARGETS; do
    # Parse label: //proto/api:api_go_proto -> proto/api api_go_proto
    label="${target#//}"                     # proto/api:api_go_proto
    pkg="${label%%:*}"                      # proto/api
    name="${label##*:}"                     # api_go_proto

    # Derive output path based on Bazel's convention:
    #   bazel-bin/proto/api/api_go_proto/<importpath>/
    OUT_PATH="bazel-bin/${pkg}/${name}_/"

    # Use bazel cquery to extract importpath (from rule attrs)
    IMPORTPATH="${IMPORT_BASE}/${pkg}"

    if [ -z "$IMPORTPATH" ]; then
        echo "    ⚠️  Skipping $target — could not determine importpath"
        continue
    fi

    SRC="${WORKSPACE_ROOT}/${OUT_PATH}/${IMPORTPATH}"
    DEST="${VENDOR_DIR}/${IMPORTPATH}"

    if [ ! -d "${SRC}" ]; then
        echo "    ⚠️  Generated directory not found for $target: ${SRC}"
        continue
    fi

    echo "    $target → vendor/${IMPORTPATH}"
    mkdir -p "${DEST}"
    cp "${SRC}"/*.go "${DEST}/"
done

echo "✅ vendor directory update complete."
