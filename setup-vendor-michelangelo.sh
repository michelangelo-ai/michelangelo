#!/bin/bash

if [ -z ${WORKSPACE_ROOT+x} ]; then
	echo "not in the git repo directory"
	exit
fi

if [ ! -d "${WORKSPACE_ROOT}/go" ]; then
  echo "${WORKSPACE_ROOT}/go does not exist."
fi

if [ ! -d "${WORKSPACE_ROOT}/proto" ]; then
  echo "${WORKSPACE_ROOT}/proto does not exist."
fi

cd "${WORKSPACE_ROOT}"
bazel build proto/...
cd "${WORKSPACE_ROOT}/go"
rm -rf vendor
go mod vendor
cp -r ../bazel-bin/proto/api/api_go_proto_/github.com/michelangelo-ai/michelangelo/proto/api vendor/github.com/michelangelo-ai/michelangelo/proto
cp -r ../bazel-bin/proto/api/v2/v2_go_proto_/github.com/michelangelo-ai/michelangelo/proto/api/v2 vendor/github.com/michelangelo-ai/michelangelo/proto/api
cp -r ../bazel-bin/proto/test/kubeproto/kubeproto_go_proto_/github.com/michelangelo-ai/michelangelo/proto/test vendor/github.com/michelangelo-ai/michelangelo/proto
