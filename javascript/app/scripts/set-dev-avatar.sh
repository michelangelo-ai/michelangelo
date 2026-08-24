#!/usr/bin/env bash
# Prints the URL to open once to set a local dev identity (name/email/avatar) from GitHub.
# Usage: ./scripts/set-dev-avatar.sh <github-username> [--email <email>] [base-url]
set -euo pipefail

if [ $# -lt 1 ] || [ -z "$1" ]; then
  echo "Usage: $0 <github-username> [--email <email>] [base-url]" >&2
  echo "  base-url defaults to http://localhost:8090 (the sandbox UI)." >&2
  echo "  --email overrides the shown email directly (recommended: GitHub rarely exposes a" >&2
  echo "  public one, and the sandbox has no way to read your local git config)." >&2
  exit 1
fi

USERNAME="$1"
shift

EMAIL=""
if [ "${1:-}" = "--email" ]; then
  EMAIL="${2:?--email requires a value}"
  shift 2
fi

BASE_URL="${1:-http://localhost:8090}"

URL="$BASE_URL/?ghUser=$USERNAME"
if [ -n "$EMAIL" ]; then
  URL="$URL&email=$EMAIL"
fi

echo "Open this once in your browser (works against a running dev server or the sandbox UI):"
echo ""
echo "  $URL"
echo ""
echo "The app fetches your public GitHub profile once and caches your name, email, and avatar"
echo "in localStorage, so a normal reload afterward keeps showing them."
echo "No rebuild or restart needed."
