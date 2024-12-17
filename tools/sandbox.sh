#!/usr/bin/env bash

set -euo pipefail # strict mode

DIR="$(dirname "$(readlink -f "${0}")")/sandbox"
CLUSTER="ma-sandbox"

function create {

  set -x
  kind create cluster --config "$DIR/kind.yaml" --name $CLUSTER --wait 120s

  kubectl apply -f "$WORKSPACE_ROOT/kubernetes/crd/michelangelo/api/v2beta1"
  kubectl apply -f "$DIR/boot.yaml"
  kubectl apply -f "$DIR/michelangelo-config.yaml"
  kubectl apply -f "$DIR/mysql.yaml"
  kubectl apply -f "$DIR/cadence.yaml"
  kubectl apply -f "$DIR/minio.yaml"

  kubectl wait --for=condition=ready --all pods --timeout 120s

  kubectl run register-domain --restart=Never --rm -it --image ubercadence/cli:v1.2.6 \
    --env=CADENCE_CLI_ADDRESS=cadence:7933 --command -- cadence --domain default domain register --rd 1

  kubectl run create-bucker --restart=Never --rm -it --image minio/mc:RELEASE.2024-12-13T22-19-12Z \
    --command -- bash -c "mc alias set minio http://minio:9000 minioadmin minioadmin && mc mb minio/default"

  cat <<INFO

Cadence  : http://localhost:8088/domains/default/settings
MinIO    : http://localhost:9090/browser/default    * login and password: minioadmin

[ ok ]
INFO
}

function deploy_ray_cluster {
  set -x
  kubectl create -k "github.com/ray-project/kuberay/ray-operator/config/default?ref=v0.4.0&timeout=120s"
  kubectl -n ray-system wait --for=condition=available --timeout=300s --all deployments

  kubectl apply -f "$DIR/ray-cluster.yaml"

  kubectl wait --for=condition=ready --all pods --timeout 120s

  set +x
  cat <<INFO

[ ok ]
INFO
}

function deploy_michelangelo {
  set -x

  bazel build //go/cmd/controllermgr:image.tar --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64
  kind load image-archive "$WORKSPACE_ROOT/bazel-bin/go/cmd/controllermgr/image.tar" --name $CLUSTER

  kubectl apply -f "$DIR/michelangelo.yaml"
  kubectl wait --for=condition=ready --all pods --timeout 120s

  cat <<INFO

Controller Metrics: http://localhost:8090/metrics
Controller Health:  http://localhost:8091

[ ok ]
INFO
}

function delete {
  kind delete cluster --name $CLUSTER
  echo "[ ok ]"
}

function help {
  cat <<INFO

HELP: TODO

Commands:

  create
  delete
  deploy_michelangelo
  deploy_ray_cluster

INFO
}

for f in "$@"; do
  "$f"
done
