#!/usr/bin/env bash
# Integration test: apply and run a pipeline end-to-end.
#
# Usage:
#   cd $REPO_ROOT/python
#   source .venv/bin/activate
#   ../.github/scripts/integration-test.sh
#
# Environment variables (all optional, defaults shown):
#   MA_NAMESPACE     – project namespace               (ma-dev-test)
#   MINIO_ENDPOINT   – MinIO API endpoint              (http://localhost:9091)
#   MINIO_ACCESS_KEY – MinIO access key               (minioadmin)
#   MINIO_SECRET_KEY – MinIO secret key               (minioadmin)
#   POLL_INTERVAL    – seconds between status checks  (30)
#   TIMEOUT          – max seconds to wait per run    (1800)

set -euo pipefail

NAMESPACE="${MA_NAMESPACE:-ma-dev-test}"
MINIO_ENDPOINT="${MINIO_ENDPOINT:-http://localhost:9091}"
# Read credentials from the minio-credentials k8s Secret so the script
# automatically picks up whatever the sandbox VM is configured with.
# Falls back to minioadmin for local dev where the Secret has defaults.
MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-$(kubectl get secret minio-credentials -o jsonpath='{.data.AWS_ACCESS_KEY_ID}' 2>/dev/null | base64 -d || echo minioadmin)}"
MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-$(kubectl get secret minio-credentials -o jsonpath='{.data.AWS_SECRET_ACCESS_KEY}' 2>/dev/null | base64 -d || echo minioadmin)}"
POLL_INTERVAL="${POLL_INTERVAL:-30}"
TIMEOUT="${TIMEOUT:-1800}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
PYTHON_DIR="${REPO_ROOT}/python"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

log() { echo "[$(date -u '+%H:%M:%S')] $*"; }

# Wait until a pipeline run reaches a terminal state.
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

# ---------------------------------------------------------------------------
# Step 1: clean up namespace from previous runs for a fresh state
# ---------------------------------------------------------------------------

log "Cleaning up namespace '${NAMESPACE}' from previous runs..."
kubectl delete namespace "${NAMESPACE}" --ignore-not-found=true --wait=true || true
log "Namespace cleanup done."

# ---------------------------------------------------------------------------
# Step 2: create project
# ---------------------------------------------------------------------------

log "Creating namespace and project '${NAMESPACE}'..."
kubectl create namespace "${NAMESPACE}"
kubectl apply -f "${PYTHON_DIR}/michelangelo/cli/sandbox/demo/project.yaml"
log "Project created."

# ---------------------------------------------------------------------------
# Step 3: install example dependencies
# ---------------------------------------------------------------------------

log "Installing example dependencies..."
pip install "michelangelo[example]" --quiet
log "Dependencies installed."

# ---------------------------------------------------------------------------
# Step 4: apply pipeline (builds and uploads uniflow tar to MinIO)
# ---------------------------------------------------------------------------

log "Applying bert-cola-test pipeline..."
AWS_ACCESS_KEY_ID="${MINIO_ACCESS_KEY}" \
AWS_SECRET_ACCESS_KEY="${MINIO_SECRET_KEY}" \
AWS_ENDPOINT_URL="${MINIO_ENDPOINT}" \
  ma pipeline apply --file="${PYTHON_DIR}/examples/bert_cola/pipeline.yaml"
log "Pipeline applied."

# ---------------------------------------------------------------------------
# Step 5: trigger a dev pipeline run (builds tar and submits inline spec)
# ---------------------------------------------------------------------------

log "Triggering dev pipeline run for 'bert-cola-test'..."
AWS_ACCESS_KEY_ID="${MINIO_ACCESS_KEY}" \
AWS_SECRET_ACCESS_KEY="${MINIO_SECRET_KEY}" \
AWS_ENDPOINT_URL="${MINIO_ENDPOINT}" \
  ma pipeline dev-run --file="${PYTHON_DIR}/examples/bert_cola/pipeline.yaml"
log "Pipeline dev-run triggered."

# ---------------------------------------------------------------------------
# Step 6: find the run name and wait for it
# ---------------------------------------------------------------------------

log "Waiting for pipelinerun to appear..."
for i in $(seq 1 10); do
  RUN_NAME=$(kubectl get pipelinerun -n "${NAMESPACE}" \
    --sort-by=.metadata.creationTimestamp \
    -o jsonpath='{.items[-1].metadata.name}' 2>/dev/null || true)
  if [[ -n "${RUN_NAME}" ]]; then
    break
  fi
  sleep 3
done

if [[ -z "${RUN_NAME:-}" ]]; then
  log "❌ No pipelinerun found in namespace '${NAMESPACE}'."
  exit 1
fi

log "Pipeline run name: ${RUN_NAME}"
wait_for_run "${RUN_NAME}"

log "✅ Integration test passed."
