#!/usr/bin/env bash
# Detects whether Go service code changed on this branch and whether the
# affected services are running a locally built image.
#
# "Changed" means committed since the merge-base with origin/main, plus staged,
# unstaged and untracked files. A service is affected when a changed file lives
# in a package its binary depends on (resolved with `go list -deps`), not only
# under go/cmd/<service>/. A go.mod/go.sum change affects every service.
#
# Prints one line per affected service and exits 0 always:
#
#   deployed <service>        — affected AND the local image is running in k3d
#   needs-deploy <service>    — affected but a non-local image is running
#
# or a single `clean` line when no service is affected (Vite-only verify is
# appropriate). The caller decides how to proceed from the output.

set -euo pipefail

REPO_ROOT=$(git rev-parse --show-toplevel)
SERVICES=(controllermgr worker apiserver)

BASE=$(git -C "$REPO_ROOT" merge-base HEAD origin/main 2>/dev/null \
  || git -C "$REPO_ROOT" merge-base HEAD main 2>/dev/null \
  || echo HEAD)

# `git diff <base>` compares the working tree to <base>, so it covers committed
# and uncommitted changes to tracked files; untracked files are added separately.
CHANGED_FILES=$(
  { git -C "$REPO_ROOT" diff --name-only "$BASE" 2>/dev/null
    git -C "$REPO_ROOT" ls-files --others --exclude-standard 2>/dev/null
  } | sort -u
)

service_affected() {
  local svc=$1 file dir pkg_dirs
  if echo "$CHANGED_FILES" | grep -qE '^go/go\.(mod|sum)$'; then return 0; fi
  # Directories of every in-module package the binary links, relative to the repo root.
  if ! pkg_dirs=$(cd "$REPO_ROOT/go" && go list -deps -f '{{if .Module}}{{if .Module.Main}}{{.Dir}}{{end}}{{end}}' "./cmd/$svc" 2>/dev/null); then
    # Can't resolve dependencies: any Go change is treated as affecting this service.
    echo "go list failed for $svc; treating any go/ change as affecting it" >&2
    echo "$CHANGED_FILES" | grep -qE '^go/'
    return
  fi
  pkg_dirs=$(echo "$pkg_dirs" | sed "s#^$REPO_ROOT/##")
  while IFS= read -r file; do
    [ -n "$file" ] || continue
    case "$file" in
      "go/cmd/$svc/"*) return 0 ;;  # includes non-package files such as config/
    esac
    dir=$(dirname "$file")
    if echo "$pkg_dirs" | grep -qxF "$dir"; then return 0; fi
  done <<< "$CHANGED_FILES"
  return 1
}

AFFECTED=()
for svc in "${SERVICES[@]}"; do
  if service_affected "$svc"; then AFFECTED+=("$svc"); fi
done

if [ ${#AFFECTED[@]} -eq 0 ]; then
  echo "clean"
  exit 0
fi

for svc in "${AFFECTED[@]}"; do
  RUNNING_IMAGE=$(kubectl get pod -l "app.kubernetes.io/component=$svc" \
    -o jsonpath='{.items[0].spec.containers[0].image}' 2>/dev/null || echo "")
  if echo "$RUNNING_IMAGE" | grep -q ':local'; then
    echo "deployed $svc"
  else
    echo "needs-deploy $svc"
  fi
done
