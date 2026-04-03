# Operator & Contributor Guide Improvements

This document proposes improvements to the operator and contributor guides based on a review of current content and comparison with similar open-source ML platforms (Flyte, Ray/KubeRay).

## Background

**Operator guide** targets adopters integrating Michelangelo with external ML systems (experiment tracking servers, Kubernetes clusters, schedulers, etc.).
**Contributor guide** targets developers contributing features to the codebase.

The analysis below identifies structural gaps and specific missing documents, with priority order and rationale.

---

## Current State

### Operator Guide — what exists

| Document | Coverage |
|---|---|
| `operator-guides/index.md` | Component ConfigMap reference (API server, controller manager, worker, UI/Envoy) |
| `operator-guides/jobs/index.md` | Jobs overview — Ray vs Spark, lifecycle, observability |
| `operator-guides/jobs/register-a-compute-cluster-to-michelangelo-control-plane.md` | Step-by-step cluster registration |
| `operator-guides/jobs/run-uniflow-pipeline-on-compute-cluster.md` | Running pipelines on a compute cluster |
| `operator-guides/jobs/extend-michelangelo-batch-job-scheduler-system.md` | Custom scheduler backends (Kueue, Volcano), custom assignment strategies |
| `operator-guides/serving/index.md` | Serving architecture, InferenceServer + Deployment lifecycle |
| `operator-guides/serving/cluster-setup.md` | Serving cluster setup for local sandbox |
| `operator-guides/serving/integrate-custom-backend.md` | Plugin interfaces: Backend, ModelConfigProvider, RouteProvider |
| `operator-guides/ui/` | UI deployment, local dev, React library |
| `operator-guides/ingester-design.md` | Ingester architecture |
| `operator-guides/ingester-sandbox-validation.md` | Ingester sandbox validation |
| `operator-guides/compliance.md` | SOC 2, GDPR, HIPAA configuration |
| `operator-guides/api-framework.md` | High-level API architecture overview |

**Structural issue:** `operator-guides/index.md` is the full platform setup reference, not a navigation hub. Operators have no entry point that explains where to start or which guide applies to their situation.

### Contributor Guide — what exists

| Document | Coverage |
|---|---|
| `contributing/building-michelangelo-ai-from-source.md` | Build commands for Go components and Python tooling |
| `contributing/how-to-write-apis.md` | Proto definitions, Gazelle, gRPC code generation |
| `contributing/manage-go-dependencies.md` | `go mod tidy`, `bazel mod tidy` |
| `contributing/use-go-mocks-in-unit-test.md` | gomock usage |
| `contributing/uniflow-plugin-guide.md` | End-to-end plugin development (Go worker → Starlark → Python TaskConfig) |
| `contributing/documentation-guide.md` | Documentation conventions |
| `contributing/TERMINOLOGY.md` | Glossary of core concepts |
| `contributing/dev/go/error-handling.md` | Go error handling patterns and PR checklist |
| `contributing/dev/python/mactl/coding_guidelines.md` | Python coding guidelines |
| `contributing/dev/ui/` | UI patterns, components, configuration |

**Structural issue:** No contributing overview or `CONTRIBUTING.md` equivalent. External contributors have no entry point explaining types of contributions, component ownership, or how to submit a PR.

---

## Reference: Flyte and Ray

**Flyte** organizes its operator guide into: Deployment Paths, Plugin Setup, Agent Setup, Cluster Configuration, Configuration Reference, Security Overview. Contributor guide uses a learning-first approach: run examples, understand architecture, then contribute.

**Ray/KubeRay** operator guide covers multiple deployment targets (Kubernetes, cloud VMs), custom resources (RayCluster, RayJob, RayService), ecosystem integrations, troubleshooting, and benchmarks. Contributor guide has 13 sections including: PR process, code style, CI explanation, API compatibility guide, and a committer pathway.

Key patterns worth adopting:
- Ray's troubleshooting guide is one of its most-visited operator pages
- Ray's CI transparency (what jobs run, how to interpret failures) meaningfully reduces contributor friction
- Flyte's plugin/agent setup section directly maps to Michelangelo's serving backend and job scheduler extension points — but those are currently buried rather than surfaced as first-class integration surfaces

---

## Proposed Changes

### Operator Guide

#### 1. Restructure the index (no new content needed)

Rename `operator-guides/index.md` → `operator-guides/platform-setup.md` and create a new `index.md` as a navigation hub. The hub should describe operator personas (initial deployment, adding compute capacity, integrating ML tooling, ongoing operations) and link to the relevant guide for each.

#### 2. New: External Integrations Index

**File:** `operator-guides/integrations/index.md`

A landing page surfacing Michelangelo's existing extension points as first-class integration surfaces. Links to: custom serving backends (existing), custom job scheduler backends (existing), Kueue/Volcano (existing). Modeled on Flyte's Plugin Setup section — makes these guides discoverable rather than buried sub-pages.

#### 3. New: Monitoring & Observability Guide

**File:** `operator-guides/monitoring.md`

Covers:
- Prometheus scrape targets for each Michelangelo component
- Key operational metrics: job queue depth, scheduler assignment latency, inference request P99, workflow engine lag
- Grafana dashboard setup
- Alert recommendations for production deployments

Ray has dedicated observability documentation and it's among the most referenced pages for operators validating SLOs.

#### 4. New: Authentication & Identity Provider Setup

**File:** `operator-guides/authentication.md`

The compliance guide references RBAC and OIDC but there is no setup guide. Covers:
- OIDC provider configuration (connecting an enterprise IdP)
- Service account token flows for compute clusters
- Multi-tenant namespace isolation
- Session token expiry configuration

#### 5. New: Troubleshooting Guide

**File:** `operator-guides/troubleshooting.md`

Common failure modes and diagnostic steps for:
- Jobs: cluster registration failures, scheduler not assigning, Temporal/Cadence connectivity issues, Ray pod crashes
- Serving: model not loading, HTTPRoute not created, health check failures
- Worker: API server connectivity, workflow engine connection
- Shared: storage (S3/MinIO) permission errors, kubeconfig context issues

Ray's troubleshooting guide is one of its most-visited operator pages. A single reference with `kubectl` diagnostic commands and log patterns significantly reduces support burden.

#### 6. New: Network & Ingress Configuration

**File:** `operator-guides/network.md`

The current platform setup doc has a table of "domain settings to update" but no guide. Covers:
- Envoy proxy configuration (CORS, cluster hostnames)
- Ingress setup for API server and UI
- cert-manager TLS configuration
- Multi-cluster network topology (control plane → compute cluster connectivity requirements)

#### 7. New: External Integrations Index

**File:** `operator-guides/integrations/index.md`

A landing page for all integration guides, modeled on Flyte's Plugin Setup section. Links to: custom serving backends (existing), custom job scheduler backends (existing), Kueue/Volcano (existing). Makes the existing extension point docs discoverable as first-class integration surfaces rather than buried sub-pages.

---

### Contributor Guide

#### 1. New: Contributing Overview

**File:** `contributing/index.md` (or top-level `CONTRIBUTING.md`)

The single most important missing piece. Modeled on Ray's contributor guide entry point:
- Types of contributions (new plugins, API extensions, bug fixes, docs)
- Component map: which directory owns which subsystem
- How to find work (issue labels, good first issues)
- Quick-start: fork → build → test → PR in ~5 steps
- Links to all detailed guides below

Without this, external contributors have no entry point.

#### 2. New: PR & Review Process

**File:** `contributing/pr-process.md`

Covers:
- Branch naming conventions
- PR description template
- What CI jobs run and what must pass before merge
- Review expectations and SLAs
- How to handle review feedback
- Merge criteria

#### 3. New: Testing Strategy

**File:** `contributing/testing.md`

The mocks guide is narrow. A broader guide covers:
- Test pyramid: unit tests (same package, `_test.go`), integration tests (sandbox), E2E
- How to run unit tests locally (`bazel test //go/...`)
- Integration test setup using the sandbox (`ma sandbox create`)
- Test coverage expectations per layer
- When to use mocks vs real dependencies (links to existing mocks guide)

#### 4. New: CI Pipeline Guide

**File:** `contributing/ci.md`

Covers:
- Which jobs run on PRs (linting, unit tests, build checks)
- How to interpret CI failures
- How to re-trigger or skip jobs where appropriate
- Bazel remote caching behavior

Ray's CI transparency is a significant contributor trust-builder. Contributors who can read CI output without needing help unblock themselves.

#### 5. Expand: Building from Source — add architecture section

**File:** `contributing/building-michelangelo-ai-from-source.md` (expand existing)

Add an "Architecture for contributors" section at the top: a diagram or table mapping Go packages to subsystems. The uniflow plugin guide does this well for the plugin layer. Contributors working on the control plane (apiserver, controllermgr, worker) need the equivalent.

#### 6. New: Go Code Style Guide

**File:** `contributing/dev/go/code-style.md`

`error-handling.md` exists and is good. Expand to a broader guide:
- Package naming and structure conventions
- Interface design patterns (modeled on the `Backend`/`ModelConfigProvider`/`RouteProvider` patterns already in the codebase)
- Logging conventions (zap field names, log levels)
- Test file organization and naming

---

## Priority Order

| Priority | Item | Rationale |
|---|---|---|
| 1 | Contributing overview / `CONTRIBUTING.md` | Unblocks external contributors entirely |
| 2 | Operator guide index restructure | Navigation foundation everything else builds on |
| 3 | PR process + testing strategy | Most common friction points for first contributors |
| 4 | Troubleshooting guide | High operator value, fast to write from existing knowledge |
| 5 | Auth/OIDC guide | Required for any production deployment |
| 6 | Monitoring & observability | Required for production SLOs |
| 7 | Network/ingress guide | Unblocks non-Uber deployments |
| 8 | CI pipeline guide | Contributor quality-of-life |
| 9 | Go code style guide | Polish, reduces review comments |
