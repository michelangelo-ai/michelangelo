# Building Michelangelo from Source

To contribute to the Michelangelo repository, follow the instructions below to build from the main branch.

## Prerequisites

Ensure you have the following installed before building:

- **[Bazel](https://bazel.build/install)** — the project uses Bazel `7.4.1` (see `.bazelversion`)
- **[Go](https://go.dev/doc/install)** — version `1.24.0+` (see `go/go.mod`)
- **[Python](https://www.python.org/downloads/)** — version `3.9+`
- **[Poetry](https://python-poetry.org/docs/#installation)** — for Python dependency management

For the full sandbox environment (Docker, kubectl, k3d, GitHub token), see the [Sandbox Setup Guide](../getting-started/sandbox-setup.md).

### macOS: Set C++ Compiler for Bazel

If Bazel fails with C++ build errors on macOS, add the following to your `.zshrc`:

```bash
export CC=clang
export CXX=clang++
```

## Go Components

The Go services live under `go/cmd/` and are built with Bazel.

### API Server

The unified gRPC server for all Michelangelo APIs. It provides CRUD operations for API resource types, manages resource schemas, and invokes registered API hooks.

```bash
bazel run //go/cmd/apiserver
```

### Worker

Hosts Cadence and Temporal workflow/activity workers for various platform tasks.

```bash
bazel run //go/cmd/worker
```

To run the worker against a sandbox (without the worker component):

```bash
# Start sandbox without the worker
cd $REPO_ROOT/python
poetry run ma sandbox create --exclude worker

# Then run the worker locally
bazel run //go/cmd/worker
```

### Controller Manager

The Kubernetes controller manager. Requires a Kubernetes config connected to a Michelangelo cluster (or a local sandbox).

```bash
# Create a sandbox cluster first
cd $REPO_ROOT/python
poetry run ma sandbox create

# Start the controller manager
bazel run //go/cmd/controllermgr
```

To build and run in a container:

```bash
# Build the container image
bazel build //go/cmd/controllermgr:image.tar --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64

# Load into Docker
docker load -i $WORKSPACE_ROOT/bazel-bin/go/cmd/controllermgr/image.tar

# Run
docker run --rm --network=host \
  -e CONFIG_DIR=./go/cmd/controllermgr/config \
  -v $HOME/.kube:/root/.kube \
  bazel/go/cmd/controllermgr:image
```

### Managing Go Dependencies

See the full guide in [Managing Go Dependencies](manage-go-dependencies.md). The short version:

```bash
# After adding/removing imports in .go files
cd $REPO_ROOT/go
go mod tidy

# If go.mod changed, update Bazel module from the repo root
bazel mod tidy
```

## Python Components

Python packages and CLI tools are managed with Poetry under the `python/` directory.

### Setup

```bash
cd $REPO_ROOT/python
poetry install -E dev
```

### Linting and Formatting

```bash
cd $REPO_ROOT/python

# Pre-commit checks
poetry run pre-commit

# Lint
poetry run ruff check $FILE

# Format
poetry run ruff format $FILE
```

### Running the Sandbox

```bash
cd $REPO_ROOT/python
poetry run ma sandbox create
```

For more detail, see the [Sandbox Setup Guide](../getting-started/sandbox-setup.md).

## IDE Setup

For IDE configuration (VS Code, Cursor, GoLand), see the [IDE and Bazel Setup Guide](../getting-started/setup-ide-and-bazel.md).
