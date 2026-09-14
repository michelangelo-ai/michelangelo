#!/usr/bin/env bash
# Exercises retrain-example through both direct Python and remote Starlark execution.

set -euo pipefail

NAMESPACE="${MA_NAMESPACE:-default}"
POLL_INTERVAL="${POLL_INTERVAL:-15}"
TIMEOUT="${TIMEOUT:-3600}"
MINIO_ENDPOINT="${MINIO_ENDPOINT:-http://localhost:9091}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
PYTHON_DIR="${REPO_ROOT}/python"
PYTHON_BIN="${PYTHON_DIR}/.venv/bin/python"
MA_BIN="${PYTHON_DIR}/.venv/bin/ma"

MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-$(kubectl get secret minio-credentials -o jsonpath='{.data.AWS_ACCESS_KEY_ID}' 2>/dev/null | base64 -d || echo minioadmin)}"
MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-$(kubectl get secret minio-credentials -o jsonpath='{.data.AWS_SECRET_ACCESS_KEY}' 2>/dev/null | base64 -d || echo minioadmin)}"

log() { echo "[$(date -u '+%H:%M:%S')] $*"; }

wait_for_pipeline_run() {
  local run_name="$1"
  local start state elapsed
  start=$(date +%s)

  while true; do
    state=$(kubectl get pipelinerun "${run_name}" -n "${NAMESPACE}" \
      -o jsonpath='{.status.state}' 2>/dev/null || echo UNKNOWN)
    log "PipelineRun ${run_name}: ${state}"

    case "${state}" in
      PIPELINE_RUN_STATE_SUCCEEDED)
        return 0
        ;;
      PIPELINE_RUN_STATE_FAILED | PIPELINE_RUN_STATE_KILLED)
        kubectl get pipelinerun "${run_name}" -n "${NAMESPACE}" -o yaml || true
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

inference_state=$(kubectl get inferenceservers.michelangelo.api \
  inference-server-example -n "${NAMESPACE}" -o jsonpath='{.status.state}' \
  2>/dev/null || true)
if [[ "${inference_state}" != INFERENCE_SERVER_STATE_SERVING ]]; then
  log "Creating the inference-server-example sandbox demo"
  "${MA_BIN}" sandbox demo inference
fi

log "Registering the namespace-compatible BERT/CoLA and retrain pipelines"
AWS_ACCESS_KEY_ID="${MINIO_ACCESS_KEY}" \
AWS_SECRET_ACCESS_KEY="${MINIO_SECRET_KEY}" \
AWS_ENDPOINT_URL="${MINIO_ENDPOINT}" \
  "${MA_BIN}" pipeline apply --file=examples/retrain_example/training_pipeline.yaml
AWS_ACCESS_KEY_ID="${MINIO_ACCESS_KEY}" \
AWS_SECRET_ACCESS_KEY="${MINIO_SECRET_KEY}" \
AWS_ENDPOINT_URL="${MINIO_ENDPOINT}" \
  "${MA_BIN}" pipeline apply --file=examples/retrain_example/pipeline.yaml

log "Running retrain-example through the local Python plugin implementations"
MA_API_SERVER="${MA_API_SERVER:-localhost:15566}" \
  "${PYTHON_BIN}" -m examples.retrain_example.retrain local-run
assert_healthy_deployment
local_revision=$(deployment_revision)
test -n "${local_revision}"
log "Local execution deployed ${local_revision}"

log "Submitting retrain-example through the remote Starlark worker"
previous_remote_run=$(latest_retrain_run)
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
