#!/usr/bin/env bash
# Helper: builds the grpcurl JSON payload for CreateRayCluster.
# Usage: source _helpers.sh; create_ray_cluster "$CLUSTER_NAME" "$HEAD_IMAGE" "$WORKER_IMAGE" "$HEAD_RESOURCES" "$WORKER_RESOURCES" "$HEAD_COMMAND"
# All arguments after name are optional and default to a working config.

API="127.0.0.1:15566"

create_ray_cluster() {
  local name="${1:?cluster name required}"
  local head_image="${2:-docker.io/library/examples:latest}"
  local worker_image="${3:-docker.io/library/examples:latest}"
  local head_mem="${4:-2Gi}"
  local worker_mem="${5:-2Gi}"
  local head_command="${6:-}"  # empty = use default entrypoint

  # Build command array for head container (optional override)
  local command_field=""
  if [ -n "$head_command" ]; then
    command_field="\"command\": [${head_command}],"
  fi

  local payload
  payload=$(cat <<PAYLOAD
{
  "ray_cluster": {
    "metadata": {
      "name": "${name}",
      "namespace": "default",
      "labels": {
        "michelangelo/cluster-affinity": "michelangelo-compute-0"
      }
    },
    "spec": {
      "user": { "name": "test_user" },
      "rayVersion": "2.3.1",
      "head": {
        "serviceType": "ClusterIP",
        "pod": {
          "metadata": { "name": "", "namespace": "", "selfLink": "", "uid": "", "resourceVersion": "", "generation": "0", "creationTimestamp": {} },
          "spec": {
            "containers": [
              {
                "name": "head",
                "image": "${head_image}",
                ${command_field}
                "env": [
                  { "name": "UF_REMOTE_RUN", "value": "1" },
                  { "name": "RAY_DEDUP_LOGS", "value": "0" },
                  { "name": "PYTHONPATH", "value": "/app" },
                  { "name": "MA_NAMESPACE", "value": "default" },
                  { "name": "IMAGE_PULL_POLICY", "value": "IfNotPresent" }
                ],
                "resources": {
                  "requests": {
                    "cpu": { "string": "1" },
                    "memory": { "string": "${head_mem}" }
                  },
                  "limits": {
                    "memory": { "string": "${head_mem}" }
                  }
                },
                "imagePullPolicy": "IfNotPresent",
                "envFrom": [{ "configMapRef": { "localObjectReference": { "name": "michelangelo-config" } } }]
              }
            ]
          }
        },
        "rayStartParams": {
          "block": "true",
          "dashboard-host": "0.0.0.0"
        }
      },
      "workers": [
        {
          "pod": {
            "metadata": { "name": "", "namespace": "", "selfLink": "", "uid": "", "resourceVersion": "", "generation": "0", "creationTimestamp": {} },
            "spec": {
              "containers": [
                {
                  "name": "worker",
                  "image": "${worker_image}",
                  "env": [
                    { "name": "UF_REMOTE_RUN", "value": "1" },
                    { "name": "RAY_DEDUP_LOGS", "value": "0" },
                    { "name": "PYTHONPATH", "value": "/app" },
                    { "name": "MA_NAMESPACE", "value": "default" },
                    { "name": "IMAGE_PULL_POLICY", "value": "IfNotPresent" }
                  ],
                  "resources": {
                    "requests": {
                      "cpu": { "string": "1" },
                      "memory": { "string": "${worker_mem}" }
                    },
                    "limits": {
                      "memory": { "string": "${worker_mem}" }
                    }
                  },
                  "imagePullPolicy": "IfNotPresent",
                  "envFrom": [{ "configMapRef": { "localObjectReference": { "name": "michelangelo-config" } } }]
                }
              ],
              "restartPolicy": "Never"
            }
          },
          "minInstances": 1,
          "maxInstances": 1,
          "nodeType": "worker-group-1",
          "rayStartParams": {
            "block": "true",
            "dashboard-host": "0.0.0.0"
          }
        }
      ]
    }
  }
}
PAYLOAD
)

  echo "Creating RayCluster '${name}'..."
  echo ""

  grpcurl -plaintext -max-time 30 \
    -H 'rpc-caller: grpcurl-test' \
    -H 'rpc-service: ma-apiserver' \
    -H 'rpc-encoding: proto' \
    -d "$payload" \
    "$API" michelangelo.api.v2.RayClusterService/CreateRayCluster

  echo ""
  echo "Created. Monitor with:"
  echo "  ./scripts/raycluster-failure-tests/get-status.sh ${name} --watch"
  echo "  ./scripts/raycluster-failure-tests/watch-kuberay.sh ${name}"
}
