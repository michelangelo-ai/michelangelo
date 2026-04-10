# Contributing to Michelangelo

Michelangelo welcomes contributions from the community. This guide is your entry point — it explains what you can contribute, where each part of the codebase lives, and how to get started.

## Types of Contributions

| Type | Examples |
|------|---------|
| New Uniflow plugins | Add a Flink task type, a new compute backend |
| API extensions | New proto resource, new controller |
| Bug fixes | Scheduler edge cases, controller reconcile bugs |
| Documentation | Guides, examples, terminology |
| UI improvements | New pages, component library additions |
| Testing | Integration tests, test coverage gaps |

For small fixes (typos, obvious bugs), open a PR directly. For new features or significant changes, open a GitHub issue first to discuss the approach before writing code.

## Component Map

Understanding which directory owns which subsystem helps you find the right code quickly.

| Area | Directory | Language |
|------|-----------|----------|
| API definitions (Protobuf) | `proto/` | Proto |
| Generated Go protobuf bindings | `proto-go/` | Go (generated) |
| API server | `go/cmd/apiserver/` | Go |
| Controller manager | `go/cmd/controllermgr/` | Go |
| Workflow worker | `go/cmd/worker/` | Go |
| Uniflow plugins (Go layer) | `go/worker/plugins/` | Go |
| Shared controller components | `go/components/` | Go |
| Python SDK (Uniflow) | `python/michelangelo/uniflow/` | Python |
| CLI (`ma`) | `python/michelangelo/` | Python |
| Web UI | `javascript/` | TypeScript/React |
| Bazel build configuration | `BUILD.bazel`, `MODULE.bazel`, `WORKSPACE.bazel` | Bazel/Starlark |

**API server** (`go/cmd/apiserver/`) is a gRPC server that acts as the control plane API. It validates and stores resources (CRDs via the Kubernetes API), and invokes registered API hooks.

**Controller manager** (`go/cmd/controllermgr/`) runs Kubernetes controllers for each ML resource type (RayCluster, SparkJob, InferenceServer, Deployment, Pipeline, etc.). Each controller reconciles the desired state in etcd with the actual state in compute clusters.

**Worker** (`go/cmd/worker/`, `go/worker/plugins/`) hosts Temporal/Cadence workflow and activity workers. Uniflow plugins extend the worker with domain-specific capabilities (Ray cluster management, Spark job submission, etc.).

**Uniflow SDK** (`python/michelangelo/uniflow/`) is the Python framework users write workflows in. It provides `@uniflow.task()` and `@uniflow.workflow()` decorators. At submission time, Python workflows are transpiled to Starlark and executed by the worker.

## Before You Start

1. **Fork and clone** the repository
2. **Read [TERMINOLOGY.md](TERMINOLOGY.md)** — understand the vocabulary (Task, Workflow, Pipeline, PipelineRun, etc.) before reading code
3. **Build from source** — follow [Building from Source](building-michelangelo-ai-from-source.md) to ensure your environment is working
4. **Set up the sandbox** — `poetry run ma sandbox create` gives you a local Kubernetes cluster with all Michelangelo components running. Most integration tests and manual testing use this.

## Finding Work

- Browse [GitHub Issues](https://github.com/michelangelo-ai/michelangelo/issues) filtered by `good first issue` or `help wanted`
- Issues are tagged by component (e.g., `area/serving`, `area/jobs`, `area/uniflow`) to help you find relevant work
- If you have an idea that isn't tracked yet, open an issue before writing code

## Making a Contribution

1. Create a branch from `main`: `git checkout -b feat/your-feature`
2. Make your changes following the relevant guide below
3. Write tests (see [Testing Strategy](testing.md))
4. Run linters locally before pushing
5. Push and open a PR — see [PR Process](pr-process.md)

## Contribution Guides by Type

### New Uniflow plugin
The most common contribution type. Uniflow plugins add new task execution environments (new compute backends, external services, etc.).

→ **[Uniflow Plugin Guide](uniflow-plugin-guide.md)** — end-to-end walkthrough (Go worker plugin → Starlark orchestration → Python `TaskConfig`)

### New API resource (proto + controller)
Adding a new ML resource type requires proto definitions, a gRPC service, and a Kubernetes controller.

→ **[How to Write APIs](how-to-write-apis.md)** — proto definitions, Gazelle, gRPC code generation

### Go backend changes
For changes to the API server, controller manager, worker, or shared components.

→ **[Error Handling](dev/go/error-handling.md)** — required patterns for controllers and services
→ **[Managing Go Dependencies](manage-go-dependencies.md)** — `go mod tidy` + `bazel mod tidy`
→ **[Using Go Mocks in Tests](use-go-mocks-in-unit-test.md)** — gomock patterns

### Python SDK changes
For changes to the Uniflow decorators, task types, or the `ma` CLI.

→ **[Python Coding Guidelines](dev/python/mactl/coding_guidelines.md)**

### UI changes
For changes to the Michelangelo web UI.

→ **[UI Development](dev/ui/index.md)** — component patterns, types, configuration system

### Documentation
For adding or improving guides, references, or examples.

→ **[Documentation Guide](documentation-guide.md)** — formatting, structure, and style conventions

## Getting Help

- **GitHub Issues** — for bug reports and feature requests
- **GitHub Discussions** — for questions about the codebase, design discussions, and community Q&A
