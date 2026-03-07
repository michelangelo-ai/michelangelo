# Michelangelo Documentation Improvement Plan - FINAL
## Synthesized from PM Audit, Engineer Technical Validation, and Tech-Writer Quality Review

**Status**: FINAL (March 7, 2026)
**Prepared By**: Project Manager (synthesizing all team findings)
**Validated By**: Engineer technical analysis + Tech-writer quality assessment

---

## Executive Summary

This plan synthesizes three independent assessments:
1. **PM Structural Audit** - Information architecture and consistency
2. **Engineer Technical Validation** - Code accuracy and codebase alignment
3. **Tech-Writer Quality Review** - ML Pipelines documentation clarity

**Key Finding**: All three assessments identified **terminology inconsistencies as blocking** (especially RayTask vs Ray). Engineer confirmed this is a **P0 blocking issue** affecting all code examples.

**Priority Shift**: RayTask import error must be fixed before broader documentation work proceeds.

---

## Critical Blocking Issues (P0 - Fix First)

### 1. RayTask Import Error - BLOCKS ALL EXAMPLES ⚠️

**Status**: CRITICAL DEFECT
**Impact**: All code examples fail when users copy-paste
**Location**: Across 9+ documentation files
**Issue**:
- **Docs show**: `from uniflow.plugins.ray import Ray`
- **Code actually has**: `from uniflow.plugins.ray import RayTask`
- **Result**: Every user trying the examples gets ImportError

**Evidence**:
- Engineer verified against codebase (Task #8)
- Tech-writer identified RayTask/Ray confusion (Task #7)
- PM identified terminology inconsistency (Task #6)

**Fix Required** (Task #14):
- Replace `Ray` with `RayTask` across all docs
- Update TERMINOLOGY.md with correct import
- Verify all code examples execute

**Files Affected**:
- train-and-register-a-model.md
- ml-pipelines/index.md
- ml-pipelines/getting-started.md
- ml-pipelines/pipeline-management.md
- ml-pipelines/running-uniflow.md
- Any examples using Ray clusters

---

### 2. Reference System Undocumented - BLOCKS UNDERSTANDING ⚠️

**Status**: CRITICAL GAP
**Impact**: Users don't understand how task data flows between tasks
**Issue**:
- Task data is passed using a "reference system" (verified by engineer)
- Users have no documentation on how this works
- Critical for understanding task composition

**Fix Required** (Task #15):
- Document the reference system architecture
- Explain how task outputs become inputs to next tasks
- Provide working examples with DatasetVariable
- Link from ml-pipelines/index.md

---

### 3. Type System & Codecs Undocumented - BLOCKS ADVANCED USE ⚠️

**Status**: CRITICAL GAP
**Impact**: 5 supported data serialization codecs completely unknown to users
**Issue**:
- Engineer verified: 5 codec types available
- No user documentation on:
  - What codecs exist
  - When to use each
  - How to configure

**Fix Required** (Task #16):
- Document each codec type
- Explain use cases and performance characteristics
- Provide configuration examples
- Link from architecture documentation

---

## High-Priority Issues (P1 - Fix in Week 1)

### 4. Model Deployment Guide Missing

**Status**: FEATURE EXISTS (confirmed by engineer)
**Impact**: Incomplete ML workflow documentation
**Current State**: Marked "Coming Soon" in 3 places
**Evidence**: Engineer verified full implementation in Go backend

**Files with "Coming Soon"**:
- docs/user-guides/index.md (line 28)
- docs/user-guides/train-and-register-a-model.md (line 195)
- docs/user-guides/ml-pipelines/index.md

**Fix Required** (Task #13):
- Create docs/user-guides/model-deployment.md
- Document deployment to Triton Inference Server
- Batch vs real-time serving patterns
- Link from all 3 locations above

---

### 5. Cross-Reference Links Broken

**Status**: MEDIUM IMPACT (Navigation issue)
**Impact**: External GitHub wiki links instead of internal docs
**Files Affected**:
- docs/user-guides/index.md (lines 25-27)
- Multiple "Next steps" sections

**Fix Required** (Task #11):
- Replace external URLs with relative paths
- Add navigation breadcrumbs
- Test all links

---

### 6. User Journey Unclear

**Status**: NEW USER EXPERIENCE ISSUE
**Impact**: No clear path from "I'm new" to "I deployed my model"
**Current State**: index.md mentions guides but doesn't clearly sequence them

**Fix Required** (Task #12):
- Restructure index.md with explicit Getting Started path
- Clear progression: prepare → train → register → deploy
- Separate advanced topics (triggers, project management)
- Explain why users need pipelines and when to use them

---

## Medium-Priority Issues (P2 - Fix in Week 2)

### 7. Duplicate Content - DatasetVariable Documentation

**Status**: MAINTENANCE BURDEN
**Files Affected**:
- prepare-your-data.md (detailed section)
- train-and-register-a-model.md (overlapping section)

**Fix**: Consolidate to single source of truth, link from other

---

### 8. Architecture Concepts Undefined

**Status**: GAP IN ADVANCED DOCUMENTATION
**Concepts Not Explained**:
- Starlark compilation
- Cadence/Temporal execution model
- S3 checkpoint system
- Distributed execution model

**Fix**: Create architecture overview doc

---

### 9. Troubleshooting Scattered

**Status**: POOR DISCOVERABILITY
**Current**: Tips scattered across multiple guides
**Fix**: Create unified troubleshooting guide with index

---

## 4-Phase Implementation Plan

### Phase 1: CRITICAL (Week 1) - P0 Blocking Issues
**Timeline**: 12-14 hours of focused work
**Objective**: Fix blocking defects, unblock all other work

| Task | Owner | Hours | Priority | Dependency |
|------|-------|-------|----------|------------|
| #14: Fix RayTask imports | Tech-writer | 2 | P0-1 | None |
| #15: Document Reference system | Tech-writer + Engineer | 3 | P0-2 | #14 |
| #16: Document Type system | Tech-writer + Engineer | 2 | P0-3 | #14 |
| #10: Verify & Apply Terminology | Tech-writer + Engineer | 2 | P0-4 | #14, #15, #16 |
| #13: Create Deployment Guide | Engineer | 6 | P1-1 | None (parallel) |

**Deliverables**:
- ✓ All code examples executable
- ✓ Reference system documented
- ✓ Type system documented
- ✓ TERMINOLOGY.md verified
- ✓ Deployment guide complete
- ✓ No "Coming Soon" in critical path

---

### Phase 2: HIGH PRIORITY (Week 2) - User Experience & Navigation
**Timeline**: 8-10 hours
**Objective**: Clear navigation, remove broken links, improve user journey

| Task | Owner | Hours | Priority |
|------|-------|-------|----------|
| #11: Fix cross-references | Tech-writer | 2 | P1-1 |
| #12: User journey revision | Tech-writer | 5 | P1-2 |
| #9: Review project management | Engineer | 3 | P1-3 |

**Deliverables**:
- ✓ All links internal, tested
- ✓ Clear user path in index
- ✓ Navigation breadcrumbs
- ✓ Project management docs validated

---

### Phase 3: STRUCTURAL (Weeks 3+) - Deduplication & Architecture
**Timeline**: 6-8 hours
**Objective**: Remove duplication, add advanced docs

| Task | Owner | Hours |
|------|-------|-------|
| Consolidate DatasetVariable docs | Tech-writer | 2 |
| Create architecture overview | Engineer | 3 |
| Create troubleshooting guide | Tech-writer | 2 |
| Add community/contribution section | Tech-writer | 1 |

**Deliverables**:
- ✓ No duplicate content
- ✓ Architecture fully documented
- ✓ Troubleshooting centralized
- ✓ Open source community ready

---

## Success Metrics

### Phase 1 Completion Criteria ✓
- [ ] All code examples are syntactically correct and executable
- [ ] RayTask used consistently everywhere
- [ ] Reference system documented with examples
- [ ] Type system/codecs documented
- [ ] TERMINOLOGY.md verified by engineer
- [ ] Deployment guide complete and linked
- [ ] No "Coming Soon" in critical workflow path

### Phase 2 Completion Criteria
- [ ] All GitHub wiki URLs replaced with local links
- [ ] All internal links tested and working
- [ ] Clear user journey in index (prepare → train → register → deploy)
- [ ] Navigation breadcrumbs present
- [ ] Project management docs validated

### Phase 3 Completion Criteria
- [ ] No duplicate content
- [ ] Architecture fully explained
- [ ] Troubleshooting guide complete
- [ ] Open source community section added

---

## Risk Mitigation

### Risk 1: RayTask Error Blocks Everything
**Mitigation**: Make Task #14 priority 1 (2-hour fix, unblocks all)

### Risk 2: Terminology Updates Miss Locations
**Mitigation**: Engineer + Tech-writer pair on TERMINOLOGY.md validation (Task #10)

### Risk 3: Deployment Guide Scope Unclear
**Mitigation**: Engineer confirmed feature exists; outline ready in PHASE1_EXECUTION_CHECKLIST.md

### Risk 4: Users Still Confused After Fixes
**Mitigation**: Add clear "Getting Started" path in index (Task #12)

---

## Team Coordination

### Phase 1 Sequencing (Avoid Rework)
1. **Start**: Task #14 (RayTask fixes) - unblocks everything
2. **Parallel**: Task #13 (Deployment guide) - no dependencies
3. **Then**: Tasks #15, #16 (Reference + Type system) - use corrected terminology
4. **Finally**: Task #10 (Terminology validation) - verify consistency

### Communication Points
- Weekly sync on Phase 1 progress
- Daily blockers escalation
- Engineer validates terminology during Task #10

---

## Alignment with Open Source Goals

✅ **Complete ML Workflow** - All steps now documented (prepare → train → register → deploy)
✅ **Accurate Examples** - All code fixes prevent user frustration
✅ **Clear Onboarding** - New users have explicit path to follow
✅ **Community Ready** - Architecture and advanced docs support power users

---

## Conclusion

This plan synthesizes three independent assessments which all converged on the same critical issues:
1. **Terminology inconsistency** (especially RayTask)
2. **Missing technical documentation** (Reference system, Type system)
3. **Broken user journey** (no clear path for new users)

The engineer's validation confirmed these issues are blocking and must be fixed first. Phase 1 focuses on these P0 items (12-14 hours), enabling Phase 2 and beyond.

**Status**: Ready for execution. All blockers identified. Clear sequencing established. Team aligned.

---

**Prepared**: March 7, 2026
**Plan Confidence**: HIGH (based on 3-angle validation + engineer code confirmation)
**Ready to Execute**: YES
