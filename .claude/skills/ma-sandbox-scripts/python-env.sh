#!/usr/bin/env bash
# Ensure python/.venv exists on a supported interpreter (3.10-3.12) with the
# project's dependencies installed, so `python/.venv/bin/ma` works.
# Python 3.13 is avoided: onnxruntime 1.19.2 has no 3.13 wheel, so
# `poetry install` fails there.
#
# Usage: python-env.sh [--repo <path>] [--extras plugin]
# The last line of stdout is the path of the `ma` executable.

set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
EXTRAS=""
while [ $# -gt 0 ]; do
  case "$1" in
    --repo) REPO_ROOT=$(cd "$2" && pwd); shift 2 ;;
    --extras) EXTRAS="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

PY_DIR="$REPO_ROOT/python"
MA="$PY_DIR/.venv/bin/ma"
SUPPORTED='import sys; sys.exit(0 if (3,10) <= sys.version_info[:2] <= (3,12) else 1)'

venv_supported() {
  [ -x "$PY_DIR/.venv/bin/python" ] && "$PY_DIR/.venv/bin/python" -c "$SUPPORTED"
}

if venv_supported && [ -x "$MA" ] && "$MA" --help >/dev/null 2>&1 && [ -z "$EXTRAS" ]; then
  echo "$MA"
  exit 0
fi

find_python() {
  local v cand
  for v in 3.12 3.11 3.10; do
    for cand in "$HOME"/.pyenv/versions/"$v"*/bin/python "$(command -v "python$v" 2>/dev/null || true)"; do
      if [ -n "$cand" ] && [ -x "$cand" ]; then echo "$cand"; return 0; fi
    done
  done
  return 1
}

PYTHON=$(find_python) || { echo "No Python 3.10-3.12 found (try: pyenv install 3.12)." >&2; exit 1; }
echo "Using $PYTHON" >&2

cd "$PY_DIR"
# A venv on an unsupported interpreter (e.g. 3.13) can't be repaired in place.
if [ -d .venv ] && ! venv_supported; then
  rm -rf .venv
fi
poetry config virtualenvs.in-project true --local >&2
poetry env use "$PYTHON" >&2
if [ -n "$EXTRAS" ]; then
  poetry install --extras "$EXTRAS" >&2
else
  poetry install >&2
fi
echo "$MA"
