#!/usr/bin/env bash
# Exercises retrain-example through both direct Python and remote Starlark execution.

set -Eeuo pipefail

NAMESPACE="${MA_NAMESPACE:-default}"
POLL_INTERVAL="${POLL_INTERVAL:-15}"
TIMEOUT="${TIMEOUT:-3600}"
MINIO_ENDPOINT="${MINIO_ENDPOINT:-http://localhost:9091}"
RETRAIN_IMAGE_TAG="${RETRAIN_IMAGE_TAG:-main}"
RETRAIN_IMAGE="ghcr.io/michelangelo-ai/examples:${RETRAIN_IMAGE_TAG}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
PYTHON_DIR="${REPO_ROOT}/python"
PYTHON_BIN="${PYTHON_DIR}/.venv/bin/python"
MA_BIN="${PYTHON_DIR}/.venv/bin/ma"

MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-$(kubectl get secret minio-credentials -o jsonpath='{.data.AWS_ACCESS_KEY_ID}' 2>/dev/null | base64 -d || echo minioadmin)}"
MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-$(kubectl get secret minio-credentials -o jsonpath='{.data.AWS_SECRET_ACCESS_KEY}' 2>/dev/null | base64 -d || echo minioadmin)}"

log() { echo "[$(date -u '+%H:%M:%S')] $*"; }

diagnose_minio() {
  log "MinIO diagnostics"
  kubectl get pod/minio service/minio endpoints/minio \
    -n "${NAMESPACE}" -o wide || true
  kubectl get pod/minio -n "${NAMESPACE}" -o yaml || true
  kubectl describe pod/minio -n "${NAMESPACE}" || true
  kubectl logs pod/minio -n "${NAMESPACE}" --tail=200 || true
  kubectl logs pod/minio -n "${NAMESPACE}" --previous --tail=200 || true
  curl --include --max-time 5 \
    "${MINIO_ENDPOINT}/minio/health/ready" || true
}

diagnose_storage() {
  local pod
  diagnose_minio
  log "MySQL diagnostics"
  kubectl get pod/mysql service/mysql endpoints/mysql \
    -n "${NAMESPACE}" -o wide || true
  kubectl get pod/mysql -n "${NAMESPACE}" -o yaml || true
  kubectl describe pod/mysql -n "${NAMESPACE}" || true
  kubectl logs pod/mysql -n "${NAMESPACE}" --all-containers --tail=200 || true
  kubectl logs pod/mysql -n "${NAMESPACE}" \
    --all-containers --previous --tail=200 || true
  log "Pipeline and Ray diagnostics"
  kubectl get pipelineruns -n "${NAMESPACE}" -o wide || true
  while IFS= read -r run; do
    [[ -n "${run}" ]] || continue
    kubectl get "${run}" -n "${NAMESPACE}" -o yaml || true
  done < <(kubectl get pipelineruns -n "${NAMESPACE}" -o name 2>/dev/null || true)
  kubectl get rayjobs.ray.io,rayclusters.ray.io \
    -n "${NAMESPACE}" -o yaml || true
  while IFS= read -r pod; do
    case "${pod}" in
      *ray* | *bert-cola*)
        kubectl describe "${pod}" -n "${NAMESPACE}" || true
        kubectl logs "${pod}" -n "${NAMESPACE}" \
          --all-containers --tail=200 || true
        ;;
    esac
  done < <(kubectl get pods -n "${NAMESPACE}" -o name 2>/dev/null || true)
}

check_storage_health() {
  local pod phase ready reason

  for pod in minio mysql; do
    phase=$(kubectl get pod "${pod}" -n "${NAMESPACE}" \
      -o jsonpath='{.status.phase}' 2>/dev/null || true)
    ready=$(kubectl get pod "${pod}" -n "${NAMESPACE}" \
      -o jsonpath='{range .status.conditions[?(@.type=="Ready")]}{.status}{end}' \
      2>/dev/null || true)
    reason=$(kubectl get pod "${pod}" -n "${NAMESPACE}" \
      -o jsonpath='{.status.reason}' 2>/dev/null || true)
    if [[ "${phase}" != Running || "${ready}" != True ]]; then
      log "Storage dependency ${pod} is unhealthy (phase=${phase:-missing}, ready=${ready:-missing}, reason=${reason:-none})"
      return 1
    fi
  done

  if ! curl --fail --silent --show-error --max-time 5 \
      "${MINIO_ENDPOINT}/minio/health/ready" >/dev/null; then
    log "MinIO readiness endpoint is unhealthy"
    return 1
  fi
}

process_is_running() {
  local state
  state=$(ps -o stat= -p "$1" 2>/dev/null || true)
  state="${state//[[:space:]]/}"
  [[ -n "${state}" && "${state}" != Z* ]]
}

run_with_storage_monitor() {
  local child_pid status

  "$@" &
  child_pid=$!
  while process_is_running "${child_pid}"; do
    if ! check_storage_health; then
      log "Stopping command ${child_pid} after a storage dependency failed"
      diagnose_storage
      kill "${child_pid}" 2>/dev/null || true
      wait "${child_pid}" 2>/dev/null || true
      return 1
    fi
    sleep "${POLL_INTERVAL}"
  done

  if wait "${child_pid}"; then
    return 0
  else
    status=$?
    log "Monitored command exited with status ${status}"
    return "${status}"
  fi
}

diagnose_retrain_failure() {
  trap - ERR
  log "Retrain diagnostics"
  diagnose_storage
  kubectl get inferenceservers.michelangelo.api inference-server-example \
    -n "${NAMESPACE}" -o yaml || true
  kubectl get deployments.michelangelo.api retrain-example \
    -n "${NAMESPACE}" -o yaml || true
  kubectl get deployment/triton-inference-server-example \
    service/inference-server-example-inference-service \
    -n "${NAMESPACE}" -o wide || true
  kubectl get pods -n "${NAMESPACE}" -o wide || true
  kubectl describe deployment/triton-inference-server-example \
    -n "${NAMESPACE}" || true
  kubectl get events -n "${NAMESPACE}" --sort-by=.lastTimestamp | tail -100 || true
  kubectl logs deployment/triton-inference-server-example \
    -n "${NAMESPACE}" --all-containers --tail=200 || true
  kubectl logs deployment/michelangelo-controllermgr \
    -n "${NAMESPACE}" --all-containers --tail=300 || true
  kubectl logs daemonset/model-sync \
    -n "${NAMESPACE}" --all-containers --tail=200 || true
}

trap diagnose_retrain_failure ERR

restore_minio() {
  local minio_manifest="$1"
  kubectl delete pod/minio -n "${NAMESPACE}" --ignore-not-found --wait=true
  kubectl apply -f "${minio_manifest}"
}

probe_minio() {
  local attempt
  local http_ready=false

  for attempt in $(seq 1 30); do
    if curl --fail --silent --show-error --max-time 5 \
        "${MINIO_ENDPOINT}/minio/health/ready" >/dev/null; then
      log "MinIO HTTP endpoint is ready"
      http_ready=true
      break
    fi
    if (( attempt < 30 )); then
      sleep 5
    fi
  done
  if [[ "${http_ready}" = false ]]; then
    log "MinIO HTTP endpoint did not become ready"
    return 1
  fi

  for attempt in $(seq 1 6); do
    if AWS_ACCESS_KEY_ID="${MINIO_ACCESS_KEY}" \
        AWS_SECRET_ACCESS_KEY="${MINIO_SECRET_KEY}" \
        timeout 15 aws --endpoint-url "${MINIO_ENDPOINT}" \
          s3api list-buckets >/dev/null; then
      log "MinIO S3 API is ready"
      return 0
    fi
    if (( attempt < 6 )); then
      sleep 5
    fi
  done

  log "MinIO S3 API did not become ready"
  return 1
}

ensure_minio_ready() {
  local phase pod_reason terminated_reason waiting_reason
  local minio_manifest="${PYTHON_DIR}/michelangelo/cli/sandbox/resources/minio.yaml"
  local restored=false

  if ! kubectl get pod/minio -n "${NAMESPACE}" >/dev/null 2>&1; then
    log "MinIO pod is missing; restoring the sandbox resource"
    kubectl apply -f "${minio_manifest}"
    restored=true
  else
    phase=$(kubectl get pod/minio -n "${NAMESPACE}" \
      -o jsonpath='{.status.phase}')
    pod_reason=$(kubectl get pod/minio -n "${NAMESPACE}" \
      -o jsonpath='{.status.reason}')
    waiting_reason=$(kubectl get pod/minio -n "${NAMESPACE}" \
      -o jsonpath='{.status.containerStatuses[0].state.waiting.reason}')
    terminated_reason=$(kubectl get pod/minio -n "${NAMESPACE}" \
      -o jsonpath='{.status.containerStatuses[0].state.terminated.reason}')

    if [[ "${phase}" = Failed || "${phase}" = Succeeded \
        || "${pod_reason}" = Evicted \
        || "${waiting_reason}" = CrashLoopBackOff \
        || "${waiting_reason}" = ErrImagePull \
        || "${waiting_reason}" = ImagePullBackOff \
        || "${waiting_reason}" = ContainerCannotRun \
        || "${terminated_reason}" = OOMKilled ]]; then
      log "MinIO is not runnable (phase=${phase}, reason=${pod_reason}, waiting=${waiting_reason}, terminated=${terminated_reason}); restoring it"
      diagnose_minio
      restore_minio "${minio_manifest}"
      restored=true
    fi
  fi

  if ! kubectl get service/minio -n "${NAMESPACE}" >/dev/null 2>&1; then
    log "MinIO service is missing; restoring the sandbox resource"
    kubectl apply -f "${minio_manifest}"
  fi

  kubectl wait --for=condition=Ready pod/minio \
    -n "${NAMESPACE}" --timeout=180s

  if probe_minio; then
    return 0
  fi

  if [[ "${restored}" = false ]]; then
    log "MinIO pod is ready but its HTTP/S3 endpoint is unhealthy; restoring it once"
    diagnose_minio
    restore_minio "${minio_manifest}"
    kubectl wait --for=condition=Ready pod/minio \
      -n "${NAMESPACE}" --timeout=180s
    probe_minio
    return
  fi

  return 1
}

refresh_model_sync() {
  local script_path="${PYTHON_DIR}/michelangelo/cli/sandbox/resources/sync-models.py"

  log "Refreshing the reused sandbox model-sync process"
  kubectl create configmap model-sync-script \
    -n "${NAMESPACE}" \
    "--from-file=sync-models.py=${script_path}" \
    --dry-run=client -o yaml \
    | kubectl apply -f -
  kubectl rollout restart daemonset/model-sync -n "${NAMESPACE}"
  kubectl rollout status daemonset/model-sync \
    -n "${NAMESPACE}" --timeout=180s
}

pipeline_run_state() {
  local run_name="$1"

  MA_API_SERVER="${MA_API_SERVER:-localhost:15566}" \
    "${PYTHON_BIN}" - "${NAMESPACE}" "${run_name}" <<'PY'
import sys

from michelangelo.api.v2 import APIClient
from michelangelo.gen.api.v2.pipeline_run_pb2 import PipelineRunState

try:
    APIClient.set_caller("retrain-integration-test")
    pipeline_run = APIClient.PipelineRunService.get_pipeline_run(
        namespace=sys.argv[1], name=sys.argv[2]
    )
    print(PipelineRunState.Name(pipeline_run.status.state))
except Exception as error:
    print(f"PipelineRun API lookup failed: {error}", file=sys.stderr)
    print("UNKNOWN")
PY
}

wait_for_pipeline_run() {
  local run_name="$1"
  local start state elapsed
  start=$(date +%s)

  while true; do
    state=$(pipeline_run_state "${run_name}")
    log "PipelineRun ${run_name}: ${state}"

    if ! check_storage_health; then
      log "Storage failed while waiting for PipelineRun ${run_name}"
      kubectl get pipelinerun "${run_name}" -n "${NAMESPACE}" -o yaml || true
      return 1
    fi

    case "${state}" in
      PIPELINE_RUN_STATE_SUCCEEDED)
        return 0
        ;;
      PIPELINE_RUN_STATE_FAILED | PIPELINE_RUN_STATE_KILLED)
        "${MA_BIN}" pipeline_run get \
          --namespace="${NAMESPACE}" --name="${run_name}" || true
        return 1
        ;;
    esac

    elapsed=$(( $(date +%s) - start ))
    if (( elapsed > TIMEOUT )); then
      log "Timed out waiting for PipelineRun ${run_name}"
      return 1
    fi
    sleep "${POLL_INTERVAL}"
  done
}

latest_retrain_run() {
  kubectl get pipelinerun -n "${NAMESPACE}" \
    -o jsonpath='{range .items[?(@.spec.pipeline.name=="retrain-example")]}{.metadata.creationTimestamp}{"\t"}{.metadata.name}{"\n"}{end}' \
    | sort | tail -1 | awk '{print $2}'
}

deployment_revision() {
  kubectl get deployments.michelangelo.api retrain-example -n "${NAMESPACE}" \
    -o jsonpath='{.status.currentRevision.name}'
}

assert_healthy_deployment() {
  local state stage
  state=$(kubectl get deployments.michelangelo.api retrain-example \
    -n "${NAMESPACE}" -o jsonpath='{.status.state}')
  stage=$(kubectl get deployments.michelangelo.api retrain-example \
    -n "${NAMESPACE}" -o jsonpath='{.status.stage}')
  log "Deployment retrain-example: state=${state}, stage=${stage}"
  test "${state}" = DEPLOYMENT_STATE_HEALTHY
  test "${stage}" = DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
}

cd "${PYTHON_DIR}"

log "Starting the lightweight retrain inference backend"
kubectl apply -f "${REPO_ROOT}/.github/scripts/retrain-inference-server.yaml"
kubectl rollout status deployment/triton-inference-server-example \
  -n "${NAMESPACE}" --timeout=180s

inference_state=$(kubectl get inferenceservers.michelangelo.api \
  inference-server-example -n "${NAMESPACE}" -o jsonpath='{.status.state}' \
  2>/dev/null || true)
if [[ "${inference_state}" != INFERENCE_SERVER_STATE_SERVING ]]; then
  log "Creating the inference-server-example sandbox demo"
  "${PYTHON_BIN}" - <<'PY'
from michelangelo.cli.sandbox import sandbox

sandbox._assert_sandbox_cluster_running()
sandbox._create_inference_demo_crs()
PY
fi
refresh_model_sync

log "Ensuring the deployment-example template exists"
if kubectl get deployments.michelangelo.api deployment-example \
    -n "${NAMESPACE}" >/dev/null 2>&1; then
  log "Reusing the existing deployment-example template"
else
  "${MA_BIN}" deployment apply \
    --file="${PYTHON_DIR}/michelangelo/cli/sandbox/demo/inference/deployment.yaml"
fi

log "Resetting the test-owned retrain deployment"
kubectl delete deployments.michelangelo.api retrain-example \
  -n "${NAMESPACE}" --ignore-not-found --wait=true --timeout=180s

log "Registering the namespace-compatible BERT/CoLA and retrain pipelines"
ensure_minio_ready
kubectl apply -f "${PYTHON_DIR}/examples/retrain_example/project.yaml"
AWS_ACCESS_KEY_ID="${MINIO_ACCESS_KEY}" \
AWS_SECRET_ACCESS_KEY="${MINIO_SECRET_KEY}" \
AWS_ENDPOINT_URL="${MINIO_ENDPOINT}" \
  "${MA_BIN}" pipeline apply --file=examples/retrain_example/training_pipeline.yaml
kubectl annotate pipelines.michelangelo.api bert-cola-test \
  -n "${NAMESPACE}" \
  michelangelo/uniflow-image="${RETRAIN_IMAGE}" --overwrite
AWS_ACCESS_KEY_ID="${MINIO_ACCESS_KEY}" \
AWS_SECRET_ACCESS_KEY="${MINIO_SECRET_KEY}" \
AWS_ENDPOINT_URL="${MINIO_ENDPOINT}" \
  "${MA_BIN}" pipeline apply --file=examples/retrain_example/pipeline.yaml
kubectl annotate pipelines.michelangelo.api retrain-example \
  -n "${NAMESPACE}" \
  michelangelo/uniflow-image="${RETRAIN_IMAGE}" --overwrite

log "Running retrain-example through the local Python plugin implementations"
run_with_storage_monitor env \
  MA_API_SERVER="${MA_API_SERVER:-localhost:15566}" \
  "${PYTHON_BIN}" -m examples.retrain_example.retrain local-run
assert_healthy_deployment
local_revision=$(deployment_revision)
test -n "${local_revision}"
log "Local execution deployed ${local_revision}"

log "Submitting retrain-example through the remote Starlark worker"
previous_remote_run=$(latest_retrain_run)
ensure_minio_ready
run_with_storage_monitor env \
  AWS_ACCESS_KEY_ID="${MINIO_ACCESS_KEY}" \
  AWS_SECRET_ACCESS_KEY="${MINIO_SECRET_KEY}" \
  AWS_ENDPOINT_URL="${MINIO_ENDPOINT}" \
  "${MA_BIN}" pipeline dev-run --file=examples/retrain_example/pipeline.yaml

remote_run=""
for _ in $(seq 1 20); do
  remote_run=$(latest_retrain_run)
  if [[ -n "${remote_run}" && "${remote_run}" != "${previous_remote_run}" ]]; then
    break
  fi
  sleep 3
done
if [[ -z "${remote_run}" || "${remote_run}" = "${previous_remote_run}" ]]; then
  log "No new retrain-example PipelineRun appeared"
  exit 1
fi
wait_for_pipeline_run "${remote_run}"
assert_healthy_deployment
remote_revision=$(deployment_revision)
test -n "${remote_revision}"
if [[ "${remote_revision}" = "${local_revision}" ]]; then
  log "Remote retrain did not roll out a new model revision"
  exit 1
fi
log "Remote execution deployed ${remote_revision}"
log "Retrain local and remote integration test passed"
trap - ERR
