#!/usr/bin/env bash
# Print a fingerprint of the Go sources a local build would compile: the go/
# tree at HEAD, plus uncommitted changes and untracked files under go/.
# deploy-local.sh records it on the deployment; check-backend.sh compares it.
#
# Usage: source-fingerprint.sh [<repo>]   (default: the current checkout)

set -euo pipefail

REPO=${1:-$(git rev-parse --show-toplevel)}

{
  git -C "$REPO" rev-parse HEAD:go
  git -C "$REPO" diff HEAD -- go
  git -C "$REPO" ls-files --others --exclude-standard -- go | sort | while IFS= read -r f; do
    echo "$f"
    git hash-object "$REPO/$f"
  done
} | git hash-object --stdin
