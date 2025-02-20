#!/usr/bin/env bash

# Generate gRPC client code from protobuf files
set -e
set -x

if ! command -v buf &> /dev/null; then
  echo "Buf is NOT installed. Please install it from https://docs.buf.build/installation"
  exit 1
fi

while [[ "$#" -gt 0 ]]; do
  case $1 in
    --clients)
      if [[ "$2" =~ ^(python|javascript)(,(python|javascript))*$ ]]; then
        CLIENTS="$2"
      else
        echo "Invalid clients specified. Only 'python' and 'javascript' are allowed."
        exit 1
      fi
      shift 2
      ;;
    --help|-h)
      echo "Usage: $0 [--clients python,javascript]"
      echo "  --clients: Specify the clients to generate code for. Allowed values are 'python' and 'javascript'. Defaults to all."
      exit 0
      ;;
    *) echo "Unknown parameter passed: $1"; exit 1 ;;
  esac
done

if [ -z "$CLIENTS" ]; then
  echo "No clients specified. Defaulting to all available clients: python,javascript"
  CLIENTS="python,javascript"
fi

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

  - remote: buf.build/bufbuild/es
    out: gen/javascript
    include_imports: true
EOF

# generate gRPC code
buf dep update "${TMP_DIR}"
buf generate --template "${TMP_DIR}/buf.gen.yaml" "${TMP_DIR}" -o "${TMP_DIR}"

# replace package names for python
find "${TMP_DIR}/gen/python" -name '*_pb2*.py' -print0 | \
  xargs -0 perl -pi -e \
  's/from michelangelo./from michelangelo.gen./g; s/from k8s.io./from michelangelo.gen.k8s.io./g'

# create __init__.py files
find "${TMP_DIR}/gen/python" -type d -exec touch {}/"__init__.py"  \;

# copy generated code to requesting client directories
IFS=',' read -ra CLIENT_ARRAY <<< "$CLIENTS"
for CLIENT in "${CLIENT_ARRAY[@]}"; do
  rm -rf "${WORKSPACE_ROOT}/${CLIENT}/michelangelo/gen"
  mkdir -p "${WORKSPACE_ROOT}/${CLIENT}/michelangelo/gen"

  mv "${TMP_DIR}/gen/${CLIENT}/k8s.io" "${WORKSPACE_ROOT}/${CLIENT}/michelangelo/gen/k8s.io"
  mv "${TMP_DIR}/gen/${CLIENT}/michelangelo/api"* "${WORKSPACE_ROOT}/${CLIENT}/michelangelo/gen/api"
done
