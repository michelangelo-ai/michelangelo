#!/usr/bin/env bash
# Sets the zap log level for a Michelangelo Go service running in the local k3d sandbox.
#
#   set_log_level.sh <service> <level>
#     service: apiserver | controllermgr | worker
#     level:   debug | info | warn | error | dpanic | panic | fatal
#
# apiserver/controllermgr ConfigMaps have no `logging:` block by default (only
# worker's does) — this injects/overwrites a full block rather than editing a
# field in place. Encoding is always forced to `json` (regardless of the
# service's helm default) so capture_service_logs.sh can reliably parse it.
#
# Config is loaded once at process startup (go/base/config) — there is no
# hot-reload, so a ConfigMap patch alone does nothing until the deployment
# is restarted. This script does both and verifies the ConfigMap afterward.

set -euo pipefail

SERVICE="${1:-}"
LEVEL="${2:-}"

usage() {
  echo "Usage: $(basename "$0") <apiserver|controllermgr|worker> <debug|info|warn|error|dpanic|panic|fatal>" >&2
  exit 1
}

case "$SERVICE" in
  apiserver|controllermgr|worker) ;;
  *) echo "error: unknown service '$SERVICE'" >&2; usage ;;
esac

case "$LEVEL" in
  debug|info|warn|error|dpanic|panic|fatal) ;;
  *) echo "error: unknown level '$LEVEL'" >&2; usage ;;
esac

CONFIGMAP="michelangelo-${SERVICE}-config"
DEPLOYMENT="michelangelo-${SERVICE}"

command -v kubectl >/dev/null 2>&1 || { echo "error: kubectl not found on PATH" >&2; exit 1; }

# PyYAML isn't guaranteed on the system python3, but it's already a dependency
# of the michelangelo repo's poetry venv (used for the `ma` CLI). Prefer that
# venv's interpreter when available, falling back to system python3.
REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || true)
PYTHON_BIN="python3"
if [ -n "$REPO_ROOT" ] && [ -x "$REPO_ROOT/python/.venv/bin/python3" ]; then
  PYTHON_BIN="$REPO_ROOT/python/.venv/bin/python3"
fi

"$PYTHON_BIN" -c "import yaml" 2>/dev/null || {
  echo "error: PyYAML not importable via '$PYTHON_BIN'. Activate the repo's python venv (source python/.venv/bin/activate) and retry." >&2
  exit 1
}

echo "Reading ConfigMap/${CONFIGMAP}..."
CURRENT_YAML=$(kubectl get configmap "$CONFIGMAP" -o jsonpath='{.data.base\.yaml}')

PATCH_JSON=$(CURRENT_YAML="$CURRENT_YAML" "$PYTHON_BIN" - "$LEVEL" <<'PYEOF'
import os
import sys
import json
import yaml

level = sys.argv[1]
current_yaml = os.environ["CURRENT_YAML"]

doc = yaml.safe_load(current_yaml) or {}
doc["logging"] = {"level": level, "development": False, "encoding": "json"}

new_yaml = yaml.dump(doc, default_flow_style=False, sort_keys=False)
print(json.dumps({"data": {"base.yaml": new_yaml}}))
PYEOF
)

echo "Patching ConfigMap/${CONFIGMAP} (logging.level=${LEVEL}, encoding=json)..."
kubectl patch configmap "$CONFIGMAP" --type merge -p "$PATCH_JSON" >/dev/null

echo "Restarting deployment/${DEPLOYMENT}..."
kubectl rollout restart "deployment/${DEPLOYMENT}" >/dev/null
kubectl rollout status "deployment/${DEPLOYMENT}" --timeout=60s

NEW_YAML=$(kubectl get configmap "$CONFIGMAP" -o jsonpath='{.data.base\.yaml}')
if echo "$NEW_YAML" | grep -q "level: ${LEVEL}"; then
  echo "Verified: ${CONFIGMAP} now has logging.level=${LEVEL}."
  echo "Debug-level lines will only appear once traffic or reconciliation happens — this doesn't send a synthetic request."
else
  echo "error: patch did not take — ${CONFIGMAP} does not show level: ${LEVEL} after patching" >&2
  exit 1
fi
