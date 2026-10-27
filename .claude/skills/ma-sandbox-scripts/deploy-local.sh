#!/usr/bin/env bash
# Build a Go service from a checkout, import it into the k3d sandbox, deploy it
# via `ma sandbox sync`, and verify the three "what's deployed" signals.
#
# Usage: deploy-local.sh <apiserver|controllermgr|worker> [--repo <path>]
#   --repo   checkout to build from (default: the repo this script lives in).
#            Treated as read-only; all build output goes under /tmp.
#
# WARNING: `ma sandbox sync` DROPS AND RECREATES the sandbox MySQL
# `michelangelo` database before running helm. Metadata-storage-only data
# (e.g. PipelineRuns pruned from etcd) is lost.
#
# Services already running a `michelangelo-<svc>:*` image keep it: sync runs
# with --reuse-values, so every image override must be re-passed on each call.
# Exit codes: 0 all verify signals pass, 1 failure, 2 usage.

set -euo pipefail

SELF_REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
CLUSTER=michelangelo-sandbox
SERVICE=${1:-}
if [ -n "$SERVICE" ]; then shift; fi
SRC_REPO=$SELF_REPO
while [ $# -gt 0 ]; do
  case "$1" in
    --repo) SRC_REPO=$(cd "$2" && pwd); shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done
case "$SERVICE" in
  apiserver|controllermgr|worker) ;;
  *) echo "usage: deploy-local.sh <apiserver|controllermgr|worker> [--repo <path>]" >&2; exit 2 ;;
esac

IMAGE="michelangelo-$SERVICE:local"
DEPLOY="michelangelo-$SERVICE"
CTX="/tmp/ma-sandbox-deploy/$SERVICE"
step() { echo; echo "==> $*"; }

step "Preflight"
k3d cluster list --no-headers | awk '{print $1}' | grep -qx "$CLUSTER" \
  || { echo "cluster $CLUSTER missing; run 'ma sandbox create'" >&2; exit 1; }
MA=$(bash "$SELF_REPO/.claude/skills/ma-sandbox-scripts/python-env.sh" | tail -1)

# Other services already on locally-built images, captured before sync restarts anything.
OVERRIDES=()
for svc in apiserver controllermgr worker ui; do
  if [ "$svc" = "$SERVICE" ]; then continue; fi
  cur=$(kubectl get deployment "michelangelo-$svc" -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || true)
  case "$cur" in
    "michelangelo-$svc:"*) OVERRIDES+=(--set "images.$svc.repository=michelangelo-$svc" --set "images.$svc.tag=${cur##*:}") ;;
  esac
done

step "Build static linux/arm64 binary from $SRC_REPO"
rm -rf "$CTX"
mkdir -p "$CTX"
(cd "$SRC_REPO/go" && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o "$CTX/$SERVICE" "./cmd/$SERVICE/")
cp -r "$SRC_REPO/go/cmd/$SERVICE/config" "$CTX/config"

step "Docker build $IMAGE"
BRANCH=$(git -C "$SRC_REPO" rev-parse --abbrev-ref HEAD)
SHA=$(git -C "$SRC_REPO" rev-parse --short=8 HEAD)
DIRTY=$(git -C "$SRC_REPO" diff --quiet HEAD && echo false || echo true)
docker build \
  -f "$SRC_REPO/docker/service.Dockerfile" \
  --build-arg BINARY_PATH="$SERVICE" \
  --build-arg CONFIG_PATH=config \
  --platform linux/arm64 \
  --label "git.branch=$BRANCH" --label "git.sha=$SHA" --label "git.dirty=$DIRTY" \
  -t "$IMAGE" "$CTX"

step "Import into k3d"
k3d image import "$IMAGE" -c "$CLUSTER"

step "ma sandbox sync (WARNING: drops and recreates the MySQL 'michelangelo' database)"
"$MA" sandbox sync \
  --set "images.$SERVICE.repository=michelangelo-$SERVICE" \
  --set "images.$SERVICE.tag=local" \
  ${OVERRIDES[@]+"${OVERRIDES[@]}"} \
  --set images.pullPolicy=IfNotPresent \
  --set ui.enabled=true --set envoy.enabled=true \
  --set controllermgr.enabled=true --set worker.enabled=true

step "Rollout"
# Sync restarts deployments; restart explicitly so a same-tag rebuild is always picked up.
kubectl rollout restart "deployment/$DEPLOY"
kubectl rollout status "deployment/$DEPLOY" --timeout=180s

step "Verify what's deployed"
SEL="app.kubernetes.io/component=$SERVICE"
FAIL=0
POD_IMAGE=$(kubectl get pod -l "$SEL" --field-selector=status.phase=Running -o jsonpath='{.items[0].spec.containers[0].image}')
echo "1. pod image:   $POD_IMAGE"
[ "$POD_IMAGE" = "$IMAGE" ] || { echo "   FAIL: expected $IMAGE"; FAIL=1; }

LABELS=$(docker image inspect "$IMAGE" --format '{{json .Config.Labels}}')
echo "2. labels:      $LABELS"
case "$LABELS" in
  *"\"git.sha\":\"$SHA\""*) ;;
  *) echo "   FAIL: git.sha != $SHA"; FAIL=1 ;;
esac

BUILT=$(docker image inspect "$IMAGE" --format '{{.Created}}')
STARTED=$(kubectl get pod -l "$SEL" --field-selector=status.phase=Running -o jsonpath='{.items[0].status.startTime}')
echo "3. image built: $BUILT"
echo "   pod started: $STARTED"
# Both are UTC ISO-8601; compare at second precision (first 19 chars).
if [[ "${STARTED:0:19}" < "${BUILT:0:19}" ]]; then
  echo "   FAIL: pod started before image build"
  FAIL=1
fi

if [ "$FAIL" = 0 ]; then
  echo
  echo "All three signals pass ($BRANCH @ $SHA, dirty=$DIRTY)."
else
  exit 1
fi
