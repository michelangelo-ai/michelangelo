#!/usr/bin/env bash
# Recreates only the disposable OSS sandbox when its persisted k3d nodes are
# too storage-constrained to run the integration suite reliably.

set -Eeuo pipefail

CLUSTER_NAME="${K3D_CLUSTER_NAME:-michelangelo-sandbox}"
NAMESPACE="${MA_NAMESPACE:-default}"
MIN_AVAILABLE_KIB="${MIN_SANDBOX_AVAILABLE_KIB:-26214400}"
FORCE_RECREATE_SANDBOX="${FORCE_RECREATE_SANDBOX:-false}"

log() { echo "[$(date -u '+%H:%M:%S')] $*"; }

main() {
  local api_ready=false container available_kib pod_signal status
  local -a recovery_reasons=()

  if ! k3d cluster get "${CLUSTER_NAME}" >/dev/null 2>&1; then
    log "Sandbox ${CLUSTER_NAME} does not exist; sync will create it"
    return 0
  fi

  if [[ "${FORCE_RECREATE_SANDBOX}" = true ]]; then
    log "Recreating disposable sandbox ${CLUSTER_NAME} as explicitly requested"
    k3d cluster delete "${CLUSTER_NAME}"
    if k3d cluster get "${CLUSTER_NAME}" >/dev/null 2>&1; then
      log "Sandbox ${CLUSTER_NAME} still exists after delete"
      return 1
    fi
    log "Removed only ${CLUSTER_NAME}; sandbox sync will perform a clean create"
    return 0
  fi

  # A stopped cluster has no current Kubernetes health signal. Start it only
  # long enough to inspect the disposable node and storage pods.
  k3d cluster start "${CLUSTER_NAME}" >/dev/null 2>&1 || true
  for _ in $(seq 1 12); do
    if kubectl get --raw=/readyz >/dev/null 2>&1; then
      api_ready=true
      break
    fi
    sleep 5
  done

  if [[ "${api_ready}" = true ]]; then
    while IFS=$'\t' read -r node status; do
      if [[ "${status}" = True ]]; then
        recovery_reasons+=("node ${node} reports DiskPressure")
      fi
    done < <(kubectl get nodes \
      -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .status.conditions[?(@.type=="DiskPressure")]}{.status}{end}{"\n"}{end}')

    for pod in minio mysql; do
      pod_signal=$(kubectl get pod "${pod}" -n "${NAMESPACE}" -o \
        jsonpath='{.status.phase}{" "}{.status.reason}{" "}{.status.message}{" "}{range .status.conditions[*]}{.reason}{" "}{.message}{" "}{end}{range .status.containerStatuses[*]}{.state.waiting.reason}{" "}{.state.waiting.message}{" "}{.state.terminated.reason}{" "}{.state.terminated.message}{" "}{end}' \
        2>/dev/null || true)
      if [[ "${pod_signal}" =~ Evicted|ephemeral-storage|TerminationByKubelet ]]; then
        recovery_reasons+=("pod ${NAMESPACE}/${pod} reports ${pod_signal}")
      fi
    done
  else
    log "Kubernetes API is unavailable; checking k3d node filesystems directly"
  fi

  # The examples image is large enough that waiting for kubelet's hard
  # eviction threshold leaves too little room to unpack and start task pods.
  # Inspect only node containers belonging to this exact k3d cluster.
  while IFS= read -r container; do
    [[ "${container}" =~ ^k3d-${CLUSTER_NAME}-(server|agent)-[0-9]+$ ]] || continue
    status=$(docker inspect --format '{{.State.Running}}' "${container}" 2>/dev/null || true)
    [[ "${status}" = true ]] || continue
    available_kib=$(docker exec "${container}" \
      sh -c "df -Pk /var/lib/rancher/k3s | awk 'NR == 2 {print \$4}'" \
      2>/dev/null || true)
    if [[ "${available_kib}" =~ ^[0-9]+$ ]] \
        && (( available_kib < MIN_AVAILABLE_KIB )); then
      recovery_reasons+=(
        "${container} has ${available_kib} KiB available below ${MIN_AVAILABLE_KIB} KiB"
      )
    fi
  done < <(docker ps -a \
    --filter "label=k3d.cluster=${CLUSTER_NAME}" --format '{{.Names}}')

  if (( ${#recovery_reasons[@]} == 0 )); then
    log "Sandbox ${CLUSTER_NAME} has no storage-pressure recovery trigger"
    return 0
  fi

  log "Recreating disposable sandbox ${CLUSTER_NAME}:"
  printf '  - %s\n' "${recovery_reasons[@]}"
  k3d cluster delete "${CLUSTER_NAME}"
  if k3d cluster get "${CLUSTER_NAME}" >/dev/null 2>&1; then
    log "Sandbox ${CLUSTER_NAME} still exists after delete"
    return 1
  fi
  log "Removed only ${CLUSTER_NAME}; sandbox sync will perform a clean create"
}

main "$@"
