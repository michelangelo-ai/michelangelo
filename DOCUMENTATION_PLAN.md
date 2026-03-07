# Michelangelo Documentation Improvement Plan

**Date**: March 6, 2026
**Scope**: docs/user-guides/
**Goal**: Ensure consistent, comprehensive documentation for open source project success

---

## Executive Summary

The Michelangelo documentation structure is well-organized with clear sections on data preparation, model training, and pipeline management. However, the current state has:

- **Critical gaps**: Model deployment guide is missing (marked "Coming Soon")
- **Consistency issues**: Terminology, cross-references, and duplicate coverage
- **Fragmented user journey**: No clear path from high-level overview to hands-on tutorials
- **Incomplete infrastructure docs**: Architecture and troubleshooting need expansion

**Priority**: Address critical gaps and establish terminology consistency to improve open source onboarding.

---

## Documentation Audit Results

### Current Documentation Structure

#### Level 1: Top-Level User Guides (docs/user-guides/)
| File | Status | Audience | Key Issue |
|------|--------|----------|-----------|
| **index.md** | ✅ Good | New users | Incomplete tutorial table (missing actual file links) |
| **prepare-your-data.md** | ✅ Good | Data engineers | Uses DatasetVariable pattern well |
| **train-and-register-a-model.md** | ⚠️ Needs revision | ML engineers | Duplicates data loading info from prepare-your-data.md |
| **model-registry-guide.md** | ✅ Good | ML engineers | Comprehensive; could link better to training guide |
| **cli.md** | ✅ Good | Operators | Reference complete; lacks integration with user journey |
| **set-up-triggers.md** | ⚠️ Needs revision | ML engineers | Assumes pipeline knowledge; needs better introduction |
| **project-management-for-ml-pipelines.md** | 🔴 Not reviewed | Operators | **Action needed**: Review and assess |

#### Level 2: ML Pipelines Sub-Guides (docs/user-guides/ml-pipelines/)
| File | Status | Purpose | Key Issue |
|------|--------|---------|-----------|
| **index.md** | ✅ Excellent | Framework overview | Clear concepts; great landing page |
| **getting-started.md** | ⚠️ Review needed | First pipeline tutorial | May need alignment with broader user journey |
| **pipeline-management.md** | ✅ Good | Concept explanation | Clear standard vs custom comparison |
| **pipeline-running-modes.md** | ✅ Good | Execution modes | Well-explained; linked properly |
| **running-uniflow.md** | ✅ Good | Technical setup | Environment config is clear |
| **cache-and-pipelinerun-resume-form.md** | ✅ Good | Advanced feature | Well-integrated into index |
| **file-sync-testing-flow-runbook.md** | ✅ Good | Developer workflow | Practical and specific |

---

## Critical Issues

### 1. **BLOCKING: Missing Model Deployment Guide**

**Status**: Marked "Coming Soon" in 3 places
**Impact**: Complete ML workflow is incomplete (prepare → train → register → **deploy missing**)
**Affected Files**:
- docs/user-guides/index.md (line 28)
- docs/user-guides/train-and-register-a-model.md (line 195)
- docs/user-guides/ml-pipelines/index.md (implied in workflow)

**Action Required**:
- [ ] Create docs/user-guides/model-deployment.md covering:
  - Triton Inference Server integration
  - Batch vs real-time serving
  - Scaling and monitoring
  - Example deployment code
- [ ] Link from index.md and training guide
- [ ] Coordinate with engineer for technical validation

---

### 2. **Terminology Inconsistency**

**Problem**: Core concepts used interchangeably and inconsistently

| Concept | Current Usage | Should Be |
|---------|---------------|-----------|
| **Workflow** | Python function with @workflow decorator | ✅ Correct in ml-pipelines/index.md |
| **Pipeline** | Deployable instance OR the overall system | ⚠️ Ambiguous across docs |
| **Task** | Discrete unit of work with @task decorator | ✅ Consistent |
| **Uniflow** | Framework name | ✅ Used correctly |

**Action Required**:
- [ ] Create TERMINOLOGY.md glossary document
- [ ] Apply consistently across all docs
- [ ] Flag all ambiguous uses in train-and-register-a-model.md and set-up-triggers.md

---

### 3. **Fragmented User Journey**

**Problem**: No clear path from "I'm new" to "I deployed my first model"

**Current Issue**:
- docs/user-guides/index.md mentions "Get Started" but doesn't actually direct to a unified path
- ML Pipelines documentation is separate but deeply needed for all workflows
- prepare-your-data.md and train-and-register-a-model.md both exist at same level but aren't clearly sequenced

**Action Required**:
- [ ] Revise docs/user-guides/index.md to include:
  - Clear "New User Path": data prep → training → registry → deployment
  - Explicit links to ML Pipelines section with explanation
  - Position trigger setup as advanced (not in main path)
- [ ] Create quick reference showing "which guide do I read first"

---

### 4. **Duplicate Content & Unclear Scope**

**Issue**: Data loading covered in both:
1. docs/user-guides/prepare-your-data.md (section: "DatasetVariable")
2. docs/user-guides/train-and-register-a-model.md (section: "Understanding Training Inputs")

**Action Required**:
- [ ] Decide single source of truth for DatasetVariable documentation
- [ ] Remove duplicate, link to primary
- [ ] Ensure consistency in API examples

---

### 5. **Cross-Reference Problems**

**Issue**: Multiple external GitHub wiki links instead of local paths

**Examples** (docs/user-guides/index.md):
- Line 25: `https://github.com/michelangelo-ai/michelangelo/wiki/Prepare-Your-Data`
- Line 26: `https://github.com/michelangelo-ai/michelangelo/wiki/Train-and-Register-a-Model`
- Line 27: `https://github.com/michelangelo-ai/michelangelo/wiki/Model-Registry-Guide`

**Action Required**:
- [ ] Replace with relative links: `./prepare-your-data.md`
- [ ] Verify all internal links resolve correctly
- [ ] Update next-steps links in docs

---

### 6. **Architecture Documentation Gap**

**Problem**: Core infrastructure mentioned but not explained

**Mentioned but unexplained**:
- Cadence/Temporal worker execution model
- Kubernetes integration
- Starlark compilation
- S3-compatible storage checkpoint system

**Action Required**:
- [ ] Create docs/user-guides/architecture-overview.md covering:
  - Separation of workflow vs task execution
  - Checkpoint and resume mechanics
  - Infrastructure requirements
  - For whom: should be optional advanced reading

---

## Inconsistencies & Improvements by Category

### Code Examples
**Issue**: Inconsistent import patterns

**Current variations**:
```python
# Pattern 1:
import michelangelo.uniflow.core as uniflow

# Pattern 2:
from michelangelo.uniflow.core import task, workflow

# Pattern 3 (trainer SDK):
from michelangelo.sdk.trainer.torch.pytorch_lightning.lightning_trainer import LightningTrainer
```

**Action**: Standardize on Pattern 1 throughout; document in style guide

---

### API Version References
**Issue**: Trigger.yaml uses `michelangelo.api/v2` but no context on what versions exist

**Action**: Document API versioning strategy

---

### Example Integration
**Issue**: Top-level index.md lists examples but doesn't link to them

**Examples mentioned**:
- Boston Housing (XGBoost)
- BERT Text Classification
- GPT Fine-tuning
- Amazon Books Recommendation

**Action Required**:
- [ ] Link to python/examples/ or create index
- [ ] Add "Run This Example" section to index.md
- [ ] Ensure examples are up-to-date with current docs

---

## Missing Topics for Open Source

### 1. **Getting Help & Community**
- Where to file issues
- Where to ask questions (Discord, GitHub discussions, etc.)
- How to contribute documentation improvements

### 2. **Upgrade & Compatibility Guide**
- Breaking changes between versions
- Upgrade path
- Deprecation notices

### 3. **Performance & Scaling Guide**
- When to use each execution mode (Local/Remote/Dev/Pipeline)
- Scaling guidelines for different data sizes
- Cost estimation

---

## Implementation Roadmap

### Phase 1: Critical Path (Weeks 1-2)
**Goal**: Fix blocking issues and establish consistency

- [ ] **Task 1**: Create Model Deployment guide (coordinate with engineer)
- [ ] **Task 2**: Create and apply Terminology glossary
- [ ] **Task 3**: Fix all cross-references (GitHub wiki → local)
- [ ] **Task 4**: Revise index.md with clear user journey

### Phase 2: Structural Improvements (Weeks 2-3)
**Goal**: Reduce duplication and improve navigation

- [ ] **Task 5**: Consolidate DatasetVariable documentation
- [ ] **Task 6**: Review set-up-triggers.md for context and prerequisites
- [ ] **Task 7**: Review project-management-for-ml-pipelines.md
- [ ] **Task 8**: Create Architecture Overview guide

### Phase 3: Polish & Open Source Readiness (Week 3+)
**Goal**: Ensure docs serve open source community

- [ ] **Task 9**: Add community/contribution section to index
- [ ] **Task 10**: Create quick reference card (user paths)
- [ ] **Task 11**: Enhance troubleshooting sections
- [ ] **Task 12**: Link and test all examples

---

## Success Metrics

- [ ] All "Coming Soon" sections replaced with working guides
- [ ] 100% internal link consistency (no external GitHub wiki links)
- [ ] Clear user journey: new user can complete end-to-end workflow
- [ ] No duplicate content
- [ ] Terminology glossary created and consistently applied
- [ ] Open source onboarding experience validated

---

## Team Coordination

### Required Reviews
1. **Engineer** (teammate): Technical accuracy of deployment guide, API examples, architecture
2. **Tech Writer** (teammate): Style consistency, grammar, structure improvements
3. **Project Lead**: Alignment with open source roadmap and community goals

### Unblocked Parallel Work
- Engineer can start reviewing existing docs and validating code examples
- Tech Writer can begin refactoring index.md and user journey
- Project Manager (this role) creates style guide and coordinates

---

## Next Steps

1. Communicate this plan to the team (tech-writer and engineer)
2. Prioritize Phase 1 tasks in order of user impact
3. Assign tasks to team members
4. Create documentation style guide (terminology, code examples, link format)
5. Establish review process for PR merge
