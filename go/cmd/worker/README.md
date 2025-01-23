Worker is a Cadence [workflow worker](https://cadenceworkflow.io/docs/concepts/topology#workflow-worker)
and [activity worker](https://cadenceworkflow.io/docs/concepts/topology#activity-worker).
It hosts a set of Cadence workflows and activities required for different tasks within the Michelangelo platform.

## Developer Guide

This section is intended for contributors. Below are the instructions for setting up the development environment and
running test workflows.

1. **Run Sandbox**:
   Run the Sandbox without the worker component (you will run the worker separately in the next step).
    ```sh
    sandbox create --exclude worker
    ```
   See the [sandbox README]() for more details.

2. **Run the Worker**:
   Run the worker with the following command:
    ```sh
    bazel run //go/cmd/worker
    ```

3. **Run Workflows**:
   Now, as the worker is running, you can run test workflows.
   TODO: andrii: Add instructions on how to run workflows.
