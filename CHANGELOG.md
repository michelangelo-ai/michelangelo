# Changelog

All notable changes to this project will be documented in this file.

## [0.4.0] - 2026-07-10


### Bug Fixes


- **helm,ci:** Nil pointer guards on upgrade + configurable integration-test ref (#1405)


- **ci:** Bypass poetry-dynamic-versioning in nightly Python build (#1406)


- **ci:** Add helm repo registration before dependency build (#1408)


- **ci:** Update npm workspace name in nightly workflow (#1407)


- Reflector error monitoring via DefaultWatchErrorHandler (#1319)


- **ci:** Fix python and npm nightly publish failures (#1414)


- **python:** Sanitize pre-existing model_manager leaks + fix broken custom-packager import (#1431)


- **trainer:** Default RunConfig storage to backend bucket on multi-node runs (#1441)


- **python:** Train_tabular returns ModelVariable, drops storage_backend (#1442)


- **ci:** CVE Scan fails to resolve trivy-action/trivy binary version (#1444)


- **spark:** Infer SparkApplication.Type from entrypoint instead of hardcoding Python (#1465)


- **python:** Demote oversized/invalid model registry labels to annotation (#1446)


- **sandbox:** Add missing minio-credentials secret and kuberay images (#1474)



### CI/CD


- Add Trivy CVE scanning workflow for container images (#1398)


- Add cross-compiled Go binary builds to release workflow (#1399)


- Add nightly artifact retention cleanup workflow (#1400)


- Add API surface change detection for Go, Proto, and Helm (#1402)


- Add compatibility testing matrix for Python, Node.js, and Helm (#1403)


- Configure Release Drafter for PR-based release notes (#1401)


- Notify Slack on scheduled/release workflow failures (#1438)



### Documentation


- Address sandbox setup feedback — timing, sync, missing prereqs, troubleshooting (#1247)



### Features


- **trainer:** Add TrainingObserver protocol for pluggable metrics observation (#1364)


- **triggerrun:** Make notification settings updatable post-creation (#1385)


- **trainer:** Add tabular_trainer config schema (PR 7a, issue #1359) (#1415)


- **ui:** Add description field to pipeline run form and list (#1422)


- **trainer:** Tabular_trainer pure helpers module (_dataset.py) [PR 7b] (#1421)


- **trainer:** Tabular_trainer dispatcher + ModelMetadata fields (PR 7c) (#1426)


- **trainer:** Flexible experiment tracking abstraction (#1432)


- **trainer:** Wire MLflow experiment tracking (PR 10) (#1434)


- **trainer:** Restore fused_model_submodule to warm-start schema (#1435)


- **revision:** Add pluggable Revision controller (#1314)



### Miscellaneous


- Add proxy_user to v2 Trigger message (#1374)


- Move Roadmap page to Getting Started section (#1412)


- Add Support & Community section to docs landing page (#1386)


- Add Dual-Track Pipeline and RFC process to contribution guides (#1387)


- Respect primary and secondary actions' disabled state (#1413)


- **precommit:** Add gofmt and go vet hooks for Go files (#1420)


- Rename pusher/pusher.py -> pusher/task.py; complete task.py convention (#1429)


- **examples:** Remove unused notebook_workflow example (#1439)


- Delete MAINTAINERS.md (#1449)


- Fold successOperations into useStudioMutation (#1455)


- Update intro.md (#1467)


- Add standalone Support & Community page (#1470)


- Add require-cast-comment rule; annotate all existing as-assertions (#1333)


- Replace connect_grpc_bridge with grpc_json_transcoder (#1468)


- **worker:** Remove stale implementation TODOs in ray/spark starlark plugins (#1475)


- Drop skill guidance now enforced by existing eslint rules (#1483)


- Add a route successOperation that skips the toast (#1485)



### Refactoring


- **trainer:** Move _dataset.py into a _private/ subpackage (#1433)


- **ui:** Centralize mutation middleware in the mutation hook (#1482)

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
- `MlflowConfig` is now fully supported — set
  `ExperimentTrackerConfig(tracker=MlflowConfig(...))` to log to MLflow.
  Closes GitHub issue #1427.
- `fused_model_submodule` field on `IncrementalTrainingSpec` and
  `TransferLearningSpec` (`lib/trainer/torch/pytorch_lightning/schema.py`),
  restoring schema-shape parity with the internal Uber SDK. Schema-only for
  now — no OSS code reads or acts on this field yet.

### Changed

- **BREAKING: Pipeline delete now cascades by default.** Deleting a Pipeline drains and deletes its
  child PipelineRuns and TriggerRuns (foreground propagation), with no feature flag; run history is
  retained in MySQL. Opt out per delete with `kubectl delete pipeline … --cascade=orphan`. GC deletes
  children with the controller's RBAC, and the MA Studio UI does not yet cascade. See the
  [Cascade Delete operator guide](docs/operator-guides/cascade-delete.md).
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
