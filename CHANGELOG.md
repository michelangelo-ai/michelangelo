# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

### Added

- To be populated.

### Changed

- **BREAKING: Pipeline delete now cascades by default.** Deleting a Pipeline drains and deletes its
  child PipelineRuns and TriggerRuns (foreground propagation), with no feature flag; run history is
  retained in MySQL. Opt out per delete with `kubectl delete pipeline … --cascade=orphan`. GC deletes
  children with the controller's RBAC, and the MA Studio UI does not yet cascade. See the
  [Cascade Delete operator guide](docs/operator-guides/cascade-delete.md).

### Fixed

- `train_tabular()` no longer defaults `RunConfig.storage_path` to a local
  tempdir. The default `RunConfig` is now built by the shared
  `michelangelo.uniflow.plugins.ray.create_run_config()` helper, which
  resolves `storage_path`/`storage_filesystem` from `UF_STORAGE_URL` (the
  same variable `DatasetVariable`/`ModelVariable` already use), so worker
  pods on a multi-node Ray cluster share checkpoint storage with the head
  pod. Falls back to a local tempdir when `UF_STORAGE_URL` is unset.
- `train_tabular()` now returns the trained model as a `ModelVariable`
  instead of eagerly packaging and uploading it as a `ModelArtifact`. The
  trained model is an intra-pipeline intermediate, not a registry-ready
  artifact — packaging and uploading into the consolidated model manager /
  artifact store is the assembler/pusher's job. The now-unused
  `storage_backend` parameter has been removed from `train_tabular()`;
  warm-start weights for `initial_model` are read directly from
  `ModelArtifact.path` (always a local filesystem path per its contract),
  with no storage backend involved.

### Removed

- The `controllermgr.cascadeDelete.enable` Helm value and its associated config (cross-binary cascade
  config key, controller-manager `CascadeDeleteConfig`). Cascade is now always on and controlled by
  Kubernetes propagation policy + RBAC instead of a flag.
