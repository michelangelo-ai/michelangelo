---
name: Release Checklist
about: Track all steps for cutting a new release
title: 'Release vX.Y.Z'
labels: ['release']
assignees: ''
---

## Pre-Release

- [ ] All CI checks pass on the release branch
- [ ] Version numbers updated across all components:
  - [ ] `python/pyproject.toml`
  - [ ] `javascript/packages/core/package.json`
  - [ ] `javascript/packages/rpc/package.json`
  - [ ] `website/package.json`
  - [ ] `helm/michelangelo/Chart.yaml` (both `version` and `appVersion`)
- [ ] `CHANGELOG.md` updated via `git cliff`
- [ ] Release notes drafted (follows three-layer template: summary, categorized changes, compatibility matrix)
- [ ] Breaking changes reviewed and documented:
  - [ ] All `BREAKING CHANGE:` commits identified in changelog
  - [ ] Migration guide written for each breaking change
  - [ ] `UPGRADING.md` updated with migration steps
  - [ ] Deprecation warnings added for items being removed (see [Deprecation Policy](../../CONTRIBUTING.md#deprecation-policy))
- [ ] Compatibility matrix updated in release notes

## Release Candidate

- [ ] Release branch created: `release/vX.Y`
- [ ] RC tag pushed (e.g. `v0.3.0-rc.1`)
- [ ] RC artifacts published and accessible:
  - [ ] Python wheel on PyPI (PEP 440: `0.3.0rc1`)
  - [ ] Go service containers on ghcr.io
  - [ ] UI container on ghcr.io
  - [ ] npm packages (`@michelangelo/core`, `@michelangelo/rpc`)
  - [ ] Helm OCI chart on ghcr.io
- [ ] RC announcement posted (GitHub Discussions or relevant channel)
- [ ] Soak period complete (minimum 1 week for minor releases)
- [ ] No P0/P1 issues reported against RC

## Final Release

- [ ] Release tag pushed (e.g. `v0.3.0`)
- [ ] All artifacts published and accessible:
  - [ ] Python wheel on PyPI
  - [ ] Go service containers on ghcr.io (tagged `v0.3.0` + `latest`)
  - [ ] UI container on ghcr.io
  - [ ] npm packages (latest dist-tag)
  - [ ] Helm OCI chart on ghcr.io
- [ ] GitHub Release created with generated release notes
- [ ] Announcement posted
- [ ] Release branch merged back to main (if diverged)
- [ ] Next development version bumped on main (if applicable)
