#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

BAZEL="${WORKSPACE_ROOT}/tools/bazel"
OUT_DIR="${WORKSPACE_ROOT}/proto-go"
IMPORT_PREFIX="github.com/michelangelo-ai/michelangelo/proto-go"

if [[ ! -x "${BAZEL}" ]]; then
  echo "ERROR: bazel wrapper not found at ${BAZEL}"
  exit 1
fi

echo "Building proto targets with generated Go sources..."
"${BAZEL}" build //proto/... --output_groups=go_generated_srcs

echo "Cleaning existing generated files under ${OUT_DIR}..."
mkdir -p "${OUT_DIR}"
find "${OUT_DIR}" -type f -name "*.go" -delete

echo "Copying generated files into ${OUT_DIR}..."
while IFS= read -r file; do
  rel_path="${file#*${IMPORT_PREFIX}/}"
  dest="${OUT_DIR}/${rel_path}"
  mkdir -p "$(dirname "${dest}")"
  cp "${file}" "${dest}"
done < <(find "${WORKSPACE_ROOT}/bazel-bin/proto" -type f -name "*.go" | grep "/${IMPORT_PREFIX}/")

echo "Syncing proto-go dependency versions with go/go.mod..."
python3 - <<'PY'
import pathlib
import re

root = pathlib.Path("/Users/yingz/git/michelangelo")
go_mod = root / "go" / "go.mod"
proto_mod = root / "proto-go" / "go.mod"

go_text = go_mod.read_text()
proto_text = proto_mod.read_text()

versions = {}
for line in go_text.splitlines():
    line = line.strip()
    if not line or line.startswith("//"):
        continue
    m = re.match(r"^([\\w./-]+)\\s+(v\\S+)$", line)
    if m:
        versions[m.group(1)] = m.group(2)

# Align Go version too.
proto_text = re.sub(r"^go\\s+\\S+", "go 1.23.2", proto_text, flags=re.M)

out_lines = []
for line in proto_text.splitlines():
    m = re.match(r"^(\\s*)([\\w./-]+)\\s+(v\\S+)(\\s*//.*)?$", line)
    if m and m.group(2) in versions:
        indent, mod, _ver, trailing = m.group(1), m.group(2), m.group(3), m.group(4) or ""
        line = f"{indent}{mod} {versions[mod]}{trailing}"
    out_lines.append(line)

proto_mod.write_text("\\n".join(out_lines) + "\\n")
PY

echo "Running go mod tidy in ${OUT_DIR}..."
(cd "${OUT_DIR}" && PATH="${WORKSPACE_ROOT}/tools:${PATH}" "${WORKSPACE_ROOT}/tools/go" mod tidy)

echo "Generated Go protobuf files are available under ${OUT_DIR}."
