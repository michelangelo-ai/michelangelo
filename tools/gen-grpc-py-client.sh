#!/usr/bin/env bash

# Generate gRPC Python client code from protobuf files
set -e
set -x

if ! command -v buf &> /dev/null; then
    echo "Buf is NOT installed. Please install it from https://docs.buf.build/installation"
    exit 1
fi

# Create a temporary directory
TMP_DIR=$(mktemp -d)

# Ensure the directory is deleted on script exit
trap "rm -rf $TMP_DIR" EXIT

# copy protobuf files to temporary directory
mkdir -p "${TMP_DIR}/michelangelo"
cp -r "${WORKSPACE_ROOT}/proto/api" "${TMP_DIR}/michelangelo"

# prepare buf configuration files
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

cat << EOF > "${TMP_DIR}/buf.gen.yaml"
version: v2

plugins:
  - remote: buf.build/protocolbuffers/python
    out: gen/python
    include_imports: true

  - remote: buf.build/grpc/python
    out: gen/python
    include_imports: true
EOF

# generate gRPC python code
buf dep update "${TMP_DIR}"
buf generate --template "${TMP_DIR}/buf.gen.yaml" "${TMP_DIR}" -o "${TMP_DIR}"

# replace package names
find "${TMP_DIR}/gen/python" -name '*_pb2*.py' -print0 | \
  xargs -0 perl -pi -e \
  's/from michelangelo./from michelangelo.gen./g; s/from k8s.io./from michelangelo.gen.k8s.io./g'

rm -rf /Users/yingz/git/michelangelo/python/michelangelo/gen
mkdir -p /Users/yingz/git/michelangelo/python/michelangelo/gen
mv "${TMP_DIR}/gen/python/k8s" "${WORKSPACE_ROOT}/python/michelangelo/gen/k8s"
mv "${TMP_DIR}/gen/python/michelangelo/api" "${WORKSPACE_ROOT}/python/michelangelo/gen/api"

find "${WORKSPACE_ROOT}/python/michelangelo/gen/" -type d -exec touch {}/"__init__.py"  \;