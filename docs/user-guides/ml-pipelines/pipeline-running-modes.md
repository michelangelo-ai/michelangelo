# Pipeline Running Modes

Michelangelo AI provides four running modes that correspond to different stages of the machine learning development lifecycle. Each mode solves specific problems developers face when moving from initial experimentation to production deployment.

## The Problem: ML Development Complexity

Machine learning workflows have unique challenges:

* **Development Stage**: Need fast iteration and debugging  
* **Integration Stage**: Need to test pipeline configurations and dependencies  
* **Staging Stage**: Need production-like execution with safety controls  
* **Production Stage**: Need enterprise-grade reliability, scheduling, and monitoring

## Running Modes overview

| Mode | Development Stage | Problem Solved | Value Proposition |
| ----- | ----- | ----- | ----- |
| **Local Run** | Development Testing | "Can I test this change right now?" | **Instant** \- Zero provisioning time |
| **Remote Run** | Resource-Intensive Testing | "My laptop can't handle this dataset" | **2-5 mins** \- Quick cloud resources |
| **Pipeline Dev Run** | Build Integration | "Does my Dockerfile actually work?" | **20+ mins** \- Full image building |
| **Pipeline Run** | Production Deployment | "How do I deploy my committed code?" | **Varies** \- Only accepts committed code |

## Local Run Mode

### Why This Mode Exists

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

### Execution Timing

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

## Remote Run Mode

### Why This Mode Exists

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

### Execution Timing

* **Provisioning**: **2–5 mins**  
* **Execution**: Starts fast using prebuilt images  
* **Best for**: GPU workloads, large datasets, memory-intensive tasks

### Speed vs Process Trade-off

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

* `--workflow` (`cadence` | `temporal`)  
* `--cron`  
* `--file-sync`  
* `--yes`

## Pipeline Dev Run Mode

### Why This Mode Exists

Engineers must validate the **entire pipeline**, including container image builds, dependency resolution, and resume functionality — without pushing to production.

### Benefits

* **Full Pipeline Testing**  
* **Resumeable Execution**  
* **Build Validation** (Dockerfile \+ dependencies)  
* **Integration Simulation**

### When to Use

* **Stage**: Pipeline integration  
* **Scenario**: "Does my Dockerfile actually work?"  
* **Team**: ML engineers validating pipeline end-to-end

### Execution Timing

* **Provisioning \+ Building**: **20+ mins**  
* **Execution**: Depends on pipeline  
* **Best for**: Pre-production verification

### Completeness vs Speed Trade-off

* **Completeness**: High  
* **Speed**: Slowest (build \+ run)

### Usage

```shell
ma pipeline dev_run -f pipeline.yaml
ma pipeline dev_run -f pipeline.yaml --env DATASET_SIZE=1000
ma pipeline dev_run -f pipeline.yaml --resume_from my-run-123:train
```

## Pipeline Run Mode

### Why This Mode Exists

Enterprises need **production-grade pipeline execution** with full CI/CD, governance, monitoring, and rollback support.

### Benefits

* **Full CI/CD Pipeline**  
* **Version Control \+ Rollback**  
* **Enterprise Monitoring \+ SLAs**  
* **Automated Governance**

### When to Use

* **Stage**: Production  
* **Scenario**: "How do I deploy my committed code?"  
* **Team**: Production ML \+ platform teams

### Execution Timing

* **Provisioning**: Depends on CI/CD  
* **Execution**: Production-grade reliability  
* **Requirements**: Code must be committed

### Reliability vs Agility Trade-off

* **Reliability**: Maximum  
* **Agility**: Lower

### Usage

```shell
ma pipeline run -n my-namespace --name my-pipeline
ma pipeline run -n my-namespace --name my-pipeline --resume_from previous-run:step
```

## Decision Tree: Which Mode Should I Use?

### Stage-Based

* **Early Development** → Local Run  
* **Scaling Tests** → Remote Run  
* **Integration Testing** → Pipeline Dev Run  
* **Production Deployment** → Pipeline Run

### Concern-Based

| Concern | Mode |
| ----- | ----- |
| Fast iteration | Local Run |
| Need more compute | Remote Run |
| Validate image \+ pipeline | Pipeline Dev Run |
| Production reliability | Pipeline Run |