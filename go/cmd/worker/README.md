Worker is both a [Cadence workflow worker](https://cadenceworkflow.io/docs/concepts/topology#workflow-worker), [Cadence activity worker](https://cadenceworkflow.io/docs/concepts/topology#activity-worker), and now also supports [Temporal workflows and activities](https://docs.temporal.io/workflows).

It hosts a set of workflows and activities required for various tasks within the Michelangelo platform.

## Developer Guide

This section provides instructions for contributors to set up the development environment and run test workflows.

### 1. Run Sandbox

Run the Sandbox without the worker component (you will start the worker separately in the next step):

```sh
sandbox create --exclude worker
```

Refer to the [sandbox README]() for more details.

### 2. Run the Worker

Run the worker using the following Bazel command, which starts both Cadence and Temporal workflow/activity workers:

```sh
bazel run //go/cmd/worker
```

3. **Run Workflows**:
   Now, as the worker is running, you can run test workflows.
   TODO: andrii: Add instructions on how to run workflows.
