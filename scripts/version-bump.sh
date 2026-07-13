#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

VERSION_FILES=(
  "python/pyproject.toml"
  "javascript/packages/core/package.json"
  "javascript/packages/rpc/package.json"
  "javascript/app/package.json"
  "helm/michelangelo/Chart.yaml"
)

usage() {
  cat <<USAGE
Usage: $(basename "$0") <VERSION> | --check

  <VERSION>   Set all component versions to VERSION (e.g. 0.4.0)
  --check     Print current versions and exit 1 if they are misaligned

Examples:
  $(basename "$0") 0.4.0
  $(basename "$0") --check
USAGE
  exit 1
}

get_version() {
  local file="$1"
  case "$file" in
    *.toml)
      grep '^version' "$REPO_ROOT/$file" | head -1 | sed 's/.*"\(.*\)".*/\1/'
      ;;
    *.json)
      grep '"version"' "$REPO_ROOT/$file" | head -1 | sed 's/.*"\([0-9][^"]*\)".*/\1/'
      ;;
    *Chart.yaml)
      grep '^version:' "$REPO_ROOT/$file" | head -1 | sed 's/version: *//'
      ;;
  esac
}

get_app_version() {
  grep '^appVersion:' "$REPO_ROOT/helm/michelangelo/Chart.yaml" | head -1 | sed 's/appVersion: *"\(.*\)"/\1/'
}

check_versions() {
  local all_match=true
  local first_version=""

  echo "Current versions:"
  echo "─────────────────────────────────────────────────"
  for file in "${VERSION_FILES[@]}"; do
    ver=$(get_version "$file")
    printf "  %-50s %s\n" "$file" "$ver"
    if [ -z "$first_version" ]; then
      first_version="$ver"
    elif [ "$ver" != "$first_version" ]; then
      all_match=false
    fi
  done

  app_ver=$(get_app_version)
  printf "  %-50s %s\n" "helm/michelangelo/Chart.yaml (appVersion)" "$app_ver"
  if [ "$app_ver" != "$first_version" ]; then
    all_match=false
  fi

  echo "─────────────────────────────────────────────────"
  if [ "$all_match" = true ]; then
    echo "All versions aligned: $first_version"
    return 0
  else
    echo "ERROR: versions are misaligned"
    return 1
  fi
}

set_version() {
  local new_version="$1"

  if ! echo "$new_version" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$'; then
    echo "ERROR: '$new_version' is not a valid semver version"
    echo "Expected format: MAJOR.MINOR.PATCH or MAJOR.MINOR.PATCH-prerelease"
    exit 1
  fi

  echo "Bumping all components to $new_version"
  echo "─────────────────────────────────────────────────"

  # python/pyproject.toml
  sed -i.bak "s/^version = \".*\"/version = \"$new_version\"/" "$REPO_ROOT/python/pyproject.toml"
  rm -f "$REPO_ROOT/python/pyproject.toml.bak"
  printf "  %-50s → %s\n" "python/pyproject.toml" "$new_version"

  # JSON package files
  for file in javascript/packages/core/package.json javascript/packages/rpc/package.json javascript/app/package.json; do
    sed -i.bak "s/\"version\": \"[^\"]*\"/\"version\": \"$new_version\"/" "$REPO_ROOT/$file"
    rm -f "$REPO_ROOT/$file.bak"
    printf "  %-50s → %s\n" "$file" "$new_version"
  done

  # Pin internal @michelangelo-ai/* workspace deps to the exact new version.
  # A bare "*" range never matches a prerelease version under node-semver, so
  # yarn's workspace resolver falls back to the npm registry and fails when
  # the version being bumped to has a -rc./-nightly. suffix. Pinning keeps
  # local resolution working regardless of prerelease suffixes.
  sed -i.bak \
    -e "s/\"@michelangelo-ai\/rpc\": \"[^\"]*\"/\"@michelangelo-ai\/rpc\": \"$new_version\"/" \
    -e "s/\"@michelangelo-ai\/core\": \"[^\"]*\"/\"@michelangelo-ai\/core\": \"$new_version\"/" \
    "$REPO_ROOT/javascript/app/package.json"
  rm -f "$REPO_ROOT/javascript/app/package.json.bak"
  printf "  %-50s → %s\n" "javascript/app/package.json (internal deps)" "$new_version"

  sed -i.bak "s/\"@michelangelo-ai\/core\": \"[^\"]*\"/\"@michelangelo-ai\/core\": \"$new_version\"/" \
    "$REPO_ROOT/javascript/packages/rpc/package.json"
  rm -f "$REPO_ROOT/javascript/packages/rpc/package.json.bak"
  printf "  %-50s → %s\n" "javascript/packages/rpc/package.json (internal deps)" "$new_version"

  # Helm Chart.yaml — version and appVersion
  sed -i.bak "s/^version: .*/version: $new_version/" "$REPO_ROOT/helm/michelangelo/Chart.yaml"
  sed -i.bak "s/^appVersion: .*/appVersion: \"$new_version\"/" "$REPO_ROOT/helm/michelangelo/Chart.yaml"
  rm -f "$REPO_ROOT/helm/michelangelo/Chart.yaml.bak"
  printf "  %-50s → %s\n" "helm/michelangelo/Chart.yaml (version)" "$new_version"
  printf "  %-50s → %s\n" "helm/michelangelo/Chart.yaml (appVersion)" "$new_version"

  echo "─────────────────────────────────────────────────"
  echo "Done. Verify with: $(basename "$0") --check"
}

if [ $# -ne 1 ]; then
  usage
fi

case "$1" in
  --check)
    check_versions
    ;;
  --help|-h)
    usage
    ;;
  *)
    set_version "$1"
    ;;
esac
