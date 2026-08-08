#!/usr/bin/env bash
# Detects whether local Go service changes exist and whether they are deployed.
# Prints one line to stdout and exits 0 always:
#
#   clean                     — no Go backend changes; Vite-only verify is appropriate
#   deployed <service>        — Go changes exist AND local image is running in k3d
#   needs-deploy <service>    — Go changes exist but default (non-local) image is running
#
# The caller decides how to proceed based on the output.

set -euo pipefail

WORKTREE_ROOT=$(git rev-parse --show-toplevel)

# Collect all changed files: staged, unstaged, and untracked
CHANGED_FILES=$(
  { git -C "$WORKTREE_ROOT" diff --name-only HEAD 2>/dev/null
    git -C "$WORKTREE_ROOT" ls-files --others --exclude-standard 2>/dev/null
  } | sort -u
)

APISERVER_CHANGED=$(echo "$CHANGED_FILES" | grep -E '^go/cmd/apiserver/' | head -1 || true)
CONTROLLERMGR_CHANGED=$(echo "$CHANGED_FILES" | grep -E '^go/cmd/controllermgr/' | head -1 || true)
WORKER_CHANGED=$(echo "$CHANGED_FILES" | grep -E '^go/cmd/worker/' | head -1 || true)

if [ -z "$APISERVER_CHANGED" ] && [ -z "$CONTROLLERMGR_CHANGED" ] && [ -z "$WORKER_CHANGED" ]; then
  echo "clean"
  exit 0
fi

# Determine which service has changes (controllermgr > worker > apiserver)
if [ -n "$CONTROLLERMGR_CHANGED" ]; then
  SERVICE="controllermgr"
elif [ -n "$WORKER_CHANGED" ]; then
  SERVICE="worker"
else
  SERVICE="apiserver"
fi

# Check what image is currently running in the k3d cluster
RUNNING_IMAGE=$(kubectl get pod -l "app.kubernetes.io/component=$SERVICE" \
  -o jsonpath='{.items[0].spec.containers[0].image}' 2>/dev/null || echo "")

if echo "$RUNNING_IMAGE" | grep -q ':local'; then
  echo "deployed $SERVICE"
else
  echo "needs-deploy $SERVICE"
fi
