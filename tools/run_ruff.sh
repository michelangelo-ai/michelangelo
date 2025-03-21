#!/bin/bash
set -euo pipefail

MODE="$1"; shift

# Use awk to transform file paths:
# - Remove leading 'python/' if present
# - Else, prepend '../'
FILES=$(printf "%s\n" "$@" | awk '
  {
    if ($0 ~ /^python\//) {
      sub(/^python\//, "", $0)
      print
    } else {
      print "../" $0
    }
  }
')

# Convert string to array for proper quoting
readarray -t FILE_ARRAY <<< "$FILES"

# Run ruff with given mode and transformed paths
echo "Running \`ruff $MODE\` on" "${FILE_ARRAY[@]}"
poetry run --directory python ruff "$MODE" "${FILE_ARRAY[@]}"
