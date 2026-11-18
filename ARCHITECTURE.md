# Architecture

Michelangelo AI is an end-to-end ML platform that runs on Kubernetes. Clients (the web UI, the CLI, and the Python SDK) call a gRPC API server. The API server stores each resource, such as a pipeline, a pipeline run, or an inference server, as a Kubernetes custom resource. Controllers in the controller manager reconcile those resources. Offline, they run Uniflow pipelines as Ray and Spark jobs on registered compute clusters, with data and artifacts in S3-compatible object storage. Online, they deploy models to inference servers such as Triton.

The full overview, with a system diagram and the list of custom resources and their controllers, is in the documentation:

- In this repository: [docs/getting-started/architecture.md](docs/getting-started/architecture.md)
- On the website: https://michelangelo-ai.org/docs/getting-started/architecture/

## Repository layout

| Directory | Contents |
|-----------|----------|
| `go/` | Backend services: API server, controller manager, and worker (`go/cmd/`), and the controllers and other components (`go/components/`) |
| `proto/` | Protobuf definitions of the API resources and services (`proto/api/v2/`) |
| `proto-go/` | Go code generated from `proto/` |
| `python/` | The Python SDK, Uniflow pipeline framework, and the `ma` CLI (`python/michelangelo/`) |
| `javascript/` | The Michelangelo AI Studio web UI |
| `helm/` | Helm charts: `michelangelo` (control plane) and `michelangelo-llm-gateway` |
| `docker/` | Dockerfiles for the service, Triton serving, and UI images |
| `docs/` | Documentation source |
| `website/` | The Docusaurus site that builds `docs/` into michelangelo-ai.org |
| `scripts/`, `tools/`, `bazel/` | Build, code generation, and release tooling |

To build and run the project locally, start with [Sandbox Setup](docs/getting-started/sandbox-setup.md). To contribute, see [CONTRIBUTING.md](CONTRIBUTING.md).
