# TODO Issues

TODOs found across the codebase that do not have a corresponding GitHub issue.

---

## Proto / API

### 1. Project phase transitions have no side effects
https://github.com/michelangelo-ai/michelangelo/issues/927
- **Type:** `feature`
- **File:** [`proto/api/v2/project.proto:150-156`](proto/api/v2/project.proto#L150)

**Is your feature request related to a problem? Please describe.**
Promoting a project to `STAGING` or `PRODUCTION` phase has no behavioral side effects — it is purely a cosmetic label. Users expect phase promotion to mean something (e.g. stricter access, deployment policies, audit requirements), but the system silently does nothing.

**Describe the solution you'd like**
Phase transitions should trigger real side effects:
- Access control changes (e.g. restrict who can modify a `PRODUCTION` project)
- Resource quota adjustments per phase
- Deployment policy enforcement (e.g. require approvals in `PRODUCTION`)
- Audit logging on every phase change

**Describe alternatives you've considered**
Keep phases as labels but add a validation webhook that enforces phase-based policies externally. This avoids controller changes but adds operational complexity.

**Additional context**
See `proto/api/v2/project.proto:150-156`. The comment explicitly notes "there is no side effect" as a known gap. This needs to be addressed before projects are used in production environments.

---

### 2. Add inter-job and anti-affinity to `JobSchedulingSpec`
https://github.com/michelangelo-ai/michelangelo/issues/928
- **Type:** `feature`
- **File:** [`proto/api/v2/job.proto:62`](proto/api/v2/job.proto#L62)

**Is your feature request related to a problem? Please describe.**
`JobSchedulingSpec` only supports `ResourceAffinity` for targeting resource pools. There is no way to express co-location or separation requirements between jobs. For distributed training, workers often need to be co-located with the parameter server for network performance, and there is currently no way to express this.

**Describe the solution you'd like**
Add two new fields to `JobSchedulingSpec`:
- **Inter-job affinity:** Schedule this job on the same node/zone as another specified job
- **Anti-affinity:** Spread this job away from other specified jobs for fault-tolerance

These should follow the K8s `PodAffinity`/`PodAntiAffinity` model.

**Describe alternatives you've considered**
Rely on node selectors and resource pool labels to approximate co-location. This is imprecise and cannot express relationships between specific job instances.

**Additional context**
See `proto/api/v2/job.proto:62`. This is particularly important for multi-node distributed training jobs (Ray, Spark) where network topology affects throughput significantly.

---

### 3. Add priority information to `JobPrioritySpec`
- **Type:** `feature`
- **File:** [`proto/api/v2/job.proto:156`](proto/api/v2/job.proto#L156)
  https://github.com/michelangelo-ai/michelangelo/issues/929
**Is your feature request related to a problem? Please describe.**
`JobPrioritySpec` only has a `preemptible` boolean. When multiple teams share a cluster, there is no way to express relative job priority, making fair scheduling and preemption decisions arbitrary.

**Describe the solution you'd like**
Add priority fields to `JobPrioritySpec`:
- `priority_class` — maps to K8s `PriorityClass` (e.g. `low`, `medium`, `high`, `critical`)
- `priority_value` — numeric value for fine-grained ordering
- `queue_priority` — priority within a team's scheduling queue

The scheduler should use these to order execution and decide which jobs to preempt under resource pressure.

**Describe alternatives you've considered**
Use resource pool labels to create separate high/low priority pools. This works but is coarse-grained and requires cluster-level configuration per team.

**Additional context**
See `proto/api/v2/job.proto:156`. Priority is closely related to the preemption work and should be designed alongside the scheduler's assignment strategy.

---

### 4. Deprecate `git_ref` and `git_branch` fields in `RevisionSpec`
- **Type:** `issue`
- **File:** [`proto/api/v2/revision.proto:50-53`](proto/api/v2/revision.proto#L50)
  https://github.com/michelangelo-ai/michelangelo/issues/930
**Describe the issue**
`RevisionSpec` contains two fields — `git_ref` and `git_branch` — that are marked for deprecation with no migration path documented. It is unclear what replaced them or whether any component still reads/writes these fields. Leaving deprecated fields with no formal deprecation notice causes confusion for API consumers.

**Steps to resolve**
1. Audit all read/write usages of `git_ref` and `git_branch` across Go and Python code
2. Identify the replacement fields (likely part of a newer commit/source spec)
3. Add `deprecated` proto annotations with a migration note pointing to the replacement
4. Remove the fields in the next major API version after a deprecation window

**Additional context**
See `proto/api/v2/revision.proto:50-53`. Both fields have separate `TODO: deprecate this field` comments with no issue or timeline attached.

---

### 5. Replace `ResourceSpec` custom types with K8s `resource.Quantity`
- **Type:** `feature`
- **File:** [`proto/api/v2/pod.proto:34`](proto/api/v2/pod.proto#L34)
  https://github.com/michelangelo-ai/michelangelo/issues/931
**Is your feature request related to a problem? Please describe.**
`ResourceSpec` uses plain integers/floats for CPU and memory (e.g. `cpu: 2`, `memory: 4`). This doesn't support fractional CPUs (`500m`) or proper memory units (`2Gi`), making it incompatible with the standard Kubernetes resource model and difficult to integrate with K8s schedulers.

**Describe the solution you'd like**
Replace the custom CPU/memory fields with K8s `resource.Quantity` strings:
- CPU: support millicores (e.g. `"250m"`, `"2"`)
- Memory: support binary/SI units (e.g. `"512Mi"`, `"2Gi"`)

Update all downstream consumers (scheduler, job client, inference server) to parse `Quantity` strings correctly.

**Describe alternatives you've considered**
Keep the existing fields but add a separate `resource_quantity` field alongside them for forward compatibility. Deprecate the old fields over time.

**Additional context**
See `proto/api/v2/pod.proto:34`. This change affects the scheduler, job client, and all resource-aware controllers and should be coordinated carefully to avoid breaking existing jobs.

---

### 6. Replace `hdfs_delegation_token` plain string with K8s Secret reference
- **Type:** `feature`
- **File:** [`proto/api/v2/pod.proto:184`](proto/api/v2/pod.proto#L184)
  https://github.com/michelangelo-ai/michelangelo/issues/932
**Is your feature request related to a problem? Please describe.**
`hdfs_delegation_token` is stored as a plain string in the pod spec and passed through the API. Tokens transmitted and stored this way may appear in API logs, etcd (unencrypted), `kubectl describe` output, and audit trails — all of which are security risks.

**Describe the solution you'd like**
Replace the plain string field with a K8s `SecretKeyRef` reference:
- Store HDFS tokens in a K8s `Secret`
- The pod spec references the secret by name/key, never the raw token value
- Mount the secret into the pod as an env var or file at runtime
- Ensure tokens are never surfaced in API responses or logs

**Describe alternatives you've considered**
Encrypt the token value before storing in the CRD. This is simpler but still exposes the token shape in logs and requires managing encryption keys separately.

**Additional context**
See `proto/api/v2/pod.proto:184`. HDFS tokens are short-lived credentials that must be rotated; using K8s Secrets also enables automatic rotation via external secret operators.

---

### 7. Add serving time dependencies to `InferenceServer` `BuildSpec`
- **Type:** `feature`
- **File:** [`proto/api/v2/inference_server.proto:27`](proto/api/v2/inference_server.proto#L27)
  https://github.com/michelangelo-ai/michelangelo/issues/933
**Is your feature request related to a problem? Please describe.**
`BuildSpec` for `InferenceServer` has no mechanism to declare runtime/serving-time dependencies. A model server often requires pre-loaded model artifacts, sidecar containers (e.g. token refresh, metrics exporter), or external service connections (e.g. feature store). There is currently no way to express these in the spec.

**Describe the solution you'd like**
Add a `serving_dependencies` field to `BuildSpec` supporting:
- Model artifact locations (blob storage paths to pre-load)
- Required sidecar containers with their configs
- External service endpoint dependencies
- Shared library or plugin requirements

**Describe alternatives you've considered**
Express dependencies via annotations on the `InferenceServer` CRD. This works but is unstructured and not validated by the API.

**Additional context**
See `proto/api/v2/inference_server.proto:27`. This is needed before inference serving can be used in production where models have non-trivial dependency chains.

---

## Config

### 8. MaCTL support for `worker_queue` annotation on Project CRD
- **Type:** `feature`
- **File:** [`go/cmd/controllermgr/config/base.yaml:13`](go/cmd/controllermgr/config/base.yaml#L13)
  https://github.com/michelangelo-ai/michelangelo/issues/934
**Is your feature request related to a problem? Please describe.**
The Cadence `taskList` is configured globally in `base.yaml` as a static fallback for all projects. The controller manager does support reading `michelangelo/worker_queue` from project annotations to override this per-project, but there is no way for users to set this annotation through MaCTL. Users must manually patch the CRD.

**Describe the solution you'd like**
- Add a `--worker-queue` flag to `mactl project create` and `mactl project apply`
- MaCTL writes the value as the `michelangelo/worker_queue` annotation on the `Project` CRD
- The controller manager prefers the per-project annotation over the global `taskList` fallback
- Document the flag in the project configuration guide

**Describe alternatives you've considered**
Require users to set the annotation manually via `kubectl annotate`. This works but is not discoverable and bypasses MaCTL's validation.

**Additional context**
See `go/cmd/controllermgr/config/base.yaml:13`. The controller manager already reads this annotation — only the MaCTL surface is missing.

---

## Python / CLI (mactl)

### 9. Selective CRD instance creation — don't create all CRDs for all services
- **Type:** `feature`
- **File:** [`python/michelangelo/cli/mactl/mactl.py:181`](python/michelangelo/cli/mactl/mactl.py#L181)
  https://github.com/michelangelo-ai/michelangelo/issues/935
**Is your feature request related to a problem? Please describe.**
`create_service_classes()` creates a CRD instance for every service ending in `Service`, regardless of whether the project uses it. This results in unnecessary CRD objects in etcd, clutters the API server, and may trigger reconcilers for services the project never needs.

**Describe the solution you'd like**
- Allow projects to declare which services they need (e.g. a `services:` field in the project YAML)
- Filter `create_service_classes()` to only instantiate declared services
- Log a warning if an undeclared service is referenced at runtime

**Describe alternatives you've considered**
Lazy creation — only create a CRD instance when it is first referenced. This avoids upfront over-creation but may cause latency on first use.

**Additional context**
See `python/michelangelo/cli/mactl/mactl.py:181`. This is a scalability concern as the number of services grows.

---

### 10. Auto-generate CLI arguments from CRD OpenAPI schema
- **Type:** `feature`
- **File:** [`python/michelangelo/cli/mactl/mactl.py:259`](python/michelangelo/cli/mactl/mactl.py#L259)

**Is your feature request related to a problem? Please describe.**
Argument parsing for CRD fields is hardcoded in mactl. Every time a new field is added to a CRD, a developer must manually update the argument parser. This creates persistent drift between the CRD schema and the CLI, and fields are often missing from the CLI long after being added to the API.

**Describe the solution you'd like**
- Read the CRD `OpenAPI` v3 schema from `spec.validation.openAPIV3Schema`
- Auto-generate `argparse` arguments from schema fields, types, and descriptions
- Validate user input against the schema before submitting to the API server
- Regenerate as part of the CRD build pipeline

**Describe alternatives you've considered**
Use a code generation tool (e.g. `datamodel-code-gen`) to generate Python classes from the OpenAPI schema, then build argument parsers from those classes.

**Additional context**
See `python/michelangelo/cli/mactl/mactl.py:259`. The comment indicates this was always the intended approach — it just hasn't been implemented yet.

---

### 11. Auto-generate mactl config templates from CRD spec
- **Type:** `feature`
- **File:** [`python/michelangelo/cli/mactl/mactl.py:304`](python/michelangelo/cli/mactl/mactl.py#L304)
  https://github.com/michelangelo-ai/michelangelo/issues/937
**Is your feature request related to a problem? Please describe.**
Parts of the mactl configuration are manually maintained and must be kept in sync with the CRD spec by hand. Every time a CRD field is added or renamed, the mactl config template also needs to be updated manually. This creates maintenance burden and is a common source of silent bugs.

**Describe the solution you'd like**
- Generate mactl config templates directly from CRD `OpenAPI` schemas at build time
- Run generation as part of the CRD build pipeline (alongside proto generation)
- Any new CRD field automatically appears in the generated config with its type and description

**Describe alternatives you've considered**
Write a linter that compares the CRD schema to the mactl config and fails CI when they diverge. This is easier to implement but doesn't eliminate the manual update burden.

**Additional context**
See `python/michelangelo/cli/mactl/mactl.py:304`. The comment says "this will be generated by CRD automatically later" — indicating this was always planned as generated code.

---

### 12. Add E2E tests for `mactl pipeline run`
- **Type:** `feature`
- **File:** [`python/michelangelo/cli/mactl/plugins/entity/pipeline/run.py:31`](python/michelangelo/cli/mactl/plugins/entity/pipeline/run.py#L31)
  https://github.com/michelangelo-ai/michelangelo/issues/938
**Is your feature request related to a problem? Please describe.**
The `mactl pipeline run` command has no end-to-end tests. Regressions in the run command can go undetected until a user reports a failure in production. The TODO specifically calls out two missing scenarios: normal run and resume from checkpoint.

**Describe the solution you'd like**
Add E2E tests covering:
- Successful pipeline run with a valid pipeline and project
- Resume from checkpoint using `--checkpoint` flag
- Run against a non-existent pipeline (expect a clear error message)
- Run with missing required parameters (expect validation error before API call)
- Run without authentication (expect auth error)

**Describe alternatives you've considered**
Add integration tests that mock the API server. Faster than full E2E but doesn't catch issues in the API call path or auth layer.

**Additional context**
See `python/michelangelo/cli/mactl/plugins/entity/pipeline/run.py:31`. The comment also mentions resume-from-checkpoint as a specific scenario to cover.

---

### 13. Add E2E tests for `get_pipeline_config_and_tar()`
- **Type:** `feature`
- **File:** [`python/michelangelo/cli/mactl/plugins/entity/pipeline/create.py:25`](python/michelangelo/cli/mactl/plugins/entity/pipeline/create.py#L25)
  https://github.com/michelangelo-ai/michelangelo/issues/939
**Is your feature request related to a problem? Please describe.**
`get_pipeline_config_and_tar()` packages the pipeline config and source code into a tar archive for upload. It is only covered by coverage-only tests that assert nothing meaningful. A bug here would silently produce a corrupt or incomplete archive, causing confusing failures later in the pipeline lifecycle.

**Describe the solution you'd like**
Add E2E tests that:
- Verify the tar archive contains the expected files with correct contents
- Verify the embedded config YAML is valid and matches the input
- Test with pipelines that have multiple source files and nested directories
- Test error cases: missing files, invalid config YAML, and permission errors

**Describe alternatives you've considered**
Snapshot testing — compare the tar output to a known-good reference archive. Fast to write but brittle when file contents legitimately change.

**Additional context**
See `python/michelangelo/cli/mactl/plugins/entity/pipeline/create.py:25`. This function is on the critical path for every pipeline upload — it needs robust test coverage.

---

### 14. Rewrite coverage-only tests in `pipeline/create_test.py`
- **Type:** `issue`
- **File:** [`python/michelangelo/cli/mactl/plugins/entity/pipeline/create_test.py:260,296,358,411`](python/michelangelo/cli/mactl/plugins/entity/pipeline/create_test.py#L260)
  https://github.com/michelangelo-ai/michelangelo/issues/940
**Describe the issue**
Four tests in `create_test.py` are explicitly annotated as "for coverage only" with no meaningful assertions. These tests inflate coverage metrics but provide no safety net — the code could be completely broken and all four tests would still pass.

**Steps to resolve**
1. Review what each of the four test functions is actually exercising
2. Rewrite with clear intent, descriptive names, and real assertions on return values, state changes, or raised exceptions
3. Add edge case coverage (empty inputs, invalid types, boundary values)
4. Remove the `TODO` comment once tests are substantive

**Additional context**
See `create_test.py` lines 260, 296, 358, 411. Coverage-only tests are worse than no tests — they create false confidence and make it harder to spot the gap.

---

### 16. Support multiple GVKs per object in ingester controller
- **Type:** `feature`
- **File:** [`go/components/ingester/controller.go`](go/components/ingester/controller.go)
  https://github.com/michelangelo-ai/michelangelo/issues/942

**Is your feature request related to a problem? Please describe.**
`scheme.ObjectKinds()` can return multiple GVKs for a single Go type (e.g. a type registered under both `v1` and `v1beta1`). The ingester controller currently always takes `gvks[0]` in `SetupWithManager`, `handleDeletion`, and `handleDeletionAnnotation`. This means only one GVK is ever used, which may be non-deterministic or incorrect when a type is registered under multiple versions.

**Describe the solution you'd like**
Decide on an explicit GVK selection strategy:
- Prefer the storage version (hub version) — requires querying the API server or scheme for the storage version
- Require exactly 1 GVK and return an error if multiple are found
- Allow per-kind configuration of the preferred GVK

**Describe alternatives you've considered**
Keep the current `gvks[0]` behavior but add a warning log when multiple GVKs are returned, so the issue is visible without breaking existing functionality.

**Additional context**
Affected locations: `SetupWithManager` (controller name + For() registration), `handleDeletion` (TypeMeta for Delete call), `handleDeletionAnnotation` (TypeMeta for Delete call). Currently all three use `gvks[0]` without checking if multiple GVKs exist.

---

### 15. Fix path retrieval in `pipeline/apply.py` to read from Project CRD
- **Type:** `issue`
- **File:** [`python/michelangelo/cli/mactl/plugins/entity/pipeline/apply.py:25,42`](python/michelangelo/cli/mactl/plugins/entity/pipeline/apply.py#L25)
  https://github.com/michelangelo-ai/michelangelo/issues/941
**Describe the issue**
`apply.py` uses heuristic/hardcoded path logic to find the pipeline source path instead of reading it from the `Project` CRD. There is commented-out code showing an incomplete attempt to use `commit.branch` and `commit.git_ref`. This means the path used during `apply` may not match the actual project source location, causing silent failures or applying the wrong pipeline config.

**Steps to resolve**
1. Fetch the `Project` CRD for the given namespace/name via the API
2. Read the pipeline source path from the project spec
3. Remove the hardcoded/heuristic path logic
4. Validate the retrieved path exists and is readable before proceeding
5. Add a test that verifies path retrieval uses the Project CRD and not a hardcoded fallback

**Additional context**
See `apply.py:25` and `apply.py:42`. The commented-out block at line 27–39 shows the intended direction but was never completed.
