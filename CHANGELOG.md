# Changelog

All notable changes to this project will be documented in this file.

## [0.9.0] - 2026-08-24


### Bug Fixes


- Reword ma-design-interview skill to reduce duration (#1714)


- **build:** Bump Go SDK to 1.26.3 + rules_go to 0.57.0 (CVE Scan fix, step 1/2) (#1567)


- **python:** Restore ColumnConfig.shape optional default (#1762)


- **ci:** Skip JavaScript coverage PR comment on fork pull requests (#1756)


- **python:** Wire up workflowClient.executionUrlFormat for the sandbox (#1761)


- **core:** Correct Model list columns for description, environment, type, last updated (#1673)


- **storage/mysql:** Validate criterion field name before SQL interpolation (#1707)


- **storage/mysql:** Implement directUpdate for the generic Update RPC (#1645)


- **ui:** Block entity navigation for disabled/coming-soon phases in menu drawer (#1791)


- **release:** Mark -rc tags as prereleases in GitHub releases (#1678)


- **api:** Log blob-storage delete failure only when it fails (#1679)


- **helm:** Drop placeholder signKey annotation from chart metadata (#1768)


- **api:** Use status.Error for non-format error messages (#1782)


- **core:** Exempt barrel files and test infra from react-refresh export rule (#1807)


- **go:** Sync paused TriggerRun schedules (#1827)


- **python:** Stop shipping test files in wheels and sdists (#1832)


- Create TriggerRun schedules paused (#1828)


- Make TriggerRun deletion idempotent (#1688)


- **ui:** Tighten breadcrumb menu drawer close button and icons (#1799)


- **core:** Stop capitalizing entity/phase names (#1846)


- **core:** Title-case Data Sources, Evaluations, and Notebooks entity names (#1857)


- **python:** Use __spec__.name for dot_path() under python -m (#1754)


- **go:** Pipelinerun revision resolve and pipeline latest revision (#1797)


- **python:** Render <crd> get <name> as table and fix -o yaml OrderedDict crash (#1822)


- **python:** Rename evaluation_strategy to eval_strategy in bert_cola example (#1883)


- **docs,examples:** Correct Lightning trainer imports and parameters (#1787)


- Plumb RayTask gpu, disk and object store memory into the cluster spec (#1801)


- **release:** Align website version and add it to version-bump.sh (#1849)


- **ci:** Pin release.yaml's Go setup to go.mod (cherry-pick for v0.9.0-rc.1) (#1898)


### CI/CD


- Bump actions/setup-go from 5 to 7 (#1743)


- Bump actions/setup-python from 5 to 7 (#1744)


- Bump aquasecurity/trivy-action from 0.32.0 to 0.36.0 in the actions-minor-and-patch group (#1742)


- Add least-privilege permissions to workflows missing them (#1711)


- Pin third-party GitHub Actions to commit SHAs (#1757)


- Pin the golangci-lint installer script to the version it installs (#1767)


- Pin apidiff to a Go 1.24-compatible x/exp pseudo-version (#1781)


- **go:** Migrate golangci-lint config off deprecated options (#1755)


- **integration-test:** Run Playwright browser tests in sandbox-e2e (#1838)


- Upload coverage reports to Codecov (#1834)


- Add OpenSSF Scorecard workflow and badge (#1835)


- Build examples image without pushing on fork PRs (#1802)


- Pin sticky-pull-request-comment in javascript-coverage to a full SHA (#1810)


- Remove unused attestations permission from release workflows (#1811)


- Bump actions/cache from 4 to 6 (#1825)


- Bump azure/setup-helm from 4 to 5 (#1823)


### Documentation


- Fix community on-ramps in CONTRIBUTING and docs (#1713)


- **core:** Document action rendering surfaces and mutation payload flow (#1722)


- **core:** Consolidate javascript skills into CLAUDE.md (#1724)


- Correct Ray Tune and Ray autoscaling claims (#1788)


- Repoint california_housing doc references to michelangelo-examples (#1749)


- Add a migration guide for KubeRay users (#1783)


- Revert getting-started.md tutorial to xgb_train (pytorch_train crashes on Apple Silicon) (#1871)


- Update CHANGELOG.md for v0.9.0-rc.2 (#1906)


### Features


- Export success operations from core (#1716)


- **native_transform:** Add fitted-statistics layers (PR B3) (#1683)


- **native_transform:** Add IDHashTokenizer wrapper layer (PR B4) (#1684)


- **native_transform:** Add Pydantic layer specs (PR B5) (#1685)


- **js:** Config-driven form engine skeleton (#1650)


- **native_transform:** Add TransformSpec DAG engine + TorchTransformModule (PR B6) (#1730)


- **native_transform:** Add auxiliary transform helpers (PR B8) (#1729)


- **ui:** Show Coming soon badge on phase cards (#1785)


- **ui:** Change steps to ongoing operations (#1784)


- **ui:** Add delete pipeline functionality (#1792)


- **trainer:** Add an XGBoost trainer built on Ray Train (#1793)


- **js:** Schema-aware wrappers for all field types (#1651)


- **js:** Declarative validation for form config (#1652)


- **native_transform:** Add schema bridge for fitted transform DAGs (PR B7) (#1772)


- **native_transform:** Add TransformSpecIO + usage guide (PR E) (#1774)


- **ui:** Add deployment information tab (#1708)


- **ui:** Add status cards to deployment detail page (#1709)


- **ui:** Add source code link (#1826)


- Add GitHub-backed dev identity override to sandbox UI (#1819)


- **core:** Add Experiment & Productionize phase (#1813)


- **core:** Add Monitor & Debug coming-soon phase (#1815)


- Let EntityTable own phase pipeline-type filtering (#1830)


- **form:** Add conditional layout with the `is` operator (#1804)


- **form:** Add isNot/isEmpty/containsAny condition operators (#1805)


- **model:** Add Model API-hook wiring skeleton (Phase 0) (#1728)


- **native_transform:** Add Ray execution adapter (PR C) (#1773)


- **blobstore:** Add a native GCS backend (#1837)


- **train:** Add FSDP2 strategy to the Lightning trainer (#1877)


### Miscellaneous


- Add ma-sandbox-test-plan Claude Code skill (#1641)


- Add dependabot config for weekly grouped dependency updates (#1691)


- Bump gazelle 0.40.0 to 0.52.2, compatible with rules_go 0.57.0 (#1747)


- **lib:** Update california_housing example references in model_manager registry docstrings/tests (#1750)


- Add Algolia crawler domain verification token to robots.txt (#1766)


- Merge back release/v0.8 to main (#1721)


- Add CODEOWNERS file (#1796)


- Route major feature requests to the enhancements RFC process (#1803)


- Bump vllm to 0.9.2 to shed known CVEs (#1812)


- Support the KubeRay suspended cluster state in the MA API (#1700)


- Bump version to 0.9.0-rc.1 (#1889)


- Bump version to 0.9.0-rc.2 (#1905)


- Release 0.9.0 (#1923)


### Refactoring


- Delete dead code in python/michelangelo/canvas/ (#1727)


- **js:** Move form types to types directory (#1671)


- **core:** Rename ErrorView to Signpost (#1786)

## [0.8.0] - 2026-08-10

### ⚠ Breaking Changes

- **python:** Add kind to ModelPusherPlugin (#1687) — add kind to ModelPusherPlugin (#1687)


### Bug Fixes


- **website:** Update footer copyright to "Michelangelo AI" (#1664)


- **pusher:** Use the shared api.v2 name generator for auto-generated model names (#1633)


- **ui:** Tighten spacing and fix stuck focus state on multi-string field (#1681)


- **go:** Sync TriggerRun inputs to Temporal schedules (#1663)


- **ci:** Widen Cadence worker-connect wait window to 5 min (#1686)


- **docs:** Update stale routing source links in integrate-custom-backend (#1625)


- **ui:** Stop ArrayFormGroup spacing from depending on parent layout (#1682)


- **ci:** Fix Go and Python lint and test checks for fork PRs (#1609)


- Cherry-pick #1762 (restore ColumnConfig.shape default) + bump to 0.8.0-rc.2 (#1763)


### CI/CD


- **python:** Run michelangelo-examples CLI e2e tests in integration-test workflow (#1665)


### Documentation


- Add MLOps glossary page (#1628)


- Sync CHANGELOG.md [0.7.0] section to main (#1677)


- **security:** Update supported versions for 0.7.x (#1680)


- Add API service stubs for 10 undocumented proto services (#1587)


- **serving:** Add DeleteXxxCollection to serving API table footnote (#1689)


- Add Docs badge to README badge row (#1696)


- Add Ask DeepWiki badge to README (#1630)


- **pipelines:** Clarify architectural difference between remote-run and dev-run (#1697)


- Update CHANGELOG.md for v0.8.0-rc.1 (#1719)


- Update CHANGELOG.md for v0.8.0-rc.2 (#1764)


### Features


- **pipelinerun:** Add status reconstruction from workflow engine (#1297)


- Add design-interview skill for structured UX design (#1676)


- **native_transform:** Add structural/shape layers (PR B2) (#1603)


- **trainer:** Re-introduce profiler subsystem (PR 5) (#1614)


- **python:** Add kind to ModelPusherPlugin (#1687) [**BREAKING**]


- **python:** Migrate tabular_assembler to OSS (#1430)


- **trainer:** Pluggable MLflow profiler sink (#1702)


### Miscellaneous


- Merge back release/v0.7 to main (#1660)


- Add content index annotation (#1501)


- Add google.protobuf.Duration support to protoc-gen-validation (#1694)


- Add LinksBox and MessageContainer components (#1701)


- Remove dead internal tracker links from code comments (#1712)


- Bump version to 0.8.0-rc.1 (#1717)


- Promote v0.8.0 (version strip from 0.8.0-rc.2) (#1769)

## [0.7.0] - 2026-08-03


### Bug Fixes


- **docs:** Correct double-suffix branding in README and examples (#1596)


- **ingester:** Soft-delete metadata-storage row on kubectl delete for non-opted-in kinds (#1585)


- **ci:** Changelog.yml CR_PAT token + stop full-history CHANGELOG.md regeneration (#1599)


- **ci:** Nightly.yml npm/helm jobs no longer hardcode stale base version 0.3.0 (#1617)


- **storage/mysql:** Validate OrderBy.Field identifier before SQL interpolation (#1616)


- **sandbox:** Pin Grafana image to 13.1.1 instead of :latest (CVE-2026-31789) (#1623)


- **build:** Bump pinned Node.js 24.14.1 -> 24.18.0 (Wiz-reported CVEs) (#1621)


- **uniflow:** Propagate namespace to Ray cluster spec (#1605)


### CI/CD


- **integration-test:** Trigger only from Nightly Build (#1613)


### Documentation


- Refresh governance files and issue templates (#1579)


- Add missing libomp macOS prerequisite to getting-started guide (#1253)


- Update CHANGELOG.md for v0.7.0-rc.1 (#1659)


### Features


- **trainer:** Add pluggable ExperimentStore for auto-resume (PR 4) (#1571)


- **python:** Add server-side --dry-run to apply, create, pipeline run, pipelinerun kill (#1591)


- **python:** Additional_columns + filter_field_map hooks on CRD (#1588)


- **python:** Framework -r/--root and -R/--recursive on apply+create (#1590)


- **python:** --owner and --type filters + OWNER/TYPE columns on pipeline get (#1618)


- **python:** --actor and --revision filters + REVISION/USER/ENVIRONMENT/STATE columns on pipeline_run get (#1619)


- **python:** Retry + round_robin service_config on mactl gRPC channels (#1615)


- **python:** --pipeline/--model/--deployment/--owner filters + 3 columns on revision get (#1620)


- **core:** Expose mutationName in success operations resolver context (#1646)


- **core:** Set mutationKey on useStudioMutation for MutationCache integration (#1647)


### Miscellaneous


- Update hero subtitle to "Now open source." (#1606)


- Merge back release/v0.6 to main (#1597)


- Bind k3d port-publishes to loopback instead of 0.0.0.0 (#1592)


- Fix site title, meta description, and favicon for search results (#1624)


- Bump version to 0.7.0-rc.1 (#1657)


- Release 0.7.0 (#1666)

## [0.6.0] - 2026-07-27


### Bug Fixes


- **helm:** Allow x-user-email header in envoy CORS preflight (#1552)


- **helm:** Envoy checksum annotation + missing x-user-name CORS header (#1558)


- **release:** Exclude RC/nightly tags from changelog diff boundary, commit CHANGELOG.md via PR (#1561)


- **docs:** Wrap Spark DataFrame in DatasetVariable in branching example (#1566)


- **rayjob:** Right-size submitter pod instead of cloning the head (#1562)


- **ci:** Changelog.yml CR_PAT token + stop full-history CHANGELOG.md regeneration (cherry-pick #1599) (#1602)


### Documentation


- Add v0.5.0 changelog entry (#1569)


- Standardize branding to "Michelangelo AI" in READMEs and PyPI (#1577)


- Standardize branding to "Michelangelo AI" across all docs (#1575)


- Update CHANGELOG.md for v0.6.0-rc.1 (#1600)


### Features


- **native_transform:** Add pyarrow conversion helpers (PR A) (#1543)


- **sandbox:** Support --set on ma sandbox create (#1554)


- **native_transform:** Add IDHashTokenizer torch layer (PR A2) (#1548)


- Add MetadataStoragePrimaryKey annotation for k8s migration (#1332)


- **python:** Expose clean_reason and clean_details on GitInfo (F037) (#1572)


- **native_transform:** Add foundation transform layers (PR B1) (#1570)


### Miscellaneous


- Quote templated image/secret-name values in core Deployment charts (#1544)


- Add examples gallery page with all 9 working examples (#1469)


- **core:** Upgrade baseui 15→18, move to peerDependencies (#1522)


- Merge back release/v0.5 to main (#1547)


- Remove stray test artifact python/create-project.json (#1476)


- Ma cli: add -o, -A, and default DESC sort to <crd> get (#1576)


- **release:** Prepare v0.6.0


- Bump version to 0.6.0-rc.1 (#1595)


- Release 0.6.0 (#1607)


### Refactoring


- **core:** Split public API from primitives entrypoint (#1526)


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
