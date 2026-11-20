#!/usr/bin/env bash
# Verifies the tooling needed to commit Python changes in this repo: ruff
# runnable through the python venv (the `dev` extra) and the pre-commit hook
# installed in the git dir.
#
# Usage: check-dev-prereqs.sh
# Prints `ok` and exits 0 when both are present; otherwise prints one line
# naming the fix command and exits 1.

set -euo pipefail

REPO_ROOT=$(git rev-parse --show-toplevel)

if ! poetry run --directory "$REPO_ROOT/python" ruff --version >/dev/null 2>&1; then
  echo "ruff is missing from the python venv: run 'poetry install -E dev' in $REPO_ROOT/python" >&2
  exit 1
fi

HOOK=$(git -C "$REPO_ROOT" rev-parse --git-path hooks/pre-commit)
case "$HOOK" in /*) ;; *) HOOK="$REPO_ROOT/$HOOK" ;; esac
if [ ! -x "$HOOK" ]; then
  echo "pre-commit hook is not installed: run 'poetry run pre-commit install' in $REPO_ROOT/python" >&2
  exit 1
fi

echo ok
