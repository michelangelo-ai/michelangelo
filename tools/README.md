# Tools Directory

This directory contains utility scripts for Michelangelo development and testing.

## Directory Structure

```
tools/
├── README.md (this file)
├── test/           # Scripts for local testing only
└── *               # Scripts for development purposes
```

## tools/ (Development Scripts)

Scripts in the root `tools/` directory are for **development purposes** and may be used in CI/CD pipelines or by developers during regular development workflows.

### Development Scripts:

- **`bazel` / `bazelisk.py`** - Bazel build tool wrapper
- **`gazelle`** - Go BUILD file generator
- **`go`** - Go toolchain wrapper
- **`goimports`** - Go import formatter
- **`mamockgen`** - Mock generator for Michelangelo
- **`grpc-svc-gen.sh`** - gRPC service generator
- **`gen-grpc-client.sh`** - gRPC client generator
- **`run_ruff.sh`** - Python linter runner
- **`assert_python_version.py`** - Python version checker
- **`utils.py`** - Common utilities

These scripts are:
- Part of the standard development workflow
- May be referenced in CI/CD configurations
- Should be committed to the repository
- Used by all developers

## tools/test/ (Local Testing Only)

Scripts in `tools/test/` are for **local testing purposes only** and should **NOT** be used in production or committed as part of production code.

### Testing Scripts:

- **`generate-certs.sh`** - Generate TLS certificates for local webhook testing
  - Generates self-signed CA and TLS certificates
  - Used for testing CRD conversion webhooks locally
  - Certificates are for development/testing only, NOT for production

These scripts are:
- For local development/testing only
- Not intended for CI/CD or production use
- May reference local paths or make assumptions about local environment
- Should be documented with clear "testing only" warnings

