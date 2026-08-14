#!/usr/bin/env bash
# Prints the URL to open once to set a local dev identity (name/email/avatar) from GitHub.
# Usage: ./scripts/set-dev-avatar.sh <github-username> [base-url]
set -euo pipefail

if [ $# -lt 1 ] || [ -z "$1" ]; then
  echo "Usage: $0 <github-username> [base-url]" >&2
  echo "  base-url defaults to http://localhost:8090 (the sandbox UI)." >&2
  exit 1
fi

BASE_URL="${2:-http://localhost:8090}"

echo "Open this once in your browser (works against a running dev server or the sandbox UI):"
echo ""
echo "  $BASE_URL/?ghUser=$1"
echo ""
echo "The app fetches your public GitHub profile once and caches your name, email, and avatar"
echo "in localStorage, so a normal reload afterward keeps showing them."
echo "No rebuild or restart needed."
