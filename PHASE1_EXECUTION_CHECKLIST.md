# Documentation Improvement - Phase 1 Execution Checklist

**Status**: Ready for Implementation
**Target Completion**: 1-2 weeks
**Team Size**: 2-3 (Engineer + Tech-writer + PM coordination)

---

## Task #10: Create and Apply Terminology Glossary

**Owner**: Tech-writer
**Effort**: 3-4 hours
**Status**: Ready to start

### Deliverable
Create `/docs/TERMINOLOGY.md` with clear definitions:

```markdown
# Michelangelo Terminology Guide

## Core Concepts

### Task
- Python function decorated with `@task`
- Discrete unit of work (data prep, training, evaluation)
- Runs in a container on Kubernetes (Ray or Spark)
- Can be cached and retried independently

### Workflow
- Python function decorated with `@workflow`
- Orchestrates one or more tasks
- Controls task sequencing, branching, and loops
- Runs in Cadence/Temporal worker (compiled to Starlark)

### Pipeline
- Deployable instance of a workflow
- Has its own YAML configuration (pipeline.yaml)
- Managed via `ma` CLI
- Can be triggered on schedule or manually

### Context
- Entry point for running a workflow
- Holds runtime information (environment variables, input arguments)

### PipelineRun
- Single execution instance of a pipeline
- Has its own status and logs
- Can be resumed if it fails

### Trigger / TriggerRun
- Scheduling policy bound to a pipeline revision
- Supports cron expressions or manual execution
- Creates PipelineRuns on schedule

### Uniflow
- Python-first framework for defining ML workflows
- Provides @task and @workflow decorators
- Enables local and distributed execution
```

### Apply Glossary
After creation, update these files to use consistent terminology:

1. **docs/user-guides/train-and-register-a-model.md**
   - Line ~5: Clarify "training task" vs "training workflow"
   - Line ~40: "Training inputs" section needs context

2. **docs/user-guides/set-up-triggers.md**
   - Throughout: Clarify "pipeline" vs "pipeline revision"
   - Line ~35: Explain "Parameter IDs" using glossary terms

3. **docs/user-guides/ml-pipelines/pipeline-management.md**
   - Table comparing "Standard Workflows" and "Custom Workflows" - align with glossary

4. **docs/user-guides/index.md**
   - Line ~4: Add reference to glossary for first-time readers

---

## Task #11: Fix Cross-References and Navigation Links

**Owner**: Tech-writer
**Effort**: 2-3 hours
**Status**: Ready to start

### Primary Fix: docs/user-guides/index.md (Lines 25-27)

**Current (BROKEN)**:
```markdown
| **Data Preparation** | [link](https://github.com/michelangelo-ai/michelangelo/wiki/Prepare-Your-Data) |
| **Model Training** | [link](https://github.com/michelangelo-ai/michelangelo/wiki/Train-and-Register-a-Model) |
| **Model Registry** | [link](https://github.com/michelangelo-ai/michelangelo/wiki/Model-Registry-Guide) |
```

**Fixed**:
```markdown
| **Data Preparation** | [Prepare your data](./prepare-your-data.md) |
| **Model Training** | [Train and register a model](./train-and-register-a-model.md) |
| **Model Registry** | [Model registry guide](./model-registry-guide.md) |
```

### Search and Replace Tasks

Use grep to find all GitHub wiki links:
```bash
grep -r "github.com/michelangelo-ai/michelangelo/wiki" docs/user-guides/
```

Replace pattern: `https://github.com/michelangelo-ai/michelangelo/wiki/X` → `./x-file.md`

### Add Navigation Breadcrumbs

1. **At top of ml-pipelines/ guides**:
   ```markdown
   > [Back to User Guides](../index.md)
   ```

2. **At bottom of guides linking to next steps**:
   Replace external URLs with local paths

3. **In set-up-triggers.md**:
   Add prerequisite links to prepare-your-data.md and train-and-register-a-model.md

4. **In getting-started.md**:
   Add links to what comes after (model registry, deployment)

### Validation
- [ ] No more `github.com/michelangelo-ai` links in docs/user-guides/
- [ ] All relative links (./file.md) work
- [ ] Breadcrumb navigation present where appropriate
- [ ] Build documentation (if available) shows no broken links

---

## Task #12: Revise User Journey in Index Documentation

**Owner**: Tech-writer
**Effort**: 4-5 hours
**Status**: Ready to start

### Goal
Make docs/user-guides/index.md answer: **"As a new user, what should I read first?"**

### Current Issues
- Mentions "Get Started" but doesn't clearly guide readers
- Tutorial table exists but lacks a narrative
- Unclear how to progress from one guide to the next
- ML Pipelines section (separate dir) not positioned in user journey

### Revised Structure

```markdown
# Michelangelo User Guides

This guide provides step-by-step instructions to build, train, and deploy machine
learning models at scale using Michelangelo's unified ML platform.

## Getting Started: Complete ML Workflow

New to Michelangelo? Follow this path to build your first ML model end-to-end:

### Step 1: Prepare Your Data
Learn to transform and prepare datasets using Ray and Spark.
- **Guide**: [Prepare Your Data](./prepare-your-data.md)
- **Topics**: Load CSVs, clean data, create train/validation splits
- **Time**: ~30 minutes

### Step 2: Train Your Model
Train models locally or at scale with distributed computing.
- **Guide**: [Train and Register a Model](./train-and-register-a-model.md)
- **Topics**: scikit-learn training, Lightning Trainer SDK, Ray scaling
- **Time**: ~45 minutes

### Step 3: Register and Manage Models
Version, track, and manage trained models with MLflow integration.
- **Guide**: [Model Registry Guide](./model-registry-guide.md)
- **Topics**: Model packaging, versioning, schema definition
- **Time**: ~20 minutes

### Step 4: Deploy Your Model (Coming Next)
Deploy models for real-time inference and batch scoring.
- **Guide**: [Model Deployment](./model-deployment.md) _(in development)_
- **Topics**: Triton Inference Server, scaling, monitoring
- **Coming**: March 2026

---

## Using ML Pipelines for Workflow Orchestration

All of the above steps can be orchestrated using **ML Pipelines** and the **Uniflow** framework.
This allows you to:
- Define workflows as Python code (no YAML)
- Run locally or at production scale
- Cache results and resume failed steps
- Schedule recurring runs

### Ready to Learn Pipelines?
Start with **[ML Pipelines Overview](./ml-pipelines/index.md)** to understand:
- How to define tasks and workflows
- Running modes (local, remote, production)
- Caching and resume features

Then follow **[Getting Started with Pipelines](./ml-pipelines/getting-started.md)** for a
hands-on tutorial building your first pipeline.

---

## Advanced Topics

### [Set Up Triggers](./set-up-triggers.md)
Schedule recurring pipeline runs with cron expressions.
*(Prerequisite: Complete Getting Started path above)*

### [Project Management for ML Pipelines](./project-management-for-ml-pipelines.md)
Create and configure MA Studio projects.
*(For operators and team leads)*

### [CLI Reference](./cli.md)
Command-line tools for pipeline and project management.
*(Reference guide for all ma commands)*

---

## Learning by Examples

Choose a tutorial based on your ML domain:

### Traditional Machine Learning
[Boston Housing Regression](#) - Predict house prices using XGBoost

### Natural Language Processing
[BERT Text Classification](#) - Classify text using transformer models
[GPT Fine-tuning](#) - Train language models with LoRA

### Recommendation Systems
[Amazon Books Recommendation](#) - Build dual-encoder systems

---

## What You'll Learn

By the end of these guides, you'll be able to:
- Prepare datasets efficiently using Ray and Spark
- Train models both locally and with distributed computing
- Register models and version them
- Deploy models for real-time serving
- Schedule and automate ML workflows with pipelines

---

## Documentation Structure
- **Getting Started Path**: 4 guides covering complete ML workflow
- **Pipeline Guides**: 7 specialized guides for orchestration
- **Reference**: CLI, project management, and architecture docs
```

### Key Changes
1. Clear "Getting Started Path" with time estimates
2. Explain what Pipelines are and when to use them
3. Link to ML Pipelines guides with context
4. Move triggers and project management to "Advanced Topics"
5. Add "Coming Next" placeholder for deployment guide
6. Link to actual examples (update paths)
7. Add "Documentation Structure" section showing map

### Validation
- [ ] New users can follow the path 1-2-3-4
- [ ] Clear why they'd want pipelines
- [ ] Examples are linked and current
- [ ] Time estimates are realistic
- [ ] "Coming Soon" clearly marked as in-progress work

---

## Task #9: Review project-management-for-ml-pipelines.md

**Owner**: Engineer (or Tech-writer)
**Effort**: 2-3 hours
**Status**: Ready to start

### Assessment Checklist

- [ ] **Completeness**: Are all project management concepts covered?
- [ ] **Accuracy**: Do examples work with current Michelangelo version?
- [ ] **Clarity**: Can an operator understand how to create a project?
- [ ] **Integration**: Does it connect properly to other guides?
- [ ] **Audience**: Is it aimed at the right user (operator vs engineer)?
- [ ] **Overlap**: Any duplication with other guides?
- [ ] **Missing**: What topics are not covered?

### Output Required
Document findings in format:
```markdown
# Assessment: project-management-for-ml-pipelines.md

## Status
[GOOD / NEEDS REVISION / INCOMPLETE]

## Findings
- [Issue 1]
- [Issue 2]

## Recommendations
- [Fix 1]
- [Fix 2]

## Coordination Needed
- [With Engineer? Tech-writer? Team lead?]
```

---

## Task #13: Create Model Deployment Guide

**Owner**: Engineer
**Effort**: 6-8 hours
**Status**: Requires technical expertise

### Deliverable: docs/user-guides/model-deployment.md

### Required Sections

1. **Introduction**
   - Why deploy models
   - Deployment options (batch vs real-time)
   - Overview of Triton Inference Server

2. **Setting Up Triton**
   - Docker container basics
   - Config.pbtxt explanation
   - Python backend setup

3. **Deploying a Registered Model**
   - How to download model from registry
   - Packaging for Triton
   - Config example

4. **Batch Inference**
   - Running batch jobs
   - File format expectations
   - Performance tuning

5. **Real-Time Inference**
   - REST API setup
   - Request/response format
   - Example client code

6. **Monitoring & Scaling**
   - Health checks
   - Logging
   - Scaling replicas
   - Performance metrics

7. **Example: Full End-to-End**
   - Complete code from train → register → deploy → inference

8. **Troubleshooting**
   - Common deployment issues
   - Performance problems
   - Debugging tips

### Code Examples Needed
- Full working Triton config
- Python client example
- Batch inference script
- Load testing example

### Links to Update After Completion
- docs/user-guides/index.md (line 28)
- docs/user-guides/train-and-register-a-model.md (line 195)
- docs/user-guides/ml-pipelines/index.md (deployment path)

---

## Execution Order Recommendation

### Week 1 (Days 1-3): Parallel Work
Start all 5 tasks simultaneously:
- Tech-writer: #10, #11, #12 (can work in parallel)
- Engineer: #9, #13 (can work in parallel)
- PM: Coordinate, unblock as needed

### Week 1 (Days 4-5): Integration
- Tech-writer completes #10, #11
- Integrate glossary and links into new index (#12)
- Engineer reviews deployment guide draft

### Week 2: Finalization
- Engineer finalizes #13 (deployment guide)
- Tech-writer reviews engineer work
- All updates integrated and tested
- Final QA pass

---

## Definition of Done

Each task is complete when:

**#10**: Glossary created, applied consistently across 4 files, no lingering terminology conflicts
**#11**: All GitHub wiki links replaced, all relative links tested, breadcrumbs in place
**#12**: New index.md approved by tech-writer and PM, clear user journey established
**#9**: Assessment document written, recommendations for #1-3 phases clear
**#13**: Complete guide written, code examples tested, all links updated

---

## Success Criteria

After Phase 1 completion:
- ✅ New user can follow: prepare → train → register → deploy (complete workflow)
- ✅ All terminology consistent throughout docs
- ✅ All links internal and working
- ✅ No "Coming Soon" sections in critical path
- ✅ Clear information architecture and navigation
- ✅ Team ready to move to Phase 2 (deduplication, advanced topics)
