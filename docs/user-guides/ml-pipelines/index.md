# Uniflow User Guide

## 1. Introduction

**Uniflow** is a human-friendly Python framework designed to build and run resilient Data and Machine Learning (ML) workflows.

**Key Principles of Uniflow:**

- **Python-First**: Uniflow allows you to use standard Python functions and syntax to define all aspects of your workflows, eliminating the need for esoteric Domain-Specific Languages (DSLs). You can create arbitrarily complex dynamic workflows, including looping, conditional branching, and nesting.

- **Rapid Development**: Developers can run workflows locally in a devpod as easily as any standard Python script. The same code can then be run at scale in a production environment.

# 2. Key Terms

## Pipeline

A pipeline is an instance of a workflow. A pipeline runs the workflow function with a set of predefined configs. This means a workflow can be associated with multiple pipelines, each one with its own set of configs. In Canvas, a pipeline can be triggered in the following ways:

- Triggered directly through MA Studio UI (remote run only)
- Triggered via MA CLI tools (remote run only)
- Triggered with poetry run (local run only)
- Triggered automatically through orchestration (remote run only)

## Task

A task is a discrete unit of work in a workflow, defined as a Python function annotated with the @task decorator. Like regular Python functions, task functions can:

- Take input, perform work, and return output.
- Cache their output and reuse it across invocations.
- Provide retries and timeout hooks to handle failures.

Moreover, task functions can:

- Cache their output and reuse it across invocations.
- Provide retries and timeout hooks to handle failures.

Consider these sample tasks:

```python
@uniflow.task
def load_dataset(dataset_id):
    return ...


@uniflow.task
def train(train_dataset, valid_dataset, train_params):
    return ...


@uniflow.task
def evaluate(model, dataset):
    return ...
```

Tasks run within containers as discrete units of work. They either succeed or fail as a whole, serving as checkpoints. Reasonable granularity is essential when defining a task. The finer the granularity of the tasks, the more control they offer in inspecting results and resuming failed runs. However, creating overly granular tasks can increase IO overhead due to checkpointing.

**Intra-Task Checkpointing**

A task can be designed to resume execution from the point of failure rather than starting from the beginning. This capability, often known as "intra-task" checkpointing, is not within the scope of the Uniflow framework, as its implementation varies depending on the task. For instance, tasks involving Ray Train code should use [Ray Checkpoints](https://docs.ray.io/en/latest/train/user-guides/checkpoints.html), while regular Python tasks could utilize [Joblib Memory Cache](https://joblib.readthedocs.io/en/stable/auto_examples/nested_parallel_memory.html#sphx-glr-auto-examples-nested-parallel-memory-py).

## Workflow

A workflow consists of a series of task calls organized in any order that makes sense for the use case.

```python
@uniflow.workflow
def train_workflow(dataset_id, train_params):
    train_data, valid_data, test_data = load_dataset(dataset_id)
    model = train(train_data, valid_data, train_params)
    metrics = evaluate(model, test_data)
    ...
```

Workflows, similar to tasks, are defined as Python functions, but it's important to distinguish between task code and workflow code. Task code executes in a container using Kubernetes or other container orchestrators, allowing tasks to run any Python code. In contrast, the workflow code is executed in the Cadence Worker and does not operate within a container. To ensure seamless execution in the Cadence Worker, Uniflow imposes several constraints on the workflow code:

- Function Calling: The workflow can call task functions, other workflow functions, and built-in functions. However, calling other functions, such as Python's standard modules, are prohibited.
- Starlark Compatibility: Workflow code must be Starlark compatible.

Despite the mentioned limitations, workflow code supports a fair amount of standard Python features that allow the authoring of highly dynamic workflows. Features included:

- Conditional Branching: Use a standard "if ... else ..." expression to decide which Task to run next based on the result of the previous task execution or workflow input arguments.
- Loops: Use a standard "for" expression for looping.
- Standard Types: Define new values via literals, e.g., {}, [], True, etc., and use them as task inputs.
- Utility Functions: Call standard utility functions, such as min, max, len, zip, etc.

**Workflow functions can:**

- Take inputs, perform work, and return outputs.
- Call task functions, other workflow functions, and built-in functions.
- Support standard Python features for dynamic workflows, such as:
  - **Conditional Branching**: Using standard if ... else ... expressions.
  - **Loops**: Using standard for expressions.
  - **Standard Types**: Defining and using values via literals (e.g., {}, [], True) as task inputs.
  - **Utility Functions**: Calling standard utility functions like min, max, len, zip.
  - **Nesting**: Common workflow logic can be factored out into reusable workflow functions.

**Important Note**: Unlike task code, workflow code executes directly in the Cadence Worker and **does not operate within a container**. This imposes certain constraints, such as prohibiting calls to standard Python modules or third-party libraries (unless covered by built-ins) and requiring Starlark compatibility.

## Built-in

Built-in is a function implemented in Go and integrated into the Cadence Worker. Like tasks, built-ins can be invoked from within workflows. However, unlike tasks, built-ins execute directly within the Cadence Worker without allocating separate resources for container execution, as tasks do. This feature makes built-ins ideal for handling lightweight transactional operations, while tasks are more suitable for resource-intensive workloads.

As noted in the Workflow section above, workflows are unable to call any standard or third-party Python modules. Built-ins cover this gap by implementing the required functionality using Cadence Go SDK. For example, here is how built-ins can cover the functionality of essential Python modules:

- Python: [time.sleep](https://docs.python.org/3/library/time.html#time.sleep); Go: [workflow.Sleep](https://pkg.go.dev/go.uber.org/cadence@v1.2.9/workflow#Sleep).
- Python: [time.time](https://docs.python.org/3/library/time.html#time.time); Go: [workflow.Now](https://pkg.go.dev/go.uber.org/cadence@v1.2.9/workflow#Now).
- Python: [concurrent.futures](https://docs.python.org/3/library/concurrent.futures.html); Go: [workflow.Go](https://pkg.go.dev/go.uber.org/cadence@v1.2.9/workflow#Go).
- Python: [uuid](https://docs.python.org/3/library/uuid.html); Go: [uuid](https://pkg.go.dev/github.com/google/uuid) with [workflow.SideEffect](https://pkg.go.dev/go.uber.org/cadence@v1.2.9/workflow#SideEffect).

Similarly, we can integrate with various Uber services (e.g., Flipr, QueryRunner, Experiments, etc.) by wrapping their Go clients in Cadence Activities and invoking them via [workflow.ExecuteActivity](https://pkg.go.dev/go.uber.org/cadence@v1.2.9/workflow#ExecuteActivity).

Built-ins, along with tasks, form what's known as the Unified Framework. This framework allows developers to seamlessly integrate both resource-intensive processes and lightweight transactional operations into a single, cohesive workflow. Here is what an end-to-end model retraining workflow can look like from the perspective of tasks and built-ins:

*[TODO: Add unified framework workflow diagram]*

## Context

Context is the entry point object for the workflow operations, such as: Running the workflow locally, as detailed below in the Local Run section. Building the workflow's distribution package, which is later to be used to run the workflow in the production environment at scale, as detailed below in the Remote Run and Starlark Package section.

The context contains runtime information, such as workflow function, input arguments, and environment variables, and is usually initialized in the main block. For example:

```python
if __name__ == "__main__":
    ctx = uniflow.create_context()
    ctx.environ.update({
        "LOG_LEVEL": "DEBUG",
        "RAY_AIR_FULL_TRACEBACKS": "1",
    })
    ctx.run(train_workflow, dataset_id="cola", train_params={ ... })
```

## Local Run

As an ML engineer, your primary productivity tool is your workstation, such as a laptop or a development pod. Uniflow prioritizes local development, treating it as a first-class concern.

In Local Run mode, workflows can execute within the local Python interpreter just like any standard Python code. This approach creates a frictionless local development experience, as it doesn't require setting up any additional infrastructure on the developer's machine other than Python.

The same code can be executed both with the local runtime and with the production runtime, enabling a rapid development, deployment, and debug cycle.

## Remote Run

While Local Run mode is for rapid workflow development on smaller datasets, Remote Run mode is for running workflows at scale and in a reproducible manner.

Remote Run mode executes workflow code within the Cadence Worker, running task code in a container using Kubernetes or other container orchestrators. This mode enables reliable and scalable workflow execution against large datasets.

## Starlark Package

To execute a workflow in the Remote Run mode, one first needs to create a distribution package with workflow instructions that the production workflow executor can understand. Uniflow analyzes the user-defined workflow code, compiles it into the appropriate Starlark code, and then packages the results into a compressed tarball file for distribution. This file is known as the Starlark Package.

A Starlark Package is a compressed tarball file that delivers workflow code to the Cadence Workflow Worker for execution. This package includes *.star files containing the workflow logic and a manifest file containing metadata, such as the path to the entry point file and the associated function name.

# 3. Features

Uniflow supports various features to enable robust ML workflows:

## Supported Types

Uniflow task functions can accept arguments and return values of several supported types:

- **Standard Python Types**: int, str, bool, float, dict, list, tuple, @dataclass classes, enum.Enum classes, type references, io.BytesIO.
- **Plugins**: Pandas DataFrame, PySpark DataFrame, Ray Dataset, Proto Message.

## Heterogeneous Workflows

Tasks of different types can be composed into a single workflow. It's possible to pass framework-specific data types between heterogeneous tasks, for example, a Spark task producing a PySpark DataFrame that a downstream Ray task consumes as a Ray Dataset.

## Dynamic Workflows

Uniflow uses standard Python If-Else syntax for conditional logic and For-Each syntax for iterative loops, avoiding esoteric DSLs for dynamic workflows.

## Nested Workflows

You can reuse workflow logic by factoring common logic into reusable workflow functions, following a standard functional programming paradigm.

# 4. Local Run

Local Run mode is prioritized for rapid workflow development on smaller datasets.

- Workflows can execute within the local Python interpreter, just like any standard Python code, without requiring additional infrastructure setup beyond Python itself.
- You can trigger a local run using a bazel run command, incorporating ctx.is_local_run() to override production input parameters.
- Uniflow writes checkpoints to the ~/uf_storage directory when running workflows locally. These checkpointed files can be inspected for debugging.

# 5. Remote Run (Cluster Run)

Remote Run mode is used for running workflows at scale and in a reproducible manner in a production environment.

- This mode executes workflow code within the Cadence Worker and task code in a container using Kubernetes or other container orchestrators.
- **Docker Build**: To ensure reproducibility, it's crucial to create a Docker image for your project.
  - You only need to build a new Docker image when **task code changes**, as task code runs inside the container. Modifying workflow input arguments or workflow code changes **do not necessitate a Docker rebuild**.
- **Trigger Remote Run**: Triggering a Remote Run uses a Bazel target similar to Local Run, but with an additional --remote-run argument. The command line will prompt you for a uBuild Revision ID if the project code has recently changed. example command:

*[Example Coming Soon]*

## 5.1. Applying Local Changes in Remote Run (New Hybrid Flow)

*Only supports Ray task and workflow code now, upcoming Spark task support in Aug*

A new hybrid approach allows development runs to capture code changes without requiring a Docker image rebuild for Cluster Run, by setting an "--apply-local-changes" flag. This is particularly useful for frequent task code changes.

**Note**: You must have a pre-built docker image based on your local branch, also your changed code must be under same branch with your pre-built docker image. 3rd party dependency change is not included in the scope (if you changed import and dependencies, you must rebuild docker image manually).

# 6. Limitations

While Uniflow offers flexibility, there are some limitations:

## 6.1. Opaque Data Types

Custom data types, such as PySpark DataFrames, cannot be directly accessed or manipulated within the workflow code. They can only be passed to downstream tasks and accessed within the task code itself. Standard data types (integers, floats, strings, dictionaries, lists, tuples, dataclasses) can be directly accessed and manipulated within the workflow code without restrictions.

## 6.2. Limited Access to Python Standard Modules and Global Variables

Workflow code is restricted in what it can access. It can only call Uniflow tasks, built-ins, or other workflows. Access to Python's standard modules or third-party libraries, as well as non-local variables (global variables), is generally prohibited. For instance, to get the current time, you should use uniflow.time() built-in function instead of time.time() from Python's standard library.

## 6.3. Starlark Compatibility

Workflow code must be compatible with **Starlark Language**, a simplified Python dialect. Certain Python syntax elements are not supported in Starlark:

- The `is` keyword is not supported; use `==` instead (e.g., `if table_name == None`).
- f-strings are not supported; use format instead.
- Chained comparisons are not supported; use `and` instead (e.g., `if 1 < len(table_name) and len(table_name) < 5`).
- try-except blocks are not supported.

# 7. Task-Level Retry

Uniflow supports task-level retry mechanisms, which are critical for ensuring robustness and operational continuity in large-scale distributed data pipelines. This helps recover from transient errors like network interruptions or temporary resource unavailability.

## 7.1. Configuration

Users can configure retry (and timeout, though timeout is currently deferred for future consideration) in two ways:

- **Uniflow Decorator**: Directly in your Python task definition using the retry parameter (e.g., `@uniflow.task(config=Spark(driver_cpu=4), retry=3)`).
- **CanvasFlex YAML**: In the pipeline_conf.yaml file under task_configs (e.g., `feature_prep: job_specs: retry: 3`).

These retry values are passed to the Starlark code layers (spark_task.star or ray_task.star).

## 7.2. Implementation

The core task-level logic for Spark and Ray tasks is extracted into a function within the .star Starlark code (e.g., spark_task_impl(), ray_task_impl()). A for loop is then used around this function to implement the retry logic.

## 7.3. Retriable Errors

- **submit_job() / create_cluster() (Not Retriable)**: Failures during the initial job submission or cluster creation (e.g., invalid data types in CRD fields) directly terminate the program and are not retried. These functions are expected to succeed most of the time.
- **sensor_job() / run_job() (Retriable)**: Failures detected by sensor_job() for Spark (e.g., Spark job failed status) or run_job() for Ray (e.g., job status "FAILED" or "STOPPED") are considered retriable. Future steps may use more detailed error messages to determine retry suitability.

## 7.4. Retry Strategy: Recreate Cluster (Chosen Approach)

Uniflow tasks run on Spark or Ray clusters. Two main retry strategies were considered:

- **Using the Same Cluster for Retry**: Lightweight and efficient for transient issues, but limited against deeper issues like node crashes or misconfigurations as it reuses the same runtime environment.
- **Recreate Cluster for Retry (Chosen)**: This approach is less efficient due to restart overhead but provides better isolation and a clean execution environment. It is more robust and can solve a broader range of failures, including infrastructure-level issues and environmental misconfigurations. Uniflow prioritizes reliability, accepting a small downgrade in performance.

# 8. Architecture Overview

Uniflow's architecture involves several key components:

- **User Plane**: Developers write workflows using the Python SDK.
- **Python SDK**: Provides tools to define workflows and tasks in Python. It packages workflow code into a Starlark Package (a tarball with .star files) for production.
- **Execution Plan**: Workflow code is compiled into an Execution Plan (Starlark code).
- **Cadence Service**: Runs the user workflow code (@workflow functions).
- **Starlark Worker (Workflow Engine)**: A Cadence Workflow Worker that hosts a generic DSL workflow capable of executing arbitrary Starlark code. It interprets the Starlark code and delegates task execution to the Job Service.
- **Compute Platform**: Runs user @task functions. For Uber internal applications, this connects to Michelangelo's Unified API (Job controllers) using RayJob and SparkJob. For open-source, it's compatible with Kubernetes Job API, KubeRay, and Spark Operator.
- **Container Registry**: Stores Docker containers with user task code. The Compute Platform pulls containers with the task code from here.
- **Storage (HDFS/Terrablob)**: Saves and loads data checkpoints between task runs and stores workflow execution checkpoint raw data. It supports various storage systems via FSSPEC.

The Starlark Worker addresses Cadence SDK limitations by allowing new user-defined workflows to execute without redeployment and enforcing deterministic code execution through the Starlark layer.
