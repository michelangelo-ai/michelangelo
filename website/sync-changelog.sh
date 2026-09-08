#!/usr/bin/env bash
# Regenerate docs/about/changelog.md from the root CHANGELOG.md.
#
# CI runs this before building the site so the published /changelog page
# always matches the release changelog. The generated file is also
# committed as a snapshot so local dev servers and preview builds work
# without an extra step; expect CI to refresh it at deploy time.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
src="$repo_root/CHANGELOG.md"
dst="$repo_root/docs/about/changelog.md"

{
  cat <<'HEADER'
---
title: Changelog
slug: /changelog
sidebar_position: 2
description: What shipped in each Michelangelo AI release.
format: md
---

<!-- GENERATED FILE - do not edit by hand.
     Source of truth is CHANGELOG.md at the repository root;
     regenerate with website/sync-changelog.sh. -->

HEADER
  # Drop the source's "# Changelog" H1; the page title comes from front matter.
  tail -n +2 "$src"
} > "$dst"

echo "wrote $dst from $src"
