# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

### Added

- `CustomTrackerConfig` on `ExperimentTrackerConfig`: bring-your-own experiment
  tracker via a dotted-path factory function (`factory_fn`/`factory_kwargs`),
  for trackers (W&B, Neptune, etc.) with no dedicated config class.
- `ExperimentTrackerConfig.tracker` field, a unified entry point for
  `CometConfig`/`MlflowConfig`/`CustomTrackerConfig` (the legacy `comet=`/
  `mlflow=` fields still work and are promoted into `tracker` internally).
- `build_comet_logger` / `build_mlflow_logger` factory functions in
  `michelangelo.lib.trainer.torch.pytorch_lightning._private.util`, usable
  as `CustomTrackerConfig.factory_fn` targets.

### Changed

- **BREAKING: Pipeline delete now cascades by default.** Deleting a Pipeline drains and deletes its
  child PipelineRuns and TriggerRuns (foreground propagation), with no feature flag; run history is
  retained in MySQL. Opt out per delete with `kubectl delete pipeline … --cascade=orphan`. GC deletes
  children with the controller's RBAC, and the MA Studio UI does not yet cascade. See the
  [Cascade Delete operator guide](docs/operator-guides/cascade-delete.md).
- `MlflowConfig` now fails fast with `ConfigurationError` at construction time
  (previously it raised `NotImplementedError` later, from inside
  `train_tabular()`). The error message points at GitHub issue #1427 and the
  `CustomTrackerConfig` + `build_mlflow_logger` workaround usable today.
- `MlflowConfig.tracking_uri` is now optional (`str | None`, defaults to
  `None`), falling back to the `MLFLOW_TRACKING_URI` environment variable.

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
  artifact store is a downstream packaging task's job (no such task exists
  in OSS yet). The now-unused `storage_backend` parameter has been removed
  from `train_tabular()`; for a lightning warm-start, `initial_model.path`
  must now point directly to the local state-dict file (e.g. as written by
  `ModelVariable.save_lightning_model()`), not a directory — this matches
  what `LightningTrainerParam.initial_weights_path` always expected. No
  storage backend is involved, and a missing file now raises
  `ConfigurationError` eagerly instead of failing deep inside Ray Train.

### Removed

- **BREAKING: `CometParam` and `LightningTrainerParam.comet_param` have been
  removed outright**, not deprecated. The original design called for a
  deprecation cycle, but a repo-wide audit found `task.py` was the only
  production caller and it is migrated onto the new `ExperimentTrackerConfig`
  tracker abstraction in this same release — there is no external consumer to
  break. Use `ExperimentTrackerConfig(tracker=CometConfig(...))` instead.
- The `controllermgr.cascadeDelete.enable` Helm value and its associated config (cross-binary cascade
  config key, controller-manager `CascadeDeleteConfig`). Cascade is now always on and controlled by
  Kubernetes propagation policy + RBAC instead of a flag.
