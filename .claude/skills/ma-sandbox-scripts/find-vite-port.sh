#!/usr/bin/env bash
# Finds or starts a Vite dev server for the current branch's worktree.
# On success: prints "<port> <pid> <started|existing>" and exits 0.
#   started  — we started this server; caller should offer to kill it when done
#   existing — server was already running; caller should leave it alone
# On failure: prints an error to stderr and exits 1.

set -euo pipefail

CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
WORKTREE_ROOT=$(git rev-parse --show-toplevel)

# --- Try to find an existing server for this branch ---
for port in 5173 5174 5175 5176 5177 5178 5179 5180; do
  PID=$(lsof -ti ":$port" 2>/dev/null | head -1)
  [ -z "$PID" ] && continue
  PORT_CWD=$(lsof -p "$PID" 2>/dev/null | awk '$4=="cwd" {print $NF}')
  [ -z "$PORT_CWD" ] && continue
  PORT_ROOT=$(git -C "$PORT_CWD" rev-parse --show-toplevel 2>/dev/null || true)
  PORT_BRANCH=$(git -C "$PORT_CWD" rev-parse --abbrev-ref HEAD 2>/dev/null || true)
  if [ "$PORT_ROOT" = "$WORKTREE_ROOT" ] && [ "$PORT_BRANCH" = "$CURRENT_BRANCH" ]; then
    echo "$port $PID existing"
    exit 0
  elif [ -n "$PORT_BRANCH" ] && [ "$PORT_BRANCH" != "$CURRENT_BRANCH" ]; then
    echo "Port $port has Vite for branch '$PORT_BRANCH' (need '$CURRENT_BRANCH') — skipping." >&2
  fi
done

# --- No existing server found — start one ---
JS_DIR="$WORKTREE_ROOT/javascript"
if [ ! -d "$JS_DIR" ]; then
  echo "javascript/ directory not found at $JS_DIR" >&2
  exit 1
fi

yarn --cwd "$JS_DIR" dev &>/tmp/vite-dev-$$.log &
SERVER_PID=$!

# Wait for Vite to bind to a port (up to 30 seconds).
# Detect by worktree root match rather than parent PID — Yarn/Vite forks
# deeply on macOS so the listening PID is rarely a direct child of SERVER_PID.
STARTED_PORT=""
STARTED_PID=""
for i in $(seq 1 30); do
  sleep 1
  # Check if the yarn process itself died early
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "Vite server process exited early. Log at /tmp/vite-dev-$$.log" >&2
    exit 1
  fi
  for port in 5173 5174 5175 5176 5177 5178 5179 5180; do
    PID=$(lsof -ti ":$port" 2>/dev/null | head -1)
    [ -z "$PID" ] && continue
    PORT_CWD=$(lsof -p "$PID" 2>/dev/null | awk '$4=="cwd" {print $NF}')
    [ -z "$PORT_CWD" ] && continue
    PORT_ROOT=$(git -C "$PORT_CWD" rev-parse --show-toplevel 2>/dev/null || true)
    if [ "$PORT_ROOT" = "$WORKTREE_ROOT" ]; then
      STARTED_PORT=$port
      STARTED_PID=$SERVER_PID
      break 2
    fi
  done
done

if [ -z "$STARTED_PORT" ]; then
  echo "Vite server did not start within 30 seconds. Log at /tmp/vite-dev-$$.log" >&2
  kill "$SERVER_PID" 2>/dev/null || true
  exit 1
fi

echo "$STARTED_PORT $STARTED_PID started"
