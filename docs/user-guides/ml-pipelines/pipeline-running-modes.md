---
last_verified: 2026-11-05
---

# Pipeline Running Modes

Michelangelo AI provides four running modes that correspond to different stages of the machine learning development lifecycle. Each mode solves specific problems developers face when moving from initial experimentation to production deployment.

## What you'll learn

* The four pipeline running modes and when to use each one
* Trade-offs between speed, completeness, and reliability
* How to choose the right mode for your development stage

## The problem: ML development complexity

Machine learning workflows have unique challenges:

* **Development Stage**: Need fast iteration and debugging  
* **Integration Stage**: Need to test pipeline configurations and dependencies  
* **Staging Stage**: Need production-like execution with safety controls  
* **Production Stage**: Need enterprise-grade reliability, scheduling, and monitoring

## Running modes overview

| Mode | Development Stage | Problem Solved | Value Proposition |
| ----- | ----- | ----- | ----- |
| **Local Run** | Development Testing | "Can I test this change right now?" | **Instant** \- Zero provisioning time |
| **Remote Run** | Resource-Intensive Testing | "My laptop can't handle this dataset" | **2-5 mins** \- Quick cloud resources |
| **Pipeline Dev Run** | Pipeline Integration | "Does my whole pipeline run through the control plane?" | **Tracked** \- Creates a resumable `PipelineRun` |
| **Pipeline Run** | Production Deployment | "How do I deploy my committed code?" | **Varies** \- Only accepts committed code |

## Local Run mode

### Why this mode exists

Data scientists and ML engineers need to iterate quickly on workflow logic without the overhead of containers, clusters, or configuration files. The \#1 pain point in ML development is slow feedback loops.

### Benefits

* **Zero Setup Time**: Run immediately without any infrastructure  
* **Instant Feedback**: See results in seconds, not minutes  
* **Full Debugging**: Use your IDE, debugger, and local tools  
* **Cost Savings**: No cloud compute costs during development

### When to Use

* **Stage**: Initial development and experimentation
* **Scenario**: "Can I test this change right now?" — Need immediate feedback
* **Team**: Individual data scientists prototyping
* **Duration**: Hours to days during active development

### Execution timing

* **Provisioning**: **Instant** (0 seconds)  
* **Execution**: **Immediate** — Runs your code instantly  
* **Best for**: Quick iterations, debugging, small datasets

### Speed vs Accuracy Trade-off

* **Speed**: Fastest possible execution  
* **Accuracy**: Lower (local environment may differ from production)

### Usage

```shell
poetry run python workflow.py
```

## Remote Run mode

### Why this mode exists

Teams need to test workflows with **larger datasets and compute** without waiting for slow image builds. This solves: "My workflow works locally, but I need real compute power right now."

### Benefits

* **Better Resources**: Use cloud compute without local hardware limits  
* **Skip Build Times**: Reuse existing images  
* **Scale Testing**: Validate workflows at realistic data scale  
* **Fast Feedback Loop**: No CI/CD delays

### When to Use

* **Stage**: Functional \+ scaling tests
* **Scenario**: "My laptop can’t handle this dataset"
* **Team**: Devs validating compute-heavy logic

### Execution timing

* **Provisioning**: **2–5 mins**  
* **Execution**: Starts fast using prebuilt images  
* **Best for**: GPU workloads, large datasets, memory-intensive tasks

### Speed vs process trade-off

* **Speed**: Fast (no build pipeline)  
* **Process**: Lightweight governance

### Usage

```shell
poetry run python workflow.py remote-run --storage-url s3://my-bucket/workflows --image my-workflow:latest
```

### Required

* `--storage-url`  
* `--image`

### Optional

* `--workflow` (`cadence` | `temporal`, default `cadence`)  
* `--cron`  
* `--file-sync`  
* `--env KEY=VALUE` (alias `--environ`)  
* `--execution-timeout-seconds`  
* `--yes`

## Pipeline Dev Run mode

### Why this mode exists

Engineers must validate the **entire pipeline** as the control plane runs it, including resume functionality, without registering a pipeline. The run uses the image named by the `michelangelo/uniflow-image` annotation in your pipeline YAML; the open-source **Image Build** stage resolves that annotation and doesn't build an image.

### Benefits

* **Full Pipeline Testing**  
* **Resumable Execution**  
* **Tracked Runs**: creates a `PipelineRun` you can query with `ma pipeline_run get`  
* **Integration Simulation**

### When to Use

* **Stage**: Pipeline integration  
* **Scenario**: "Does my whole pipeline run through the control plane?"  
* **Team**: ML engineers validating pipeline end-to-end

### Execution timing

* **Provisioning**: Same compute as a Pipeline Run; no image build
* **Execution**: Depends on pipeline
* **Best for**: Pre-production verification

### Completeness vs speed trade-off

* **Completeness**: High  
* **Speed**: Slower than Remote Run (goes through the API server and controller)

### Usage

```shell
ma pipeline dev-run -f pipeline.yaml
ma pipeline dev-run -f pipeline.yaml --env=DATASET_SIZE=1000
ma pipeline dev-run -f pipeline.yaml --resume_from=my-run-123:train
ma pipeline dev-run -f pipeline.yaml --file-sync
```

## Pipeline Run mode

### Why this mode exists

Enterprises need **production-grade pipeline execution** with full CI/CD, governance, monitoring, and rollback support.

### Benefits

* **Registered Pipeline**: the `Pipeline` entity records the git branch and commit it was created from  
* **Scheduling**: attach [triggers](./set-up-triggers.md) for cron and backfill runs  
* **Visible in MA Studio**: runs appear under their pipeline  
* **Resumable**: restart a failed run from a step with `--resume_from`

### When to Use

* **Stage**: Production  
* **Scenario**: "How do I deploy my committed code?"  
* **Team**: Production ML \+ platform teams

### Execution timing

* **Provisioning**: Register the pipeline first with `ma pipeline apply` or `ma pipeline create`
* **Execution**: Runs the registered pipeline on the control plane
* **Requirements**: Code must be committed

### Reliability vs agility trade-off

* **Reliability**: Maximum  
* **Agility**: Lower

### Usage

The `namespace` flag specifies your project.

```shell
ma pipeline run --namespace="my-project" --name="my-pipeline"
ma pipeline run --namespace="my-project" --name="my-pipeline" --resume_from=previous-run:step
```

## Remote Run vs Pipeline Dev Run: what's actually different

From the "why this mode exists" framing above, Remote Run and Pipeline Dev Run can look interchangeable — both let you test against real compute without registering a pipeline. The distinction that actually matters when choosing between them is architectural, not just "which dev stage am I in":

| | Remote Run | Pipeline Dev Run |
| ----- | ----- | ----- |
| Invoked via | `python workflow.py remote-run` (a Uniflow feature, not an `ma` command) | `ma pipeline dev-run -f pipeline.yaml` |
| Routed through the Michelangelo AI API server + controller? | No — submits directly to Cadence/Temporal | Yes |
| Creates a `PipelineRun` entity? | No | Yes |
| Resumable with `--resume_from`? | No | Yes |
| Visible in MA Studio? | No | No — no parent `Pipeline` entity is created either, so there's nothing to attach it to in Studio's pipeline list. The run itself is still a real, queryable entity via `ma get pipeline_run` / kubectl |
| Requires a prebuilt image? | Yes — you pass `--image` | Yes — named by the `michelangelo/uniflow-image` annotation in `pipeline.yaml` |

:::tip
If you'll need to come back to a run later, share it with a teammate, or resume it from a failed step, use **Pipeline Dev Run** — it's the only one of the two that creates a trackable entity. Reach for **Remote Run** when you just need real compute for a quick check and don't need a tracked run.
:::

Job scheduling itself is identical either way: both ultimately submit to the same Cadence/Temporal worker, which is what talks to the Ray/Spark compute cluster. The difference is entirely in what happens *before* that — whether the Michelangelo AI control plane is involved at all.

See the [CLI reference](../reference/cli.md#differences-between-dev-run-and-remote-run) for the full breakdown, including how `--file-sync` behaves differently under each mode.

## Decision tree: which mode should I use?

### Stage-based

* **Early Development** → Local Run  
* **Scaling Tests** → Remote Run  
* **Integration Testing** → Pipeline Dev Run  
* **Production Deployment** → Pipeline Run

### Concern-based

| Concern | Mode |
| ----- | ----- |
| Fast iteration | Local Run |
| Need more compute | Remote Run |
| Validate the full pipeline | Pipeline Dev Run |
| Production reliability | Pipeline Run |