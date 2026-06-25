# RayCluster Failure Scenario Tests

Test scripts for verifying that KubeRay RayCluster failures bubble up to the
Michelangelo control plane RayCluster status (PodErrors, state transitions).

## Prerequisites

1. Sandbox running (`ma sandbox create --workflow cadence`)
2. Controller manager running locally (`bazel run //go/cmd/controllermgr`)
3. `examples:latest` image loaded into k3d:
   ```bash
   cd $REPO_ROOT/python
   docker build -t examples:latest -f ./examples/Dockerfile .
   k3d image import examples:latest -c michelangelo-sandbox
   ```
4. `grpcurl` installed (`brew install grpcurl`)

## Test Scenarios

| # | Script | Failure Type | Expected State | Expected PodError Reason |
|---|--------|-------------|----------------|--------------------------|
| 1 | `01-bad-image.sh` | ImagePullBackOff | FAILED | ImagePullBackOff / ErrImagePull |
| 2 | `02-crash-loop.sh` | CrashLoopBackOff | FAILED | CrashLoopBackOff |
| 3 | `03-oom-killed.sh` | OOMKilled | FAILED | OOMKilled |
| 4 | `04-bad-command.sh` | RunContainerError | FAILED | RunContainerError / CrashLoopBackOff |
| 5 | `05-healthy.sh` | None (golden path) | READY | (none) |

## Usage

```bash
# Run a single test:
./scripts/raycluster-failure-tests/01-bad-image.sh

# Check status (poll until state changes from UNKNOWN):
./scripts/raycluster-failure-tests/get-status.sh <cluster-name>

# Watch KubeRay conditions directly:
./scripts/raycluster-failure-tests/watch-kuberay.sh <cluster-name>

# Clean up a cluster:
./scripts/raycluster-failure-tests/cleanup.sh <cluster-name>

# Clean up all test clusters:
./scripts/raycluster-failure-tests/cleanup-all.sh
```

## What to verify

For each failure test, check:
1. **Michelangelo RayCluster `.status.state`** transitions to `RAY_CLUSTER_STATE_FAILED`
2. **`.status.pod_errors[]`** contains the expected reason and message
3. **`.status.status_conditions[]`** has `Succeeded=False` with the failure reason
4. **KubeRay RayCluster conditions** (via `watch-kuberay.sh`) show the underlying error

For the healthy test (#5), verify state reaches `RAY_CLUSTER_STATE_READY` with no pod errors.
