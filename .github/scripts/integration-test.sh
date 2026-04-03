#!/usr/bin/env bash
# Integration test: register and run pipelines end-to-end via mactl.
#
# Usage:
#   cd $REPO_ROOT/python
#   source .venv/bin/activate
#   ../.github/scripts/integration-test.sh
#
# Environment variables (all optional, defaults shown):
#   MA_NAMESPACE    – project namespace               (ma-dev-test)
#   MINIO_ENDPOINT  – MinIO API endpoint              (http://localhost:9091)
#   MINIO_ACCESS_KEY – MinIO access key              (minioadmin)
#   MINIO_SECRET_KEY – MinIO secret key              (minioadmin)
#   POLL_INTERVAL   – seconds between status checks  (30)
#   TIMEOUT         – max seconds to wait per run    (1800)

set -euo pipefail

NAMESPACE="${MA_NAMESPACE:-ma-dev-test}"
MINIO_ENDPOINT="${MINIO_ENDPOINT:-http://localhost:9091}"
MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-minioadmin}"
MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-minioadmin}"
POLL_INTERVAL="${POLL_INTERVAL:-30}"
TIMEOUT="${TIMEOUT:-1800}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
PYTHON_DIR="${REPO_ROOT}/python"

TAR_LOCAL="${PYTHON_DIR}/michelangelo/cli/sandbox/demo/pipeline/bert_local.tar"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

log() { echo "[$(date -u '+%H:%M:%S')] $*"; }

# Wait until a pipeline run reaches a terminal state.
# Usage: wait_for_run <run-name>
wait_for_run() {
  local run_name="$1"
  local start elapsed state

  start=$(date +%s)
  log "Polling pipeline run '${run_name}' in namespace '${NAMESPACE}'..."

  while true; do
    state=$(kubectl get pipelinerun "${run_name}" \
      -n "${NAMESPACE}" \
      -o jsonpath='{.status.state}' 2>/dev/null || echo "UNKNOWN")

    log "  state: ${state}"

    case "${state}" in
      PIPELINE_RUN_STATE_SUCCEEDED)
        log "✅ Pipeline run '${run_name}' SUCCEEDED."
        return 0
        ;;
      PIPELINE_RUN_STATE_FAILED | PIPELINE_RUN_STATE_KILLED)
        log "❌ Pipeline run '${run_name}' ended with state: ${state}"
        kubectl get pipelinerun "${run_name}" -n "${NAMESPACE}" -o yaml || true
        return 1
        ;;
    esac

    elapsed=$(( $(date +%s) - start ))
    if (( elapsed > TIMEOUT )); then
      log "❌ Timed out after ${TIMEOUT}s waiting for '${run_name}'."
      return 1
    fi

    sleep "${POLL_INTERVAL}"
  done
}

# Run a pipeline and return the generated run name.
# Usage: run_name=$(run_pipeline <namespace> <pipeline-name>)
run_pipeline() {
  local ns="$1" name="$2"
  log "Running pipeline '${name}' in namespace '${ns}'"
  # ma pipeline run prints a proto response that includes metadata.name.
  ma pipeline run -n "${ns}" --name "${name}" \
    | grep -oP '(?<=name: ")[^"]+|(?<=name: )run-\S+' \
    | head -1
}

# ---------------------------------------------------------------------------
# Step 1: upload bert_local.tar to MinIO
# ---------------------------------------------------------------------------

cd "${PYTHON_DIR}"

log "Uploading bert_local.tar to s3://default/bert_local.tar ..."
AWS_ACCESS_KEY_ID="${MINIO_ACCESS_KEY}" \
AWS_SECRET_ACCESS_KEY="${MINIO_SECRET_KEY}" \
  aws s3 cp "${TAR_LOCAL}" s3://default/bert_local.tar \
    --endpoint-url "${MINIO_ENDPOINT}" \
    --region us-east-1

log "Upload complete."

# ---------------------------------------------------------------------------
# Step 2: create demo project + register demo pipelines
# ---------------------------------------------------------------------------

log "Creating demo project and registering pipelines..."
ma sandbox demo pipeline

# ---------------------------------------------------------------------------
# Step 3: run training-pipeline (bert_cola)
# ---------------------------------------------------------------------------

log "=== Test: training-pipeline (bert_cola) ==="
RUN_NAME=$(run_pipeline "${NAMESPACE}" "training-pipeline")
log "Pipeline run name: ${RUN_NAME}"
wait_for_run "${RUN_NAME}"

log "✅ Integration test passed."
