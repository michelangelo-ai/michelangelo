# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

## [0.5.0] - 2026-07-20

### Breaking Changes


- **controller:** Pipeline delete now cascades by default (foreground propagation, no flag). The `controllermgr.cascadeDelete.enable` Helm value and associated config are removed. Opt out per-delete with `kubectl delete pipeline … --cascade=orphan`.


- **python:** `CometParam` and `LightningTrainerParam.comet_param` removed outright. Use `ExperimentTrackerConfig(tracker=CometConfig(...))` instead.


### Features


- **ui:** Add multi-value tag input to StringField (#1511)


- **sandbox:** Provision REGISTRY_ENDPOINT for pipeline task pods (#1533)


- **ui:** Export useStudioMutation and MutationConfig from core (#1536)


- **python:** CustomTrackerConfig on ExperimentTrackerConfig: bring-your-own experiment tracker via dotted-path factory_fn/factory_kwargs, for trackers with no dedicated config class (W&B, Neptune, etc.)


- **python:** ExperimentTrackerConfig.tracker unified entry point for CometConfig/MlflowConfig/CustomTrackerConfig; legacy comet=/mlflow= fields still work and are promoted internally


- **python:** MlflowConfig fully supported — use ExperimentTrackerConfig(tracker=MlflowConfig(...)) to log to MLflow. Closes #1427.


- **python:** build_comet_logger / build_mlflow_logger factory functions in michelangelo.lib.trainer.torch.pytorch_lightning._private.util, usable as CustomTrackerConfig.factory_fn targets


### Bug Fixes


- **helm:** Allow x-user-email header in envoy CORS preflight (#1552, #1553)


- **helm:** Envoy checksum annotation + missing x-user-name CORS header (#1558, #1559)


- **scripts:** Pin internal @michelangelo-ai workspace deps on version bump (#1507)


- **ci:** Publish npm packages when cutting an RC, not just on promote (#1508)


- **ci:** Harden CI gate + fix changelog generation for bot-pushed tags (#1509)


- **python:** Fall back to MA_NAMESPACE for california_housing_xgb push_step (#1447)


- **ci:** Fix npm-publish.yml build order, prerelease tag, auth, and provenance (#1510)


- **ci:** Trigger Go/UI container publish for RC/final tags; fix container tag format (#1513)


- **python:** Explicitly set local_rank in RayTrainReportCallback (#1519)


- **helm:** Tie first-party image tags to chart appVersion (#1515)


- **ci:** Exclude release-promote's own jobs from its CI-green check (#1535)


- **core:** Re-export UserProvider for custom provider trees (#1523)


- Fix the trigger notification (#1534)


- **python:** MlflowConfig.tracking_uri is now optional (str | None); falls back to MLFLOW_TRACKING_URI env var


### Documentation


- Fix broken UPGRADING.md link in CONTRIBUTING.md (#1520)


- Fix broken links flagged by doc quality scanner (#1529)


### Miscellaneous


- Add user identity to NavigationBar (#1504)


- Add user identity headers to RPC request contract (#1512)


- Populate ColumnMeta augmentation to eliminate columnDef.meta casts (#1497)


- Type CELL_RENDERERS registry with per-entry value types (#1517)


- Docs/workflow patterns (#1527)


- Uniflow workflow patterns runnable example (#1524)


- Print help panel on 'ma' / 'ma -h' with prog='ma' (#1530)





## [0.4.0] - 2026-07-10


### Bug Fixes


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

