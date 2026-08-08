#!/usr/bin/env bash
# Filters kubectl logs for a Michelangelo service into clean, PR-ready JSON lines.
#
#   capture_service_logs.sh <service> [--resource NAME] [--component NAME] \
#                            [--level LVL] [--exclude PATTERN] [--tail N]
#
# Assumes the service is emitting zap's default JSON encoding (go/base/zapfx,
# zap.NewProductionEncoderConfig) — run set_log_level.sh first if it isn't.
# Non-JSON lines are dropped rather than erroring, since a service can still
# emit occasional plain-text startup lines before logging is configured.
#
# By default, strips YARPC's per-RPC observability tracing lines
# ("Handled inbound request." / "Made outbound call.", logged at debug level
# by go.uber.org/yarpc's built-in middleware) — this is the noise that makes
# raw debug logs unreadable.

set -euo pipefail

SERVICE="${1:-}"
shift || true

usage() {
  echo "Usage: $(basename "$0") <apiserver|controllermgr|worker> [--resource NAME] [--component NAME] [--level LVL] [--exclude PATTERN] [--tail N]" >&2
  exit 1
}

case "$SERVICE" in
  apiserver|controllermgr|worker) ;;
  *) echo "error: unknown service '$SERVICE'" >&2; usage ;;
esac

RESOURCE=""
COMPONENT=""
LEVEL=""
EXCLUDE=""
TAIL="200"

while [ $# -gt 0 ]; do
  case "$1" in
    --resource) RESOURCE="$2"; shift 2 ;;
    --component) COMPONENT="$2"; shift 2 ;;
    --level) LEVEL="$2"; shift 2 ;;
    --exclude) EXCLUDE="$2"; shift 2 ;;
    --tail) TAIL="$2"; shift 2 ;;
    *) echo "error: unknown argument '$1'" >&2; usage ;;
  esac
done

command -v kubectl >/dev/null 2>&1 || { echo "error: kubectl not found on PATH" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "error: jq not found on PATH" >&2; exit 1; }

DEPLOYMENT="michelangelo-${SERVICE}"

# Build the jq filter incrementally. Drop non-JSON lines first (`fromjson? //
# empty`), then the always-on tracing-noise filter, then each opt-in filter.
JQ_FILTER='fromjson? // empty'
JQ_FILTER="$JQ_FILTER | select(.msg != \"Handled inbound request.\" and .msg != \"Made outbound call.\" and .msg != \"Error handling inbound request.\" and .msg != \"Error making outbound call.\")"

if [ -n "$LEVEL" ]; then
  JQ_FILTER="$JQ_FILTER | select(.level == \$level)"
fi
if [ -n "$COMPONENT" ]; then
  JQ_FILTER="$JQ_FILTER | select(.component == \$component)"
fi
if [ -n "$RESOURCE" ]; then
  # Identity field names vary by controller (name, namespace-name, pipelineRun,
  # ...), so match by substring against the whole serialized line instead of
  # hardcoding one field.
  JQ_FILTER="$JQ_FILTER | select(tostring | contains(\$resource))"
fi
if [ -n "$EXCLUDE" ]; then
  JQ_FILTER="$JQ_FILTER | select((.msg // \"\") | test(\$exclude) | not)"
fi

kubectl logs "deployment/${DEPLOYMENT}" --tail="$TAIL" | jq -Rc \
  --arg level "$LEVEL" \
  --arg component "$COMPONENT" \
  --arg resource "$RESOURCE" \
  --arg exclude "$EXCLUDE" \
  "$JQ_FILTER"
