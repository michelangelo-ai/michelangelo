# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]


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


- **ci:** Release-cut/promote tags never triggered artifact publish (#1499)


- **ci:** Publish @michelangelo-ai/rpc to npm alongside core (#1500)


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


- **helm:** Allow x-user-email header in envoy CORS preflight (#1552)


- **helm:** Envoy checksum annotation + missing x-user-name CORS header (#1558)


- **release:** Exclude RC/nightly tags from changelog diff boundary, commit CHANGELOG.md via PR (#1561)


- **docs:** Wrap Spark DataFrame in DatasetVariable in branching example (#1566)


- **rayjob:** Right-size submitter pod instead of cloning the head (#1562)



### CI/CD


- Add Trivy CVE scanning workflow for container images (#1398)


- Add cross-compiled Go binary builds to release workflow (#1399)


- Add nightly artifact retention cleanup workflow (#1400)


- Add API surface change detection for Go, Proto, and Helm (#1402)


- Add compatibility testing matrix for Python, Node.js, and Helm (#1403)


- Configure Release Drafter for PR-based release notes (#1401)


- Notify Slack on scheduled/release workflow failures (#1438)


- Run CI on release/v* branches (#1491)


- **release:** Npm-publish workflow_call + PyPI PEP 440 normalization (#1492)


- **release:** Add release-cut.yml workflow (#1493)


- **release:** Add release-promote.yml workflow (#1494)



### Documentation


- Address sandbox setup feedback — timing, sync, missing prereqs, troubleshooting (#1247)


- Document release workflow in CONTRIBUTING.md (#1495)


- Fix broken UPGRADING.md link in CONTRIBUTING.md (#1520)


- Fix broken links flagged by doc quality scanner (#1529)


- Add v0.5.0 changelog entry (#1569)


- Standardize branding to "Michelangelo AI" in READMEs and PyPI (#1577)


- Standardize branding to "Michelangelo AI" across all docs (#1575)



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


- **ui:** Add multi-value tag input to StringField (#1511)


- **sandbox:** Provision REGISTRY_ENDPOINT for pipeline task pods (#1533)


- **ui:** Export useStudioMutation and MutationConfig from core (#1536)


- **native_transform:** Add pyarrow conversion helpers (PR A) (#1543)


- **sandbox:** Support --set on ma sandbox create (#1554)


- **native_transform:** Add IDHashTokenizer torch layer (PR A2) (#1548)


- Add MetadataStoragePrimaryKey annotation for k8s migration (#1332)


- **python:** Expose clean_reason and clean_details on GitInfo (F037) (#1572)


- **native_transform:** Add foundation transform layers (PR B1) (#1570)



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


- Extract configurable NavigationBar from CoreApp (#1486)


- **release:** Prepare v0.4.0 (#1489)


- Add isRecord type guard to replace typeof-object-and-cast pattern (#1487)


- Replace decorative casts in composeTableState with typed iteration (#1490)


- Strip pagination from table state when pagination is disabled (#1488)


- Update to the license docs page formatting/nesting (#1478)


- Add user identity to NavigationBar (#1504)


- Populate ColumnMeta augmentation to eliminate columnDef.meta casts (#1497)


- Add user identity headers to RPC request contract (#1512)


- Type CELL_RENDERERS registry with per-entry value types (#1517)


- Docs/workflow patterns (#1527)


- Uniflow workflow patterns runnable example (#1524)


- Print help panel on 'ma' / 'ma -h' with prog='ma' (#1530)


- Quote templated image/secret-name values in core Deployment charts (#1544)


- Add examples gallery page with all 9 working examples (#1469)


- **core:** Upgrade baseui 15→18, move to peerDependencies (#1522)


- Merge back release/v0.5 to main (#1547)


- Remove stray test artifact python/create-project.json (#1476)


- Ma cli: add -o, -A, and default DESC sort to <crd> get (#1576)


- **release:** Prepare v0.6.0


- Bump version to 0.6.0-rc.1 (#1595)



### Refactoring


- **trainer:** Move _dataset.py into a _private/ subpackage (#1433)


- **ui:** Centralize mutation middleware in the mutation hook (#1482)


- **core:** Split public API from primitives entrypoint (#1526)



## [0.3.1] - 2026-06-26


### Bug Fixes


- **ci:** Add helm dependency build before packaging (#1397)



### CI/CD


- Add nightly build trigger to integration tests (#1389)



## [0.3.0] - 2026-06-26


### Bug Fixes


- Fix proto import path (#11)


- Fix package name in v2/emtpy.go; run bazel mod tidy (#54)


- Fix the directory of generated python code (#105)


- Fix michelangelo api server configuration in sandbox (#111)


- Fix ray issue when downgraded to v1.0 (#125)


- Fix json unmarshal number issue (#152)


- Fix enum json unmarshal (#155)


- Fixing lint issues


- Fixing linter warnings


- Fix release action (#308)


- Fixed lint issues


- **controllermgr:** Remove invalid port field from config (#457)


- Fix dry run (#584)


- **inference-server:** Fix fx dependencies for inference-server controller (#637)


- **sandbox:** Fix inference url endpoint (#682)


- **inference:** Update httproute path matching (#684)


- **inference:** Rename configmap model storage path field to be protocol agnostic (#697)


- Fix sitecustomize log to avoid using basicConfig (#814)


- **ci:** Resolve race condition in combined coverage report (#851)


- **docs:** Use absolute paths for Welcome page links (#854)


- **website:** Migrate onBrokenMarkdownLinks to markdown.hooks (#857)


- **ingester:** Use scheme.ObjectKinds() to resolve GVK, fixing controllermgr CrashLoopBackOff (#922)


- **ci:** Use explicit run IDs to prevent stale coverage data (#862)


- Add pipefail to docs-check build step (#1077)


- Correct broken code examples and remove internal references from docs (#1085)


- Annotate broken markdown links in docs-check workflow (#1078)


- **ci:** Restrict build-examples push trigger to main branch (#1090)


- Normalize Uniflow capitalization (#1099)


- Use k8s Secret for MinIO credentials to survive sandbox sync (#1115)


- **mysql:** Fall back to scheme for empty GVK in getTableName (#1118)


- Always return activityID from worker activities for retry targeting (#1120)


- Wire MetadataStorage into apiserver so enableMetadataStorage works (#1153)


- Honor --exclude in sandbox k3d port forwards (#1205)


- Replace broken relative Go links with GitHub URLs and add workflow_dispatch to docs-check (#1226)


- Bracket-notation slash keys missing from error banner (#1202)


- **sandbox:** Disable unused Cadence kafka subchart (#1230)


- **sandbox:** Enable Cadence strongly-consistent queries in helm sandbox (#1234)


- Remove unnecessary PyFileSystem wrapping in RayDatasetIO (#1248)


- **helm:** Remove .helmignore that blocks subchart loading (#1249)


- **url-field:** Support relative paths as navigable links (#1277)


- Prevent TTL data loss when metadata storage is disabled (#1271)


- **pre-commit:** Fix prettier hook and document install step (#1275)


- Pipeline deletion prereqs — propagation threading, reconcile fixes, SparkJob termination (#1265)


- **spark:** Persist SparkJob status before immutable Update on termination (#1303)


- Register TypedStruct in ConnectRPC type registry (#1304)


- **sandbox:** End-to-end california housing xgb with UI, CLI and apiserver (#1349)


- **helm:** Auto-register Temporal default namespace on sandbox create (#1346)


- Atomic update to prevent Temporal schedule race condition (#1330)


- **notification:** OSS quality — extensible sinks, shared worker, configurable types (#1259 #1260) (#1283)


- **helm:** Add missing taskList to Temporal workflowClient config (#1366)


- Fixed //go/components/jobs/client/k8sengine:go_default_test build failure (#1392)



### CI/CD


- Skip unrelated workflows for docs-only changes (#844)


- Add dependency caching and bump setup-bun to v2 (#845)


- **website:** Enable experimental_faster for Docusaurus builds (#846)


- Add docs build check for PRs and fail on broken links (#855)


- Speed up Bazel test workflow (#858)


- Consolidate Bazel caches and speed up coverage workflow (#859)


- Remove dead code and fix silent failures in CI workflows (#860)


- Add proper path filters to avoid unnecessary workflow runs (#864)


- Remove combined coverage report workflow (#1098)


- Add helm lint + template CI gate for helm/ PRs (#1160)


- Replace QEMU cross-arch build with native runners (#1270)


- **release:** Add cliff.toml for changelog generation (#1365)


- **release:** Add PyPI publishing step to release workflow (#1367)


- **release:** Add Helm OCI chart publishing to release workflow (#1371)


- Add tag-based triggers to container and UI release workflows (#1382)


- Add release checklist issue template (#1381)


- Add nightly build workflow (#1379)


- Add changelog generation workflow (#1380)


- Enhance changelog and release checklist for breaking changes (#1390)



### Documentation


- **examples:** Add comprehensive docstrings to amazon_books_qwen example (#624)


- **examples:** Add comprehensive docstrings to llm_prediction example (#625)


- **examples:** Add comprehensive docstrings to nomic_ai example (#626)


- **examples:** Add comprehensive docstrings to bert_cola example (#627)


- **uniflow:** Add comprehensive docstrings to registration module (#629)


- **uniflow:** Add comprehensive docstrings to plugins module (#630)


- **examples:** Add comprehensive docstrings to boston_housing_xgb example (#628)


- Improve page titles for clarity and consistency (#832)


- **website:** Unify navbar styling and add GitHub icon (#834)


- **website:** Accessibility audit improvements (#833)


- **inference:** Add inference docs in wiki (#816)


- **ingester:** Fix reconcile decision tree to match actual handleDel… (#951)


- Add compliance guide for SOC 2, GDPR, and HIPAA (#1026)


- Add Helm prerequisite and Colima resource requirements to sandbox setup (#1064)


- Update README for public release (#1076)


- Add missing prerequisites, next steps, and troubleshooting to pipeline guides (#1086)


- Add authentication and identity provider setup guide (#1039)


- Address post-merge review feedback from PR #1026 (#1074)


- Add operator troubleshooting guide (#1038)


- Add PR process and testing strategy guides (#1037)


- Add CI pipeline guide and architecture overview for contributors (#1045)


- Add contributing overview (#1035)


- Restructure operator guide index into navigation hub (#1036)


- Add network and ingress configuration guide (#1044)


- Add monitoring and observability guide (#1040)


- Add experiment tracking integration guide for operators (#1041)


- Add model registry integration guide for operators (#1042)


- Add integrations landing page for operator guides (#1043)


- Add Go code style guide for contributors (#1046)


- **website:** Point Docusaurus at custom domain michelangelo-ai.org (#1101)


- Update doc links to michelangelo-ai.org custom domain (#1102)


- Improve getting-started docs (Ray comparison) (#1100)


- Add History and Evolution page to About section (#1116)


- **website:** Add Algolia DocSearch to docs site (#1122)


- Add backfill pipeline guide (#1137)


- Add MLflow integration guide (#1132)


- Add user-facing deploy guide (#1180)


- Add SQL concepts guide (#1170)


- Reorder getting started sidebar (#1169)


- Go key concepts and terms reference (#1185)


- Restructure operator-guides integrations section (#1184)


- Developer reference — protobuf, bazel, python utilities, shell scripts, YAML config (#1186)


- Restructure operator-guides into setup, components, and operations sections (#1187)


- Add ARCHITECTURE.md for @michelangelo-ai/core (#1191)


- Add Michelangelo Helm Chart operator guide (#1146)


- Improve sandbox setup and ports-and-endpoints guides (#1196)


- Add Why Michelangelo value proposition section to Welcome page (#1245)


- Restructure user-guides section (#1209)


- Add blog with open-source launch announcement (#1212)


- **contributing:** Add conventional commit message guide (#1335)


- Add roadmap page (#1211)


- **contributing:** Add breaking change checklist to PR template (#1370)


- **operator-guides:** Add Comet ML experiment tracking integration guide (#1361)


- **operator-guides:** Add notification delivery setup guide (#1343)


- **contributing:** Local image build and fix stale Helm-era references (#1347)


- **contributing:** Document SemVer tag format and versioning rules (#1377)


- **contributing:** Add deprecation policy (#1378)



### Features


- **MaCTL:** Pipeline Dev Run (#369)


- **uniflow:** Add transpiler callback (#453)


- **Uniflow:** Batch Run Support for Uniflow Tasks With Configurable Max Concurrency (#455)


- **inference-server:** Add inference server proto (#535)


- **controllermgr:** ConditionEngines Support Actor.Retrieve() (#509)


- **inference-server:** Inference server gateway and model configmap utilities (#583)


- **deployment:** Add oss plugin for deployment controller (#618)


- **inference-server:** Inference server controller with minimal triton plugin (#632)


- **inference:** Add sandbox support to demo inference (#640)


- Add optional TLS support for Temporal workflow client (#819)


- Add pr-template skill for consistent PR formatting (#1082)


- Add PipelineNameLabel to trigger-created pipeline runs (#1121)


- Add retry history tracking with AttemptDetails for pipeline steps (#1119)


- Add helm/michelangelo Helm chart for control plane deployment (#1143)


- Add Ingress support for UI, Envoy, and apiserver (#1154)


- Add Temporal optional subchart with fail-fast validation (#1159)


- **inferenceserver:** Make services in target clusters discoverable within control plane cluster (#1140)


- Add Ingress support for UI, Envoy, and apiserver (hosts[] structure) (#1157)


- Helm integration in sandbox, Cadence & Temporal subcharts, port sync (#1162)


- Support pipeline run ttl in config to delay the immutable set when reaching desired state (#1117)


- **inferenceserver:** Inference-server controller owns and provisions routing artifacts  (#1142)


- [CanvasFlex] Pusher Phase 1 PR1 — foundation layer (exceptions, types, storage backend, registry client) (#1214)


- [CanvasFlex] Pusher Phase 1 PR2 — config and plugin registry (#1222)


- [CanvasFlex] Pusher Phase 1 PR3 — DatasetVariable + DataSink + PandasIO (#1233)


- Switch browser RPC from gRPC-Web to Connect protocol (#1238)


- [CanvasFlex] Add EvaluationReport, Chart, Time protos + YARPC service (#1236)


- Port uniflow plugin improvements from internal — Ray IO, ProtoIO (#1232)


- **pusher:** Pluggable EvalReportSink interface + gRPC/LocalFile built-ins (#1237)


- **apiclient:** Instance-based constructor for per-process isolation (#1256)


- **core:** Introduce ResolvedActionItem and useResolvedActionItems (#1242)


- **apiclient:** Add EvaluationReportService to ServicesGen (#1263)


- **eval-report:** EvaluationReportService + APIClientEvalReportSink (#1254)


- **pusher:** ModelPusherPlugin + MinioStorageBackend + APIRegistryClient (#1261)


- **pusher:** Push() dispatch function + plugin registry (#1276)


- **revision:** Version-agnostic revision.Manager (#1145)


- SuccessOperations for mutation actions (#1201)


- **example:** California Housing XGBoost end-to-end UniFlow workflow (#1285)


- **pusher,sinks,docs:** S3Sink, Spark push_step, YARPC registry client, getting-started improvements (#1282)


- **cascadedelete:** Owner-ref stamping, drain, and retention for cascade delete (#1266)


- **pipeline:** Make revision management configurable (#1268)


- **cascadedelete:** Pipeline cascade-delete consumers, wiring, CLI, docs (#1316)


- **cli:** Add --notify-slack, --notify-email, --notify-on to pipeline run (#1351)


- **cli:** Add STARTED event type to --notify-on flag (#1369)


- **rayjob:** Add ownerReference and TTL for automatic RayJob cleanup (#1336)


- **ray:** Bubble up KubeRay RayCluster failures to Michelangelo control plane (#1384)


- **model_manager:** Add TorchTritonPackager for PyTorch model packaging (#1373)


- **rayjob:** Enable ShutdownAfterJobFinishes for RayJob cleanup (#1396)


- **python:** Add runtime warning for nightly builds (#1388)



### Miscellaneous


- Initial commit


- Create LICENSE


- Add bazel and .gitignore file (#1)


- Set up a CI/CD pipeline (#3)


- Add list, options and conditions.proto files (#2)


- .envrc for direnv and fix bazel setting for gazelle (#4)


- Add bazelisk to make bazel hermetic (#5)


- Update package names in proto files (#6)


- Move protobuf files from idl/michelangelo directory to proto directory (#7)


- Add gazelle:proto_strip_import_prefix to proto/BUILD.bazel (#9)


- Init go module (#10)


- Update api/BUILD.bazel (#8)


- Add the protobuf for Project CRD  (#12)


- Copy api, auth, and logging package, Fix go.mod (#13)


- Add Spark and Ray job CRD protos from v2beta1 (#15)


- Fix broken bazel build for go (#16)


- Adding k8s/api protos (#17)


- Add git action for ci/cd bazel test (#18)


- Remove buildkite folder (#22)


- Remove internal document link and license in go files (#14)


- Migrate sandbox from uber/michelangeo to michelangelo-ai/michelangelo (#19)


- Initial Python code structure using Poetry (#25)


- Make gazelle ingore python directory (#28)


- Change to use v1.2.2 kuberay which using v1 spec of Ray (#29)


- Remove temp file (#31)


- Kubeproto (#30)


- Create ray cluster and make ray job kubeRay compatible (#21)


- Build api protobuf files with kubeproto (#33)


- Add Cache for CI/CD github action (#32)


- Fix raycluster proto (#35)


- Add github action to check BUILD.bazel files are up to date (#34)


- Use tools/gazelle instead of installing gazelle in github action (#37)


- Protobuf validation compiler (#41)


- Ignore /pkg in .bazelignore (#36)


- Setup static analyzers for Go (#40)


- Adding go mod tidy check to CI (#39)


- Simplify proto compiler unit test (#42)


- Check Go formatting on CI (#43)


- Kubeyarpc compiler (#44)


- Add .idea and go/vendor to .gitignore (#45)


- Refactor and cleanup logging (#47)


- Initial Python-based Sandbox code (#46)


- Grpc / yarpc services for CRD types (#48)


- Add `.gitignore` file with more general coverage (#50)


- Add controller manager (#49)


- Add kubeyaml compiler (#53)


- Add logging, config and env to controller manager (#52)


- Gomock (#56)


- Build python package for pypi.org (Exclude all source code in poetry package builder in temporary) (#51)


- Set Pypi Package name as `michelangelo-ai` (#62)


- Add ray-cluster controllers (#55)


- API handler (#63)


- Simplify and improve error handling in api handler (#66)


- Add ray job controller (#64)


- Add ray job controller test (#68)


- Add Python-based tools/sandbox (#69)


- Add unit test for ray cluster (#70)


- Worker Service. Initial code. (#72)


- Dev Release of Michelangelo services (#74)


- Enable container registry auth for sandbox (#76)


- Michelangelo api server (#75)


- Use kubenates v1.podTemplateSpec proto (#73)


- Import Cadence starlark-worker (#78)


- Initial code (#79)


- Api server (part 2) (#80)


- Change `ruff` instead of `black` as python linter (with poetry package fix) (#65)


- Only do static link on Linux (#81)


- Apiserver (part 3) (#82)


- Update Python test workflow & Add Dockerfile for example  (#84)


- Add pull_request_template.md for the default template to help developers (#85)


- Python gRPC client (#83)


- Add unmarshalJson method to all proto generated go code (#87)


- Add ray activities and plugins (#89)


- Add starlark code for ray (#90)


- Add utils functions and unit tests for uniflow core (#95)


- Allow to add 'lib' directory except python packge build (#96)


- Add end to end flow for bert-cola examples and fix FX issues (#94)


- Add support for web gRPC client (#93)


- Python client (#97)


- Add minIO to sandbox storage (#92)


- Separate out python packages in poetry config (#100)


- Fix minio in sandbox and remove model pushing in examples (#101)


- Update README.md (#104)


- [integration-test] bert_cola local run in github workflow action (#103)


- Update pytorch version (2.0.1 --> 2.1.2) to run integration test (#106)


- Fix worker image for gcr.io (#110)


- Change api server port and print configuration (#107)


- Add apiserver and controllermgr in sandbox (#109)


- To fix sandbox env for running example (#112)


- Allow multiple api hooks (#113)


- Use Dockerfile under example folders (#115)


- Downgrade k8s library version (#117)


- Add nomic-ai model for fine-tune example (#98)


- Add proto file descriptions (#119)


- Add json non-inline support for JSON unmarshal (#114)


- Add auto linter run in pre-commit-config (#118)


- Remove references to "v2beta1" (#122)


- Fix per-PR lint check when the file is missing & add pre-commit script (#124)


- Updating the integration test BERT_COLA local mode to run actual pipeline run


- Generate vite application for Michelangelo UI (#116)


- Delete deprecated sandbox (#128)


- Update README.md (#130)


- Introduce Pipeline CRD and related CRDs (params, trigger_run, notification) (#121)


- Refactor duplicated docketfile in examples (#131)


- Pin yarn/node versions for UI directory


- Update lint and typescript approach for eslint 9 and vite


- Onboard CICD linting to UI


- Add examples for LLM prediction using HuggingFace and vLLM (#132)


- Remove generated JS proto files from repository


- Fix inline issue in protobuf UnmarshalJson method for array type (#126)


- Fix JavaScript generated gRPC code location (#135)


- Add Pipeline Run entity definition (#137)


- Connect UI to API server


- Protobuf -> SQL compiler (#136)


- Add model proto definition  (#138)


- Minor refactoring of protobuf comipilers (#141)


- Onboard BaseWeb to Michelangelo Studio


- Fix TypeScript error for missing CSS module


- Lint


- Add message descriptions for job.proto and pod.proto (#139)


- Add QueryByTemplateID method in MetadataStorage interface (#144)


- Include index information in the go code generated by kubeproto compiler (#146)


- Fix proto issue (#148)


- Simplify api hook registration interface (#150)


- Add a function to return the registered API hooks (#151)


- Migrate view container component


- Add OnListSuccess() api hook (#154)


- Add EnableCRDDeletion config and check not found error (#161)


- Onboard Michelangelo Studio to react-router


- Remove src/ directories


- Onboard testing to UI


- Use agnostic starlar-worker to support Temporal (#147)


- Implement apihandler.handleUpdate() (#166)


- Add TTE model SDK library for text-based similarity learning and retrieval.  (#149)


- Replace vite alias with package.json imports


- [go] Create a components directory and move all existing components u… (#157)


- Fix worker config (#168)


- Fix Get-Started example accelerate version and MA service host (#167)


- Fix import ordering


- Create build of @michelangelo/core


- Fix cadence worker address (#171)


- Add a function to clear registered API hooks (#173)


- Onboard RPC handlers to MA Studio


- CICD fixes


- CICD fixes


- CICD fixes


- Allow transport/address to replace proxy region (#175)


- Add temporal to sandbox setup (#177)


- Add query transformation to hooks (#176)


- Onboard project-aware querying to MA Studio (#178)


- Add spark plugin and controller (#165)


- Add spark-operator installation to sandbox (#181)


- Python protobuf codegen updates (#182)


- Refactor RPC handlers to be a constant


- Rename QueryProvider to ServiceProvider


- Refactor RPC injection solution to use functions


- Add cachedOutput to support resume from (#163)


- Add support for external router provider (#183)


- Ignore .python-version (#185)


- Consolidate k8s config and storage config logic (#184)


- Baoquan/pipeline controller (#172)


- Fix example run with poetry (#187)


- Fix spark installation (#186)


- Add model family CRD (#190)


- Add output schema in model crd (#195)


- Baoquan/blobstore (#194)


- Update MA Studio for merge back (#196)


- Support task overrides


- MA sandbox create returns error while list helm repo when no repos exist (#199)


- Upgrade minio release to latest. (#203)


- Migrate cell rendering system from internal repository (#200)


- Prepare cell renderers for internal migration (#201)


- Add date cell renderer with timezone support (#204)


- Add description cell renderer with text truncation (#206)


- Implement icon system and update cell renderers (#208)


- Add Link component and cell renderer (#209)


- Add MultiCell renderer and improve cell type system (#210)


- Add Tag component and style system types (#211)


- Add state cell renderer with configurable text and color maps (#212)


- Implement unified cell rendering system (#213)


- Add cell tooltip and markdown rendering system (#217)


- Refactor task imports in uniflow run_task.py (#218)


- Implement Row component for structured data display


- Add Box component for content grouping and layout


- JsonData for canvas (#219)


- Update michelangelo-core for merging cell renderers internally (#220)


- Align cell renderers with internal implementation (#224)


- Implement lightweight pipeline and project list demo (#225)


- Add shape to data schema (#226)


- Add sandbox CRDs and update sandbox creation process (#215)


- Handle external package enums in protobuf code generation (#221)


- Mactl prototype (#227)


- Add log-viewer deployment job (#205)


- Export theme provider for theme injection (#231)


- Upgrade to 0.1.7 (#232)


- Add Canvas key schemas (#233)


- PipelineRun Controller


- Add retry field to python decorator, and implement retry in starlark (#197)


- Fix OSS api server port (#230)


- Add gRPC reflection to API Server (#235)


- [MACTL] Add delete command for mactl (#236)


- Upgrade michelangelo-core to 0.1.8 (#237)


- [MaCTL] Use repo root on yaml file (instead of mactl root) (#238)


- Add logs bucket creation k8s job (#234)


- Draft uniflow dev guideline (#241)


- Enabling PipelineRun and other services for apiserver


- Change default retry attempt to 0 (#243)


- Update core version in package.json (#239)


- Set DEFAULT_RETRY_ATTEMPTS = 0 (#247)


- Convert cell renderer utilities to hooks for context integration (#245)


- Add CellProvider for extensible custom cell renderers (#246)


- Add basic log collection, compression and upload flow for client-server execution model (#240)


- Add safeStringify to display meaningful object content (#250)


- Enable RPC framework compatibility through error normalization (#251)


- Add Connect RPC error normalizer and integrate with useStudioQuery (#252)


- Add pipeline runs list functionality (#253)


- Convert ApplicationError from interface to class


- Migrate useStudioQuery to React Query's error handling


- Add ErrorView component for consistent error state presentation


- Add useScrollRatio hook for table scroll position tracking


- Add illustration components with theming support


- Add timestamp formatting utilities with timezone support


- Add DeepPartial utility type for recursive optional properties


- Add useHover hook for mouse interaction state management


- Add CLAUDE configuration to javascript directory


- [Multiversion] Support syncing multiple CRD versions (#255)


- Add foundational string interpolation system


- Add function interpolation support


- Add React interpolation resolver hook with recursive processing


- Add context providers and repeated layout integration


- Add property exclusion to useInterpolationResolver


- Add interpolation utilities for detection and cleanup


- Integrate interpolation resolution into useStudioQuery


- Update getObjectValue function definition


- Add TanStack React Table dependency


- Establish table foundation with column transformation


- Implement table header with transformer architecture


- Implement table body with transformer architecture


- Integrate components into unified table


- Add table loading state component


- Onboard Cell engine to Table


- Replace Row-based table with Table component


- Implement table empty state


- Add table error state component


- Upgrade starlark to v1.0.9 (#275)


- Add table search functionality


- Move project api hooks to /go/components/project (#276)


- Go language server settings and vendor directory for vscode & cursor (#280)


- Add protobuf struct utilities


- Add TextEditor component for JSON display


- Add sandbox route for component development


- Add controlled table state management


- Add localStorage utilities with error handling


- Implement table state persistence across browser sessions


- Add state persistence to pipeline(run) lists


- Add column resolution utilties


- Add table filter factory hook


- Integrate column filters into table


- Add execution view task list utilities


- Implement sandbox execution display


- Add datetime filter support to table columns


- Add buildMatrix utility for inline task rendering


- Add styled components for execution display


- Add basic filter menu to table action bar


- Implement categorical filter menu integrated into table


- Refactor execution test utilities using factory pattern


- Refactor Execution view component names


- Add task details accordion with metadata display


- Fix YAML config key format for storage version


- Implement subprocess-based pipeline registration using transpiler code for MaCTL


- Clean up variable names and comments


- Hardcoding spec manifest type for uniflow pipelines


- Changing from relative to absolute imports


- Ruff check


- Remove redundant fsspec import checks


- Complete datetime filter migration


- Add PhaseCard component for project workflow navigation


- Register ext validation hook


- Validation extension hook tests


- Use kubeprot/templates


- Run goimport formatting tool


- Add active filter display


- Add schema-based body renderer for execution task details


- Add state management system to PhaseCard component


- Complete schema-based body renderer implementation


- Standardize execution view styling


- Implement phase grid navigation component


- Extract entity configurations into structured config system


- Implement configuration-driven PhaseEntityView system


- Integrate PhaseEntityView system


- VSCode / Cursor Golang IDE setup using gopackagesdriver (#306)


- Python wheel file build and release (#307)


- Add ext fields to all Spec and Status (#291)


- Extract Execution component for consistent vertical spacing


- Display subtasks recursively in task detail accordions


- Add click-to-scroll navigation to execution view


- Add table pagination component


- Integrate pagination with table component


- Refactor worker plugins (#313)


- Add Cluster CRD, Cluster Service and Kubernetes related proto messages (#316)


- Add column sorting functionality to table component


- Populate manifest content for uniflow pipeline registration


- Python ruff check


- All imports on top of file


- Returning content as JSON


- Centralize column transformation


- Modify sandbox cli to create jobs cluster (#258)


- Add column configuration system for table component


- Create RBAC and Ray Manager service account for Ray jobs (#318)


- Add pipeline run functionality to mactl


- Add YAML config system for mactl with MinIO credentials


- Edit


- Move run command implementation to pipeline plugin


- Refactor run command to use consistent plugin architecture


- Add row selection functionality to table component


- Implement cell tooltip filtering


- Add Docker and Kubernetes integration for Michelangelo UI (#322)


- Implement sticky table columns for horizontal scroll UX


- Implement expandable table rows with sub-row rendering


- Add metrics to unmarshal error (#328)


- Fix the conflict port of UI and Grafana (#335)


- Move FluentBit to the experimental services in the Sandbox (#336)


- Improve error logs reporting in Uniflow's remote run command (#338)


- Fix wrong branch of controller manager (#340)


- Add table overrides suport (#331)


- Add responsive column widths to table (#341)


- Add initialState parameter to table state persistence (#342)


- Improve icon sizing compatability (#343)


- Shrink table header spacing (#344)


- Fix table filter option generation (#345)


- Disable page index reset (#346)


- Fix issues during Execution merge back (#347)


- Implement basic detail view component (#351)


- Add override infrastructure to execution components (#348)


- Implement configuration-driven entity detail view (#352)


- Fix the wrong variable usage for sandbox cluster name in jobs cluster secret creation (#333)


- Sandbox minor code refactoring, cleanup, and improvements (#354)


- Provide Row action access to row record (#353)


- Implement tab-based detail view pages with URL navigation (#356)


- Add project-namespaced filter persistence to table state (#360)


- Add github action for python lint (#361)


- [MaCTL] update pipeline create-plugin to contain uniflow image annotation (#358)


- Fix sandbox setup (#355)


- Extend detail view pages with execution support (#359)


- Prevent users from being stuck on invalid pages after filtering (#362)


- Fix table search icon size inconsistencies (#365)


- Match action column and configuration header styling (#366)


- Long TTL for ray-manager cluster auth token (#337)


- Kuberay Rest Client and Type definitions for Jobs Infrastructure (#260)


- [MaCTL] Fix MaCTL error - missing annotation field (#357)


- Allow none return value in uniflow task (#363)


- Add dynamic row selection state management (#367)


- Enable dynamic column resolution and full column rendering in filters (#370)


- Introduce InputTableState to eliminate pageIndex requirement


- UI Release (#373)


- Add table grouping support (#374)


- Export more properties to TableRow API (#375)


- Fix table column width collapsing (#377)


- Fix multi-cell column filter display (#379)


- Add handling for tables with empty columns list (#380)


- Hide pagination for small tables (#382)


- Fix global filtering for complex data types in table (#383)


- Fix table selection context API inconsistency (#385)


- Export Table for Uber internal consumption (#384)


- Refactor api handler (#350)


- Separate column tooltip from core table cell (#387)


- Enable row data access in table cell tooltips (#388)


- Add support for applying styles to columns (#391)


- Include all cells in TableRow type (#389)


- Support custom table header component (#393)


- Remove unfilterable colums from filter menu (#394)


- Support custom sorting functions for columns (#392)


- Fix aggressive table cell truncation (#395)


- Implemented DevRun PipelineRun Controller Changes (#381)


- Fix apihander factory Fx for controllerMgr (#390)


- Fixed Temporal Client args (#396)


- Add support for cell help tooltip (#397)


- Fix ColumnConfig tooltip type (#398)


- Remove TableData from accessor generic (#399)


- Make sticky sides the table default (#401)


- Upgrade michelangelo UI for table fixes (#402)


- Proto change for triggerrun (#403)


- Onboard trigger list to UI (#404)


- Add generics to view configurations (#406)


- Introduce reusable TableConfig pattern (#407)


- PipelineRun "Continued Reconcile Re-queue after Terminal State" Bug Fix (#408)


- Implement test utility for mocking queries (#409)


- Add table page support to detail view components (#410)


- Onboard javascript/ to vitest projects (#415)


- Display task metadata in for subtasks (#417)


- Replace build-time environment config with runtime injection (#412)


- Implemented per-task status (#400)


- Add deployment proto and module (#414)


- PipelineRun Step State Update for Temporal Workflow Cancellation (#421)


- Add trigger detail view with dynamic pipeline run filtering (#420)


- Fix javascript/tsconfig.json (#425)


- Add initial form component system with textbox (#424)


- Add RadioField with dual string/boolean value support (#426)


- Add SelectField component (#428)


- Add BooleanField for checkboxes and toggle switches (#430)


- Add CollapsibleBox component (#431)


- Added guides for error logging and code refactory (#386)


- Added Pipeline Run Kill (#419)


- Upgrade k8s clients (#433)


- Add form group and row layout components (#432)


- Add Dialog component with scroll-aware button dock (#434)


- Add render prop pattern to Form for wrapper composition (#436)


- Add Imagespec to uniflow task decorator (#422)


- Cron Trigger Support with Dynamic Parameters (#371)


- Change mactl syntax to resource-action order (#418)


- [Trigger controller] fix flaky tests (#437)


- [MaCTL] Add unittest for gRPC reflection (#438)


- Add FormDialog component (#441)


- Add CRD naming utilities for resource generation (#443)


- Add shell script for running Starlark code on Cadence or Temporal (#423)


- Model Search Starlark Plugin Implementation for Remote Execution (#405)


- Implement create Pipeline Run action


- PipelineRun ResumeFrom (#413)


- Add Observed generation to conditions (#444)


- Add webhook server (#285)


- Add PageHeader component (#447)


- CRD version conversion compiler (#446)


- Release UI version 0.1.6 (#449)


- Add generic type support to form system (#450)


- Fix the imageSpec field typo naming and remove it from overrides (#456)


- Added Ray Cluster Creation Timeout (#454)


- Add backfill trigger with trigger run controller change, cadence workflows and activities (#452)


- [MaCTL] Add unittest for GRPC reflection utils (#458)


- Added timeout_seconds parameter to create_cluster (#461)


- Fix tls issue for apiserver (#462)


- Remove `clusterName` field in pipeline apply/create for OSS api server (#463)


- **starlark-worker:** Bump starlark worker version (#469)


- Added Dynamic PipelineRun Task Queue and Storage Url (#439)


- [MaCTL] refactoring - separate out GET/DELETE implementation out (#465)


- [MaCTL] Add unittest for gRPC recursive search (#467)


- Implement Granular control for Incompatible schema with per-CRD allowlist (#468)


- [Proto] Change ProjectSpec.ext CRD schema as optional (from required) (#471)


- [MaCTL] Fix linter error in mactl & Revise TLS feature related unit test (#466)


- [MACTL] Separate the implementation of apply/create from CRD class (#474)


- [MaCTL] Create a new CRD with APPLY mactl command (#478)


- NEW FEATURE: Added `mactl triger_run kill` command (#460)


- Make ruff lint & format workflow fails when it catches some issue (no blocking check) (#479)


- [MaCTL] Disable TLS config in default. (#482)


- [mactl] remove non-default generator in base CRD (#481)


- [mactl] Merge GET/LIST method to GET (#480)


- [MaCTL] Apply function for using annotation and user labels (non-metadata fields) (#477)


- Export CompareCRDSchemas method to Import back (#485)


- Fix typos in source codes including proto files (#486)


- [mactl] Merge mactl command inside of the ma command (#484)


- [MACTL] Fix missed top-level `ApiVersion` and `kind` fields in Michelangelo yaml format (#493)


- Revert UnmarshalJSON from main CRD type (#487)


- Feature/amazon books qwen recommendation (#489)


- Add Claude Code Effective Go skill configuration (#495)


- File sync on docker container for both ray and spark task (#475)


- Fix ignored error returns in logging and pipeline processing (#494)


- Fix error wrapping in pipeline workflow execution (#510)


- Fix error wrapping in kubeproto code generation (#511)


- Fix error wrapping in trigger parameter parsing (#512)


- Add .mactlrc configuration file support for mactl (#483)


- Remove debug fmt.Println statements from executeworkflow.go (#522)


- Fix boolean comparison anti-patterns in kubeproto generators (#519)


- Fix boolean comparison anti-patterns in API (#518)


- Update URL cell renderer to support localhost (#523)


- Fix boolean comparison anti-patterns in kubeproto YAML and worker plugin (#520)


- Fix error wrapping in pipeline controller status update (#513)


- Fix underscore prefix naming in worker plugins (#515)


- Fix underscore prefix naming in components (#517)


- Fix processJobTermination return signature to (bool, error) (#524)


- [MaCTL] Apply dynamic argparser by CRD and action functions (#508)


- Fix fluent-bit to use s3 and plain text file instead of CLP (#521)


- [MaCTL] Fix plugin pipeline run command missing signature & separate out CRD modules (#537)


- Fix spelling and grammar errors in Go code (#531)


- Add documentation to worker plugins and utilities (Group 2/3) (#529)


- Scaffold model manager packager interface (#538)


- Add documentation to PipelineRun and Spark controllers (Group 1/3) (#528)


- Add documentation to base infrastructure types (Group 3/3) (#530)


- Add boolean data type to model schema (#541)


- Revert refactor temporarily to unblock code yellow task (#545)


- Update LICENSE (#581)


- Add unit tests for pipeline plugins and fix imports to use absolute p… (#536)


- [MaCTL] Add entity action function signature to contain help messages for user (#539)


- [MACTL] Fix unittest failure by separated PRs (#582)


- Implement TODO tracking system with automated enforcement (#567)


- Jobs client infrastructure and utilities (#261)


- [Multi-version] Created v2alpha1 spoke version & Add Unit test for generated conversion code (#540)


- Fix the usage of TODO in Jobs code blocking the CI (#585)


- [MaCTL] fix pipeline dev-run env argument type (#586)


- Remove Appending Source Pipeline to Pipeline Run Steps (#587)


- Add Google Python Style Guide Claude skill (#569)


- Configure baseline code coverage with 90% requirement for new code (#543)


- Fix typo in list API (#593)


- Remove commented-out code in component controllers (#532)


- Migrate schema and file utils for packaging in model manager (#592)


- Add model list page (#594)


- Migrate module_finder in Model Manager to OSS (#598)


- [Formatter] Fix ruff format error under `python/tests/` path (#589)


- Remove commented-out code in worker activities and tests (#533)


- [Linter] Remove unused imports or variables 1 (#590)


- [Formatter] Fix ruff format error under `python/michelangelo/canvas/lib/` path (#588)


- [Linter] Remove unused imports or variables in boston housing example (#601)


- [Linter] Remove unused imports or variables in job_specs & nomic_ai (#602)


- Compute Cluster Controller for Michelangelo Jobs  (#451)


- Fix go-coverage workflow: remove broken setup-bazelisk action (#599)


- Compute Cluster batch job scheduler with a simple “cluster-only” assignment strategy (#464)


- Jobs Cluster controller and scheduler integration (#472)


- Integrate Scheduler & Federated client with Ray Cluster Controller (#490)


- [Multi-version] Register Conversion Webhook in apiserver (#595)


- [Python Quality] Part 1/4: Expand Ruff configuration (Issue #570) (#607)


- Add fine-tune gpt2 (#544)


- Ray Job Controller changes for Multi Cluster support for Ray Jobs (#491)


- MA Sandbox changes for running ray jobs in compute cluster (#473)f


- Connect to control plane minio storage in compute cluster instead of creating a separate storage in compute cluster (#604)


- Add file-sync support for dev_run command (#603)


- Expand README with open-source initiative details (#596)


- Fix gpt2 oss model eval (#631)


- [Multi-version] Fix ignored field still looking for hub message bug in kubeconversion compiler (#638)


- Fix spark tasks for pipeline run (#488)


- Add README files for example workflows (#647)


- Create CODE_OF_CONDUCT.md (#633)


- Ignore test fixtures in linter (#649)


- Model Manager: Add reflection utilities (#642)


- Model Manager: Add module utilities and enhance module finder (#643)


- Model Manager: Add pickle utilities for model serialization (#644)


- Model Manager: Add data utilities for model input/output handling (#645)


- Model Manager: Update schema, packager, and interface (#646)


- Add comprehensive docstrings to Uniflow core library (Phase 2) (#653)


- Add Revision Proto (#655)


- Update Python code coverage baseline to 66% (#648)


- Document I/O utilities and workflow context (Phase 3) (#654)


- Add missing training sdk (#641)


- Enforce all docstring rules (D100-D104) for new code (#656)


- Add comprehensive documentation to pipeline controller (Go) (#658)


- Update README.md (#662)


- Model Manager: Enable model class serialization during packaging  (#660)


- Delete not needed test file (#663)


- Add comprehensive docstrings to Michelangelo API v2 client (#657)


- Model Manager: Add module level docstrings for every module (#667)


- Add comprehensive docstrings to triggerrun and utils (Go) (#659)


- Add comprehensive docstrings to Spark component (Go) (#665)


- Model Manager: Add pickle dependency serialization support for custom Triton packager (#669)


- Model Manager: Add model data serialization/deserialization utilities (#674)


- Model Manager: Update documentation and comments (#675)


- Update Python code coverage baseline to 70% (#676)


- Remove maf ray and move it to pytorch (#673)


- Allow uf_storage_url to be replaced from env variables (#681)


- Restructure python folder (#683)


- Add comprehensive docstrings to project components (Go) (#685)


- Fix examples after folder structure (#686)


- Add Michelangelo JavaScript/TypeScript Claude skill (#671)


- Model Manager: Enable Creating Raw Model Package with Custom Triton Packager  (#688)


- Add Azure client for blobstorage (#687)


- **deployment+inference:** Update docstrings (#690)


- Model Manager: Add raw model package loader for custom Python models (#692)


- Model Manager: Add validation for custom Triton raw model packages (#698)


- Model Manager: Add deployable model package creation and validation for custom Triton models (#701)


- [Uniflow] Enable stack trace logging for argparse type validation errors (#700)


- Ignore .tmpl files in coverage report (#703)


- [MaCTL] Change plugin entity directory structure (#693)


- Model Manager: Bump model manager line test coverage to 100% (#704)


- Mactl api version autodetect (#691)


- Merge process_terminated_job function in starlark (#708)


- Model Manager: Allow omitting config.pbtxt name to avoid folder-name mismatches for deployable model packages (#709)


- Model Manager: add runnable packaging examples for CustomTritonPackager (#711)


- Add cron trigger with dynamic parameters (#710)


- Update python-build and python-coverage check: Free disk space before installing dependencies (#717)


- Model Manager: Remove deprecated custom_triton_packager example (#713)


- Fix JavaScript coverage N/A issue in combined coverage report (#715)


- [mactl] Add abstracted view in list function. (#723)


- MaCTL PipelineRun Kill Flag and UF_STORAGE_URL fix (#702)


- [mactl] Return result instance in CRD function call (#726)


- [MaCTL] Add unittest for more coverage (w/ fixing lint warnigs) (#727)


- [linter] Update python linter report more compact (#724)


- Fix Uniflow workflow input on pipeline run controller (#712)


- Add issue templates (#719)


- Create CONTRIBUTING.md (#733)


- Update CONTRIBUTING.md (#736)


- Update CONTRIBUTING.md (#737)


- Create ADOPTERS.md (#740)


- Create MAINTAINERS.md (#738)


- Add notification to pipeline run and enable trigger run notification (#735)


- Remove the Any ext fields in protobuf messages (#745)


- Allow customize version conversion logic (#748)


- Add docs (migrate from github wiki) (#741)


- Create docusaurus docs site (#746)


- Add github workflow to deploy docs site (#747)


- Update baseUrl for docs site (#750)


- Update doc guide (#752)


- Add missing code blocks, add code block types (#753)


- Added Pipeline and PipelineRun Controller Metrics (#754)


- Fix Temporal sandbox setup with manual database creation (#755)


- Claude skill for updating docs (#751)


- Fix formatting on model registry, sandbox pages (#758)


- Implement cron schecule in temporal using schedule (#749)


- Add generated proto-go module and tools for non‑Bazel users (#757)


- Add BUILD.bazel files in proto-go directory (#776)


- Add comprehensive JSDoc to core public hooks (Phase 1 of #652) (#773)


- Add comprehensive JSDoc to core public components (Phase 2 of #652) (#774)


- Replace external GitHub image URLs with local assets in docs (#782)


- Auto-detect user name from environment for OSS compatibility (#707)


- Replace external GitHub image URLs with local assets in api-framework.md (#783)


- Fix JSDoc accuracy issues in hook and component documentation (#781)


- Add .npmrc to website to use public npm registry (#784)


- In hasInstances only list storage version (#785)


- [linter] Fix mactl linter warnings (#779)


- Redesign docs site CSS with Uber brand styling (#786)


- Add clickable category index pages to docs sidebar (#788)


- Add favicon derived from logo with multiple sizes (#789)


- Add landing page with animated hero, features, and code examples (#791)


- Remove duplicate docs and fix image references (#792)


- Add retry support for Uniflow in same pipeline run (#780)


- [MaCTL] Fix mismatched function signature (#796)


- Improve overview.md for ML practitioners (#777)


- Make sandbox setup configable for temporal (#804)


- Add UI architecture documentation to Developer Reference (#763) (#794)


- Add check for pipeline git ref (#803)


- Add retry support to pipeline run detail page (#801)


- Fix Sandbox temporal setup (#807)


- Fix duplicate BaseProvider causing z-index issues with Layers (#805)


- Fix retry mutation binary serialization failure (#810)


- Refactor config loading into config.py (#795)


- Fix minor issues in overview.md documentation (#809)


- Remove decorative emojis from documentation (#831)


- Add landing page navbar styling and theme toggle animation (#820)


- Add CLAUDE.md to website directory (#821)


- Migrate config from ~/.mactlrc to ~/.ma/config.toml (#813)


- Add conversion option to RayCluster and RayJob (#818)


- Create NewReconciler constructors for RayJob and RayCluster (#837)


- Add workflow URL in config for debugging workflow in workflow engine cadence/temporal (#731)


- Move ray and spark plugins/activities into CMD instead of core modules (#838)


- CR version conversion functions (#798)


- Update pipeline-management.md (#843)


- Add missing table test coverage (#849)


- Refactor TLS config for both cadence/temporal to support full implementation (#839)


- Add skipIncompatibleCheck config flag for dev environments (#848)


- Fix ray logger issue in fx.in (#852)


- Move mactl dependencies to required for ma CLI command (#863)


- Update overview.md (#840)


- Update core-concepts-and-key-terms.md (#841)


- Update index.md (#842)


- [mactl] Fix ValueError when UpdateTimestamp label is empty (#861)


- Introduce text area field (#868)


- Add field-level validation to form system (#871)


- Fix sandbox setup for temporal for controllermgr (#850)


- Add base banner component and banner layout (#870)


- Add form step layout (#872)


- [mactl] add pagination limit support  (#866)


- Add field focus support to the form system (#873)


- Make storage-url passed from dev-run command line (#869)


- Rename variables to remove cadence words (#875)


- Fix retry error by stripping protobuf internals at request layer (#879)


- Update model registry guide with comprehensive examples and API reference (#878)


- Improve documentation quality for open source release (#881)


- Add ability to initialize form field's through defaultValue and initialValue (#876)


- Upgrade k8s to 1.31 from 1.28 (#877)


- Support tile radio field (#884)


- Add FormErrorBanner with field registry (#886)


- Add StickyFooter component with Form footer prop (#891)


- Make filter menu scrollable (#880)


- Add hook for programmatic form mutations (#893)


- Bump version to 0.1.14 (#895)


- Extend select field component (#890)


- Fix critical CLI documentation errors verified against codebase (#887)


- Create license.md (#759)


- Replace broad JavaScript style-guide skill with focused Claude skills (#892)


- Add miscellaneous form fields & layouts (#897)


- Add support for repeatable form fields (#894)


- Ingester & Mysql for CRD backup (#778)


- Support markdown in form-control captions (#901)


- Remove .mcp.json and add to gitignore (#904)


- Export AddButton component (#902)


- Improve project management documentation for open source release (#888)


- Fix sandbox setup for ingester (#908)


- Support parse and format in all form fields (#905)


- Add DateField component (#906)


- Run_pipeline uniflow plugin for oss (#815)


- Add input/output to pipelineRunStepInfo to show in the UI (#907)


- Fix TaskBody passing parent context to TaskListRenderer (#913)


- Fix flaky select-field test (#900)


- Add ConfirmDialog component (#914)


- [MaCTL] Support multiple plugins (#835)


- Change plugin configuration and Add plugin usage documentation. (#921)


- Change default API server port from 14566 to 15566 to avoid IDE exten… (#919)


- Fix bazel run for control plane (#912)


- Add SubTaskListRenderer override to ExecutionOverrides (#918)


- Bump michelangelo-ui app version to 0.1.7 (#925)


- Add BreadcrumbBar with URL-driven breadcrumb trail (#916)


- Add slide-out menu drawer to BreadcrumbBar (#917)


- Add GitHub issue links to all TODO comments (#942)


- Bump michelangelo-ui app version to 0.1.8 (#947)


- Expand building-from-source docs with Go and Python setup (#945)


- Add Uniflow plugin development guide (#952)


- Fix pipeline create tar upload failure (#953)


- **mlflow:** Get mlflow working on sandbox (#957)


-  Apply Go code quality patterns: unexported fields, functional options, guard clauses, named constants  (#958)


- Remove grid layout from DateField (#915)


- Upgrade packages/core to 0.1.15 (#956)


- Update README with OS Badges (#661)


- Update ADOPTERS.md (#964)


- Create security.md file (#889)


- Create CHANGELOG.md (#739)


- Update README.md with beta disclaimer (#967)


- Update MAINTAINERS.md (#968)


- Update CHANGELOG.md (#966)


- Update file-sync-testing-flow-runbook.md (#963)


- Update CONTRIBUTING.md (#970)


- Update README.md (#969)


- Fix baseUrl for GitHub Pages project site (#972)


- Restructure docs into 4 coherent sections (#955)


- Start displaying artwork in FormBanner (#973)


- Removing generic examples (#975)


- Export form components and banner (#974)


- Change PyPi pakcage name from `michelangelo-ai` to `michelangelo` (#965)


- Update cli.md (#962)


- Update index.md (#979)


- Update README.md (#981)


- Standardize project/namespace terminology in user guides (#984)


- Update exclamation mark icon (#954)


- Add git validation support in mactl (#910)


- Add CNAME for custom domain (michelangelo-ai.org) (#971)


- Update core-concepts-and-key-terms.md (#976)


- Update building-michelangelo-ai-from-source.md (#989)


- Fixing wrong link for sandbox setup (#990)


- Update getting-started.md (#926)


- Improve triggers documentation to open source quality standards (#982)


- Reduce interaction delay in table pagination tests (#994)


- Add inline broken link annotations to Docs Check (#995)


- Enforce no module-level test setup via custom ESLint rule (#978)


- Update index.md (#988)


- Delete docs/contributing/developer-guide.md (#997)


- Rename package to @michelangelo-ai/core and add npm publish CI (#992)


- Update overview.md (#996)


- Boston-housing-xgb pipeline run fix (#1001)


- Add top-level nav links to StudioBar menu drawer (#1002)


- Fix relative link resolution on GitHub Pages (#999)


- Add local ESLint rule to ban barrel exports (#1003)


- Fix import layering violations in packages/core (#1006)


- Add no-nested-ternary, eqeqeq, and no-default-export ESLint rules (#1008)


- Enforce TypeScript import and naming conventions via ESLint (#1004)


- Introduce label addon (#983)


- Add OCI DC type (#1010)


- Fix docs-check workflow YAML syntax (#1007)


- Remove v2.AllCRDObjects list (#1012)


- Mactl add proxy support (#1005)


- Update overview.md (#959)


- Update ingester-sandbox-validation.md (#961)


- Update ingester-design.md (#960)


- Support protobuf oneof in version conversion (#1014)


- Reorganize Claude-related entries in .gitignore (#1018)


- Remove CR_PAT authentication requirement (#1019)


- Mactl add user config (#1015)


- Add per-row actions system to table views (#1016)


- [OSS] Michelangelo open source version upgrade (#1021)


- Add robots.txt for docs site SEO (#811)


- Implement kill/pause/resume for trigger (#1020)


- Improve getting-started docs for open-source quality (#1022)


- Add IAM for minio s3 config (#1025)


- Extend no-module-scope-test-setup to catch wrapper helper functions (#1023)


- Bump core library version (#1009)


- Upgrade Node.js from 22.11.0 to 24.14.1 (#1030)


- Add workflow_dispatch to npm-publish workflow (#1031)


- [CI/CD] Leave comment for python lint error message only if it fails (#993)


- [MaCTL] Change plugin import timing earlier and preparation for module level override (#1028)


- Add trigger configuration support for mactl pipeline create (#1027)


- Add repository field to core package.json for npm provenance (#1032)


- Allow any localhost port in sandbox CORS config (#1047)


- Use TypeScript projectService for ESLint type-aware rules (#1049)


- Move Prettier out of ESLint into dedicated checks (#1050)


- Enable ESLint caching for faster local lint runs (#1051)


- Extend action buttons to entity detail page headers (#1024)


- Add integration tests (#1029)


- Amanda/fix pipeline run name format and output (#1060)


- [CLI] Support module level plugin override (#1062)


- Add MapField component to packages/core (#1066)


- [Git Ignore] Add `coverage.lcov` file in gitignore (from pytest coverage format) (#1065)


- Export timezone enum for correctly typed timezone to be passed to UserProvider (#1072)


- Enable mactl trigger_run create from pipeline triggerMap (#1052)


- Update docs and remove trigger example yaml file (#1061)


- Ayao/implement pipeline apply update path (#1059)


- Add disabled action support to the action menu (#1048)


- Add interpolatable action configs for per-row resolution (#1069)


- Update example docker image to use ghcr (#1083)


- Clear sandbox tabs for shipped features (#1070)


- Add tests for project list and detail views (#1071)


- Add docs/.claude agents and skills for documentation review team (#1058)


- [CLI] Separate entity `get()` function into raw and user facing part (#1067)


- Remove outdated python/.cursor folder (#1081)


- Revert baseUrl to / for custom domain (#1000)


- Revert baseUrl to /michelangelo/ until custom domain is verified (#1089)


- [mactl] load metadata with lazy way in pipeline dev-run plugin (#1092)


- Pass cluster affinity from labels (#1097)


- Add kill action to trigger runs (#1054)


- Bump versions for core to 0.2.0 and rpc to 0.1.0 (#1103)


- Introduce controlled expanded state to form-group (#1126)


- Bump core version up to 0.2.1 (#1127)


- Add list deployment page (#1123)


- Fix schedule update in Trigger run not reflecting cadence/temporal (#1128)


- Extend no-module-scope-test-setup to flag top-level describe scope (#1087)


- Add ESLint rule: types must live in types.ts files (#1068)


- Replace integration-test.sh with pytest suite from integration-test repo (#1151)


- Add cli e2e to workflow for integration test (#1156)


- Add deployment details page (#1135)


- Pass trigger execution timestamp as STARLARK_TIME to uniflow workflow (#1149)


- Make breadcrumb bar sticky with scroll-triggered shadow (#1150)


- Add useSchemaMiddleware hook (#1147)


- Add YAML scaffold support to useSchemaMiddleware (#1161)


- Add Ray cluster log persistence (proto + collector sidecar + log_url) (#1124)


-  KubeRay History Server setup in the local sandbox (#1133)


- Add logo above landing hero title (#1181)


- Fix Studio phase documentation links (#1176)


- Add targeted JavaScript API docs (#1182)


- Add target list page (#1155)


- Update landing page hero copy and add stats bar (#1188)


- Register auto generated version conversion functions to k8s scheme (#1204)


- Update RevisionSpec v2 with source field and structured git commit info (#1144)


- Fix/helm dependency build (#1197)


- Fix navigation when on detail page (#1192)


- Add missing yaml@^1.10.2 yarn.lock entry (#1200)


- Add Targets detail page (#1189)


- Add rollout fail stage (#1207)


- Snapshot internal PyTorch Lightning trainer + MovieLens demo (#1213)


- [MA-CLI] Require *args, **kwargs in plugin function signatures (#1225)


- Add missing tls config for temporal (#1223)


- Selim/export details page building blocks (#1239)


- Refresh landing hero with animated moiré background (#1210)


- Adopt UberMove fonts, remove bg animation, tighten navbar (#1262)


- Upgrade Ray to 2.51.2 (py3.9) / 2.55.1 (py≥3.10) via version markers (#1274)


- Update lightning_trainer_kwargs in MovieLens example training task (#1235)


- Print dev_run response so it is visible at default log level (#1240)


- Ma cli: support MACTL_RPC_SERVICE env override (#1280)


- Fix get --name flag silently listing instead of fetching (#1290)


- Add ability to customize detail view header via titleEnhancer (#1289)


- **michelangelo-ai/core:** 0.2.4 (#1308)


- Create NewReconciler constructor for Pipeline (#1305)


- Add last schedule timestamp from trigger to pipeline run (#1183)


- Condense javascript/CLAUDE.md to coding rules (#1320)


- Enforce semantic testing-library queries in core tests (#1321)


- Isolate per-plugin load failures so one broken plugin can't crash the CLI (#1312)


- Add ModelVariable to workflow/variables/_private (#1322)


- Add eslint-plugin-react with structural component rules (#1323)


- Add pre-apply hook registry for downstream policy enforcement (#1327)


- Add [plugin.packages] discovery for import-based plugin loading (#1331)


- Add filename-matches-export ESLint rule (#1326)


- Make michelangelo a shared-namespace package via pkgutil.extend_path (#1334)


- Rename no-event-handler-prefix → no-handler-mirror (#1341)


- Add require-handler-prefix; enforce handle* prefix on locally-defined handlers (#1325)


- Align all component versions to 0.3.0 (#1376)


- Extend filename-matches-export to hook files (#1328)


- Enforce strictCamelCase for function names (#1329)


- Add version-bump script (#1383)



### Refactoring


- **Worker:** Rename UAPI Plugin to Model (#448)


- **deployment+inference:** Decouple inference server and deployment components via Gateway abstraction (#706)


- **deployment:** Condition actor improvements and correct usage of plugin.GetState() and plugin.ParseStage  (#732)


- **inference+deployment:** Make user interfaces pluggable (#802)


- **workflowfx:** Replace TLS config field with fx injection (#867)


- **mactl:** Unify pipeline create/apply converters into one (#1096)


- **inferenceserver:** Adjust actors to support provisioning infenceserver in multiple clusters (#1139)


- **deployment:** Restructure condition actors to deploy to multiple inferenceserver targets (#1141)


- Remove pusher/types.py shim — types canonical in workflow/variables (#1221)


- Introduce MutationConfig and wrap mutation envelopes (#1195)


- Restructure ActionConfig as action + modal union (#1199)


- **types:** Type the accessor input so callers don't have to cast records (#1278)


- **registry:** APIRegistryClient delegates to APIClient.ModelService (#1302)


- Migrate kill trigger run to declarative MutationActionConfig (#1203)


- **python:** Hoist shared _internal utilities out of trainer (#1372)



### Testing


- **trainer:** Add comprehensive test suite for PyTorch Lightning trainer (#1360)




