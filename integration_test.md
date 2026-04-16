# Michelangelo End-to-End Integration Test Infrastructure

To receive all notifications, click Tools -> Notification settings -> enable All comments and tasks

## Metadata

| Field | Value |
|---|---|
| **Authors** | weric@uber.com |
| **ERD uPlan URL** | d68e1299-1e57-4557-9eaf-04e74b3beb14 |
| **Project Summary** | Full-stack end-to-end integration test infrastructure for Michelangelo, exercising the Go backend (API server, controllermgr, worker), Python SDK and CLI (`ma`), JavaScript UI (project, pipeline, run pages), and complete training workflows using UniFLOW, Ray, and Spark — across both Cadence and Temporal workflow engines |
| **uOwn Asset** | Uber AI |
| **Date started** | Mar 06, 2026 |

---

## Value Proposition

Michelangelo is a full-stack ML platform spanning three language domains: Go (API server, controllermgr, worker), Python (SDK, CLI, pipeline DSL), and JavaScript (React UI). Today each layer has unit and component tests in isolation, but there is no automated test that exercises the full vertical slice — from the `ma` CLI registering a pipeline, through the API server and controllermgr reconciling it in Kubernetes, to the worker executing a real Ray or Spark training job, and the UI displaying the results.

Without end-to-end coverage, a breaking change in any one component (a CRD schema change, a broken controller reconcile loop, an API response format change) is only caught when a user reports it or an on-call engineer investigates a production incident. This ERD proposes a nightly CI integration test that boots the full Michelangelo stack in a local k3d sandbox, runs representative training pipelines for each executor type (UniFLOW, Ray, Spark), and performs UI smoke tests to verify the project, pipeline, and run pages respond correctly.

**Expected impact:**
- Catch cross-component regressions (Go ↔ Python ↔ JS) before they reach production
- Reduce time-to-detect pipeline-breaking changes from days to minutes
- Validate that `ma` CLI, Go services, and UI all agree on the CRD contract
- Give the team a reproducible environment for testing new features end-to-end

---

## Current Challenges

- **No cross-language E2E coverage**: Go, Python, and JavaScript tests run in separate CI jobs with no shared integration surface.
- **No UI regression tests**: The React UI has unit tests but no automated check that the project list, pipeline list, or run detail pages load real data from the API.
- **Sandbox is manual-only**: `mactl sandbox create` was designed for local developer use. Running it in CI requires solving image pre-warming, startup timing, and resource constraints on GitHub Actions runners.
- **Ray and Spark jobs untested end-to-end**: No automated test verifies that a Ray cluster is created, a RayJob completes, or that Spark submits and finishes a job through the Michelangelo controllermgr.
- **Two workflow engines, no shared test**: Cadence and Temporal are both supported but there is no test that runs the same pipeline through both engines to verify behavioral parity.
- **Examples image build time**: The task image (bert_cola + Spark + PyTorch) takes 30+ minutes to build from scratch. A pre-build and caching strategy is required for CI to be practical.

---

## Non-Goals

- GPU-accelerated training in CI (CPU-only runs; GPU via self-hosted runners is a follow-up)
- Inference pipeline testing (follow-on scope; see resource estimate section)
- Performance or load testing
- Testing Cadence UI or MinIO console
- Testing Uber-internal auth/authFx integrations (sandbox uses ma-minio credentials)
- Full Playwright UI test suite (in-scope: HTTP smoke tests; full UI E2E testing is a follow-up)

---

## Proposal (High Level)

### Three-tier workflow design

```
┌────────────────────────────────┐
│   build-examples-image.yaml    │  Triggered: push / PR / dispatch
│                                │  Builds task image (bert_cola + Spark
│   ghcr.io/.../examples:<tag>   │  + PyTorch + Ray), pushes to GHCR
│                                │  with GHA layer cache [1]
└──────────────┬─────────────────┘
               │ workflow_run (on success)
               ▼
┌─────────────────────────────────────────────────────────────────────┐
│   integration-test-sandbox.yaml                                     │
│                                                                     │
│   matrix: workflow_engine: [cadence, temporal]  (runs in parallel)  │
│                                                                     │
│   For each engine:                                                  │
│   1. Boot k3d sandbox (MySQL, workflow engine, MinIO, API server,   │
│      controllermgr, worker, kuberay-operator, spark-operator,       │
│      envoy, michelangelo-ui) [2][3]                                 │
│   2. UniFLOW test  — bert_cola training pipeline                    │
│   3. Ray test      — simple Ray training job [4]                    │
│   4. Spark test    — simple Spark training job [5]                  │
│   5. UI smoke test — project / pipeline / run pages return 200      │
│   6. Tear down sandbox                                              │
│                                                                     │
│   Triggers: workflow_run | nightly 03:00 UTC | workflow_dispatch    │
└─────────────────────────────────────────────────────────────────────┘
```

**References:**
[1] GitHub Actions cache: https://docs.github.com/en/actions/using-workflows/caching-dependencies-to-speed-up-workflows
[2] k3d — lightweight k3s in Docker: https://k3d.io/
[3] k3s — lightweight Kubernetes: https://k3s.io/
[4] KubeRay operator: https://ray-project.github.io/kuberay/
[5] Spark operator: https://github.com/kubeflow/spark-operator

### What each test validates

| Test | Go components | Python CLI | UI |
|---|---|---|---|
| UniFLOW (bert_cola) | API server (CRD CRUD), controllermgr (reconcile), worker (pod scheduling) | `ma pipeline apply`, `ma pipeline run`, poll | — |
| Ray | API server, controllermgr (RayJob), kuberay-operator [4] | `ma pipeline apply`, `ma pipeline run` | — |
| Spark | API server, controllermgr (SparkApplication), spark-operator [5] | `ma pipeline apply`, `ma pipeline run` | — |
| UI smoke | API server (HTTP/JSON) | — | Project list, detail; pipeline list; run detail |

---

## Affected Parties

- **Michelangelo Python SDK team** — owns integration test script and CI workflows
- **Michelangelo platform (Go) team** — sandbox resource YAMLs and CRD schemas must remain compatible with k3d deployment
- **Michelangelo UI team** — UI must be reachable at `http://localhost:8090`; all four route patterns must return HTTP 200 after test data is created
- **GitHub billing** — larger runners required; see Cost section

---

## Engineering Risk

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Ray/Spark jobs OOM on CI runner | Low (GCP VM ≥64 GB) | Test failure | Size GCP VM to ≥16 vCPU / 64 GB |
| MySQL slow startup on constrained runner | Mitigated | kubectl wait timeout | GCP VM has sufficient CPU; also pre-import `mysql:8.0` into k3d and tune probe settings |
| Ray/Spark demo pipeline YAMLs don't exist yet | High (current gap) | Blocked tests | Must be created as part of this work |
| `workflow_run` only fires from default branch | Known | Branch testing harder | Temporary `push` trigger on feature branches |
| Cadence/Temporal parity gaps surface | Medium | Flaky parallel jobs | Run sequentially if one engine is flaky; fix parity first |
| GCP VM runner registration requires org admin | Known | Blocks initial setup | Org admin must generate runner token at GitHub org settings |

---

## Design

### Platform: GitHub Actions + k3d on GCP VM (self-hosted runner)

The sandbox runs on **GitHub Actions** [6] using a **self-hosted runner** [7] registered on a GCP VM. The Kubernetes cluster is created by **k3d** [2], which wraps **k3s** [3] (a lightweight Kubernetes distribution) inside Docker containers on the runner host.

**Why k3d?** Michelangelo uses Kubernetes CRDs (`PipelineRun`, `Pipeline`, `Project`, `RayJob`, `SparkApplication`) that require a real Kubernetes API server with CRD support, RBAC, and pod scheduling. k3d provides this on a single Linux VM in ~10 seconds without cloud credentials or persistent infrastructure.

**Why GitHub Actions?** The Michelangelo repo is hosted on GitHub (github.com/michelangelo-ai/michelangelo). GitHub Actions provides native integration with the repository event model (`push`, `pull_request`, `workflow_run`, `schedule`), secret management, and artifact storage.

**Why self-hosted GCP VM instead of GitHub-hosted runners?**

MySQL 8.0's first-boot data directory initialization (`mysqld --initialize`) is CPU-intensive and takes 20+ minutes on a 2-CPU `ubuntu-latest` runner — well beyond any kubectl readiness probe timeout. The standard GitHub-hosted runner (`ubuntu-latest`) provides only 2 CPU / 7 GB RAM [7]. Larger GitHub-hosted runners (up to 16-core/64 GB) are available but require billing approval and add significant per-minute cost.

A self-hosted GCP VM eliminates both constraints: it provides ample CPU/RAM for MySQL cold start and all Ray/Spark task pods, and runner minutes are not billed by GitHub (only GCP VM cost applies). The workflow targets the GCP runner with `runs-on: [self-hosted, linux, gcp]` [7].

**Runner registration:** A GitHub org admin must generate a runner registration token at `https://github.com/organizations/michelangelo-ai/settings/actions/runners/new` and run `config.sh` on the GCP VM. See [Self-hosted runner docs][7] for step-by-step setup.

[6] GitHub Actions: https://docs.github.com/en/actions
[7] GitHub self-hosted runners: https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/about-self-hosted-runners

### Sandbox stack

All services run as pods inside the k3d cluster on the GitHub Actions runner:

```
k3d cluster: michelangelo-sandbox (k3s v1.31, Docker-in-Docker [2])
└── default namespace
    ├── mysql             Pod      MySQL 8.0 [8]   — API server + workflow engine storage
    ├── cadence | temporal Pod     Cadence [9] or Temporal [10] — workflow engine
    ├── minio             Pod      MinIO [11]      — S3-compatible blob store
    ├── michelangelo-apiserver   Pod   REST+gRPC CRD API (Go)
    ├── michelangelo-controllermgr Pod  Reconciles Pipeline/PipelineRun CRDs (Go)
    ├── michelangelo-worker      Pod   Executes UniFLOW task pods (Go)
    ├── envoy             Pod      HTTP proxy → michelangelo-ui
    └── michelangelo-ui   Deployment  React UI (TypeScript/React [12])
├── ray-system namespace
│   └── kuberay-operator  Deployment  KubeRay operator v1.4.2 [4]
└── spark-operator namespace
    └── spark-operator    Deployment  Spark Operator v2.x [5]
```

[8] MySQL 8.0: https://hub.docker.com/_/mysql
[9] Cadence: https://cadenceworkflow.io/
[10] Temporal: https://temporal.io/
[11] MinIO: https://min.io/
[12] React: https://react.dev/

### Testing both workflow engines: Cadence and Temporal

Cadence [9] and Temporal [10] are both supported by Michelangelo as the underlying workflow engine for pipeline runs. The integration test must validate both. We use a **GitHub Actions matrix** [13] to run two parallel jobs — one per engine — on the same commit, with independent sandboxes.

```yaml
# integration-test-sandbox.yaml (simplified)
jobs:
  sandbox-e2e:
    runs-on: [self-hosted, linux, gcp]
    strategy:
      matrix:
        workflow_engine: [cadence, temporal]
      fail-fast: false          # one engine failing does not cancel the other
    steps:
      - run: poetry run ma sandbox create --workflow ${{ matrix.workflow_engine }} ...
      - run: integration-test.sh
      - run: poetry run ma sandbox delete
```

**Parallel vs sequential:**

| Mode | Pros | Cons |
|---|---|---|
| **Parallel (matrix)** | Faster total wall time (~60 min instead of ~120 min) | Requires 2× runner cost; both sandboxes share the runner's Docker daemon |
| Sequential | Lower cost; simpler debugging | Slower; a Temporal failure blocks seeing Cadence result |

**Recommendation: parallel** using GitHub Actions matrix. Each engine gets its own k3d cluster with a unique name (`michelangelo-sandbox-cadence`, `michelangelo-sandbox-temporal`) to avoid port conflicts. If runner resource contention becomes an issue, fall back to sequential by setting `max-parallel: 1`.

[13] GitHub Actions matrix strategy: https://docs.github.com/en/actions/using-jobs/using-a-matrix-for-your-jobs

### Integration test script flow

```
integration-test.sh (runs inside the sandbox-e2e job, after sandbox is up)
│
├── 0. [CI workflow] ma sandbox create --workflow <engine>
│       k3d cluster created, all services running
│
├── 1. Upload bert_local.tar → s3://default/bert_local.tar
│       aws s3 cp --endpoint-url http://localhost:9091
│       (required before demo pipeline registration; uniflowTar references this S3 path)
│
├── 2. ma sandbox demo create pipeline
│       Registers: Project CR, training-pipeline, ray-pipeline, spark-pipeline
│
├── 3. UniFLOW test
│       ma pipeline run -n ma-dev-test --name training-pipeline
│       → poll kubectl get pipelinerun ... -o jsonpath='{.status.state}'
│       → assert PIPELINE_RUN_STATE_SUCCEEDED ✅
│
├── 4. Ray test
│       ma pipeline run -n ma-dev-test --name ray-pipeline
│       → poll → assert PIPELINE_RUN_STATE_SUCCEEDED ✅
│
├── 5. Spark test
│       ma pipeline run -n ma-dev-test --name spark-pipeline
│       → poll → assert PIPELINE_RUN_STATE_SUCCEEDED ✅
│
├── 6. UI smoke test (after test data exists)
│       curl http://localhost:8090/                                    → 200 ✅
│       curl http://localhost:8090/{projectId}                         → 200 ✅
│       curl http://localhost:8090/{projectId}/train/pipelines         → 200 ✅
│       curl http://localhost:8090/{projectId}/train/runs/{runId}      → 200 ✅
│
└── [CI workflow] ma sandbox delete   (always, even on failure)
```

### Resource requirements

Resource sizing is driven by the sum of: baseline services + task pods per pipeline type.

**Baseline services (always running):**

| Service | CPU request | RAM |
|---|---|---|
| k3s server + agent | 0.5 | 1 GB |
| MySQL 8.0 (first boot init) | 1.0 | 1 GB |
| Cadence or Temporal | 0.5 | 512 MB |
| MinIO | 0.2 | 256 MB |
| API server + controllermgr + worker | 0.5 | 768 MB |
| envoy + michelangelo-ui | 0.2 | 256 MB |
| kuberay-operator | 0.5 | 512 MB |
| spark-operator | 0.5 | 512 MB |
| **Baseline total** | **~4 CPU** | **~5 GB** |

**Training task pods (sequential execution, peak per job):**

| Pipeline type | Peak CPU | Peak RAM | Notes |
|---|---|---|---|
| UniFLOW (bert_cola, CPU-only) | 4 | 8 GB | PyTorch single-process training |
| Ray (1 head + 1 worker) | 4 | 8 GB | KubeRay managed [4]; small model |
| Spark (1 driver + 1 executor) | 4 | 8 GB | Local mode or 1 executor [5] |
| **Peak (one job at a time)** | **~8 CPU** | **~13 GB** | Jobs run sequentially |

**Runner sizing by scope:**

| Scope | Runner | CPU | RAM | Cost/run |
|---|---|---|---|---|
| UniFLOW + Ray + Spark (current) | GCP VM self-hosted [7] | 16+ | 64+ GB | GCP VM cost only |
| + Inference (CPU, small model) | GCP VM self-hosted [7] | 16+ | 64+ GB | GCP VM cost only |
| + Inference (GPU, vLLM) | GCP VM self-hosted w/ GPU [7] | 8+ | 32 GB + GPU | GCP VM cost only |

**Adding inference scope:** A CPU inference job (e.g., HuggingFace model serving) adds ~8–16 GB RAM for the model and ~4 CPU for inference workers. A 16-core/64 GB GCP VM is sufficient for small models (7B or smaller, quantized). GPU inference (vLLM with a 13B+ model [14]) requires a GCP VM with an attached GPU (e.g., A10G or T4).

[14] vLLM: https://docs.vllm.ai/

### Failure detection and root cause attribution

When a nightly test fails, finding the responsible change quickly is critical. We propose a two-layer approach: **step-level attribution** (which component owns the failure) and **AI-assisted root cause analysis** (which PR most likely introduced it).

**Step-level attribution:**

Each test step maps directly to an owner team. The failing step name in GitHub Actions is sufficient to route the alert:

| Failed step | Owner | First action |
|---|---|---|
| `Create sandbox` (MySQL/k3d timeout) | Platform (Go) team | Check `sandbox.py`, resource YAML changes in recent PRs |
| UniFLOW run timeout/fail | Python SDK team | Check worker pod logs (uploaded as artifact) |
| Ray run timeout/fail | Platform (Go) team | Check kuberay CRD, controllermgr Ray handling |
| Spark run timeout/fail | Platform (Go) team | Check spark-operator CRD, controllermgr Spark handling |
| UI smoke (HTTP non-200) | JavaScript UI team | Check envoy config, michelangelo-ui container logs |
| `Pull examples image` | Python SDK team | Check `build-examples-image.yaml` run for same commit |

**AI-assisted root cause (GenAI):**

We propose integrating an LLM step into the CI failure workflow to accelerate root cause identification. When the integration test fails, a post-failure step:

1. Collects: failed step name, last 100 lines of failed step log, `kubectl describe pod` output for failed pods, and the git diff of PRs merged since the last passing nightly run (via `gh pr list --state merged --base main`)
2. Submits this context to the Claude API [16] (via `claude -p` or a small Python script) with the prompt: *"Given this CI failure log and the following recent code changes, which change most likely caused this failure and why?"*
3. Posts the LLM's response as a comment on the GitHub Actions run summary

This surfaces a ranked list of suspect PRs and likely root cause hypotheses without requiring a human to manually correlate logs and git history. GitHub Copilot autofix [17] provides similar capability natively for some failure types.

[16] Claude API: https://docs.anthropic.com/en/api/
[17] GitHub Copilot autofix: https://docs.github.com/en/code-security/code-scanning/managing-code-scanning-alerts/about-autofix-for-codeql-alerts

**Artifact collection on failure:**

```yaml
- name: Collect debug artifacts
  if: failure()
  run: |
    kubectl get pods -A -o wide > pods.txt
    kubectl describe pods -n ma-dev-test >> pods.txt
    kubectl logs -n ma-dev-test -l app --tail=200 >> logs.txt
- uses: actions/upload-artifact@v4
  if: failure()
  with:
    name: sandbox-debug-${{ matrix.workflow_engine }}
    path: |
      pods.txt
      logs.txt
```

---

## APIs and Data

### CRDs exercised

| CRD | API Group | Operations |
|---|---|---|
| `Project` | `michelangelo.ai/v2` | create |
| `Pipeline` | `michelangelo.ai/v2` | create, get |
| `PipelineRun` | `michelangelo.ai/v2` | create, get/watch |
| `RayJob` | `ray.io/v1` [4] | create, get (via controllermgr) |
| `SparkApplication` | `sparkoperator.k8s.io/v1beta2` [5] | create, get (via controllermgr) |

### Storage

| Store | Bucket/DB | Contents |
|---|---|---|
| MinIO [11] | `default` | `bert_local.tar`, model artifacts |
| MinIO | `logs` | Task pod stdout/stderr |
| MySQL [8] | `michelangelo` | Project, Pipeline, PipelineRun records |
| MySQL | `cadence` or `temporal` | Workflow engine state |

---

## Integration Design for Dependencies

- **Build image first**: `build-examples-image.yaml` must succeed before the sandbox test starts. The `workflow_run` trigger enforces this on `main`. Feature branches use a temporary `push` trigger.
- **Image tag synchronization**: The `Compute image tag` step in the sandbox workflow derives the GHCR tag from the triggering branch name (sanitizing `/` → `-`, per `docker/metadata-action` [18] `type=ref,event=branch` behavior).
- **MinIO upload precondition**: `bert_local.tar` must exist at `s3://default/bert_local.tar` before `ma sandbox demo create pipeline`, as `training-pipeline.yaml` references that S3 path in `uniflowTar`.
- **Ray/Spark pipeline YAMLs**: `ray-pipeline.yaml` and `spark-pipeline.yaml` and lightweight example modules must be added to the demo set. This is a prerequisite for those test steps.
- **UI requires test data**: The UI smoke tests run after the pipeline tests to ensure project/pipeline/run data exists in the API for the UI to display.

[18] docker/metadata-action: https://github.com/docker/metadata-action

---

## Critical Design Issues

1. **Ray and Spark demo pipelines don't exist yet**: Lightweight `ray_example` and `spark_example` pipeline modules and their `demo/pipeline/*.yaml` manifests must be implemented as a prerequisite for those test steps.

2. **MySQL startup on constrained runners**: First-boot data directory initialization takes 3–8 min. Mitigated by pre-importing `mysql:8.0` into k3d containerd via `k3d image import` [2] before pod scheduling, and setting `initialDelaySeconds: 10`, `failureThreshold: 50`.

3. **Cadence/Temporal port conflicts in parallel matrix**: Two sandbox clusters must use different k3d port mappings to coexist on the same runner. The matrix job uses cluster names `michelangelo-sandbox-cadence` and `michelangelo-sandbox-temporal` with non-overlapping NodePort ranges.

4. **`workflow_run` limitation**: The sandbox workflow must exist on the default branch for `workflow_run` to fire [6]. Feature branch testing requires a temporary `push` trigger (added to the workflow file, removed before merge).

5. **k3d image import timing**: `mysql:8.0` is imported into k3d containerd while `ma sandbox create` runs in the background. The import must complete before the mysql pod's `ContainerCreating` state resolves into a pull — otherwise containerd will still attempt to pull from Docker Hub inside k3d.

---

## Monitoring, Rollout

### Run Cadence

**Daily (nightly) is the primary cadence** — not per-PR. Full test suite run time (~60 min per engine) makes per-PR execution too slow for developer feedback loops.

| Trigger | Schedule | Purpose |
|---|---|---|
| Nightly schedule | `0 3 * * *` UTC [6] | Primary regression signal on `main` |
| `workflow_run` | After `build-examples-image` succeeds on `main` | Catch image regressions on merge |
| `workflow_dispatch` | Manual | Pre-release validation, failure debugging |
| Temporary `push` on branch | During development | Branch-level testing (removed pre-merge) |

### Failure Alerting

- **GitHub Actions native notifications** [6]: Failure email to workflow watchers; failure badge on README
- **GitHub Actions run summary**: AI-assisted root cause comment posted as a step annotation (see Design section)
- **Debug artifacts**: Pod logs and `kubectl describe` output uploaded on failure via `actions/upload-artifact` [19]
- **Owner routing**: Failing step name maps directly to component owner (table in Design section)

[19] actions/upload-artifact: https://github.com/actions/upload-artifact

### Rollout Plan

**Phase 1** — UniFLOW baseline (current): Land CI workflows, verify bert_cola pipeline succeeds end-to-end on a 16-core runner.

**Phase 2** — Ray + Spark: Add lightweight example jobs and demo pipeline YAMLs; activate Ray and Spark test steps.

**Phase 3** — UI smoke tests + dual engine: Add curl smoke test step; enable Cadence/Temporal matrix with port conflict resolution.

**Phase 4** — AI root cause + inference (optional): Integrate Claude API failure analysis step; add inference pipeline test if GPU self-hosted runner is available.

---

## Migration

Not applicable. This is new CI infrastructure. The sandbox tooling (`ma sandbox`) already exists for local developer use; this work extends it to CI.

---

## Privacy and Security Considerations

### Data

This integration test runs entirely within an ephemeral GitHub Actions runner. No personal data (L1/L2/L3) is processed or stored.

- **MinIO**: `bert_local.tar` is a pre-built ML model tarball with no PII; all artifacts are destroyed at job end
- **MySQL**: stores Cadence/Temporal workflow state and Michelangelo CRD metadata; no user data
- All storage is destroyed when `ma sandbox delete` runs (or the runner terminates)
- No Databook tables are read or written

### Access Control and Encryption

- **GHCR**: `GITHUB_TOKEN` (auto-provisioned per-job, repository-scoped) [6]
- **MinIO**: `ma-minio`/`ma-minio` — sandbox credentials only, no production connectivity
- **MySQL**: `root`/`root` — sandbox credentials only, no production connectivity
- k3d cluster is network-isolated to the GitHub Actions runner host; no external ingress

### Handling User/External Input

No user input. All configuration is via environment variables defined in the workflow YAML (version-controlled). The Claude API call in the AI root cause step sends only CI logs and git diffs — no user or production data.

### LLMs

The AI-assisted root cause step uses the **Claude API** [16] (Anthropic's externally-hosted LLM). The input is: CI failure logs + git diffs of recent merged PRs. This data is:
- Not L1/L2 personal data
- Not production data
- Consists entirely of source code changes and CI stderr/stdout

No PII redactor is required. No L8+ exception is needed.

### Logging and Monitoring

GitHub Actions provides full step-level logs retained for 90 days [6]. Debug artifacts (pod logs) are uploaded on failure with the same retention. No additional logging infrastructure is required.

---

## Appendix

### Assumptions

1. A GCP VM self-hosted runner is registered with the `michelangelo-ai` GitHub org with labels `self-hosted, linux, gcp`.
2. The GCP VM has ≥16 CPU cores and ≥64 GB RAM so that MySQL cold-boot initialization, all baseline services, and peak Ray/Spark task pods run concurrently without OOM.
3. A simple Ray training job (1 head + 1 worker, small model) will complete within 1800s on the GCP runner.
4. A simple Spark job (local mode or 1 executor) will complete within 1800s on the GCP runner.
5. The UI serves pre-compiled static assets from the examples image; no live JS build in CI.
6. Cadence and Temporal sandboxes can coexist on the same runner using separate k3d clusters with non-overlapping ports.

### Alternatives Considered

| Alternative | Why discarded |
|---|---|
| Mock/stub Go services in Python tests | Does not exercise real CRD reconciliation or controller logic |
| Shared cloud k8s sandbox (persistent) | Expensive, shared-state flakiness, complex access control |
| Build examples image in every test run | 30+ min build time; GHCR pre-build + GHA layer cache [1] is the right tradeoff |
| Playwright for UI tests | Correct long-term approach; `curl` HTTP smoke tests are faster to implement as first coverage |
| `docker-compose` instead of k3d | Michelangelo CRDs require a real Kubernetes API; k3d [2] provides this without cloud credentials |
| Sequential Cadence/Temporal runs | Doubles wall time to ~120 min; parallel matrix at 60 min is acceptable |
| Per-PR integration tests | ~60 min run time is too slow for PR feedback; nightly is the right cadence |
| GitHub-hosted larger runners (16-core) | MySQL cold-boot initialization takes 20+ min under load on shared runners; adds ~$3.84/run in runner cost. GCP VM self-hosted runner eliminates both constraints. |

### FAQ

**Why not separate test jobs for Go, Python, and JS?**
The value of this test is precisely that it exercises all three together. Only a full-stack test catches cross-layer contract breakage (e.g., a Go CRD schema change that breaks the Python CLI).

**Why k3d and not a real cloud cluster?**
k3d [2] runs entirely on the GitHub Actions runner — no cloud credentials, no persistent infra, no cost beyond runner minutes. See the platform section for details.

**Why is prometheus/grafana excluded from CI?**
They are not needed for pipeline execution or UI smoke tests. Their images are slow to pull inside k3d and consume RAM needed for Ray/Spark task pods.

**When will Ray and Spark tests be active?**
After lightweight `ray_example` and `spark_example` jobs are implemented and added to the demo pipeline YAMLs.

**How do UI smoke tests work for a SPA?**
The envoy proxy serves the compiled React bundle. For a SPA, all routes return the same `index.html` with HTTP 200. The curl test validates that envoy, the UI container, and the routing config are all healthy.

**Can the AI root cause step leak production data?**
No. It only sends CI logs (stderr/stdout) and source code diffs. No production databases, user data, or secrets are included. See Privacy section.

### Cost Considerations

The integration test runs on a **self-hosted GCP VM** [7]. GitHub does not bill runner minutes for self-hosted runners — only GCP VM compute cost applies.

| Runner | Approx. GCP cost | Avg run time | Cost/run | Nightly/month |
|---|---|---|---|---|
| GCP VM (16 vCPU, 64 GB) — both engines parallel | ~$0.50–0.80/hr (e2-standard-16) | ~60 min | ~$0.50–0.80 | ~$15–25 |
| GCP VM (8 vCPU, 32 GB) — single engine | ~$0.25–0.40/hr (e2-standard-8) | ~60 min | ~$0.25–0.40 | ~$8–12 |

Compared to GitHub-hosted 16-core runners at ~$3.84/run × 2 engines = ~$7.68/run ($230+/month), the GCP VM self-hosted approach reduces CI cost by ~90% while providing more consistent and controllable resources.

**If a pre-existing GCP VM is already allocated for other uses**, the marginal CI cost is near zero (the VM is already running).
