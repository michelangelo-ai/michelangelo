# Key Concepts and Terms
- [Task](#task)
- [Workflow](#workflow)
- [Built-in](#built-in)
- [Context](#context)
- [Local Run](#local-run)
- [Remote Run](#remote-run)
- [Starlark Package](#starlark-package)


# Task

A **task** is a discrete unit of work in a workflow, defined as a Python function annotated with the `@task` decorator. Like regular Python functions, task functions can:

- **Take input, perform work, and return output.**
- **Cache their output** and reuse it across invocations.
- **Provide retries and timeout hooks** to handle failures.

Moreover, task functions can:

* Cache their output and reuse it across invocations.
* Provide retries and timeout hooks to handle failures.

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

## Intra-Task Checkpointing
A task can be designed to resume execution from the point of failure rather than starting from the beginning. This capability, often known as "intra-task" checkpointing, is not within the scope of the Uniflow framework, as its implementation varies depending on the task. For instance, tasks involving Ray Train code should use[ Ray Checkpoints](https://docs.ray.io/en/latest/train/user-guides/checkpoints.html), while regular Python tasks could utilize[ Joblib Memory Cache](https://joblib.readthedocs.io/en/stable/auto_examples/nested_parallel_memory.html#sphx-glr-auto-examples-nested-parallel-memory-py).

# Workflow
A **workflow** consists of a series of task calls organized in any order that makes sense for the use case.

```
@uniflow.workflow
def train_workflow(dataset_id, train_params):
    train_data, valid_data, test_data = load_dataset(dataset_id)
    model = train(train_data, valid_data, train_params)
    metrics = evaluate(model, test_data)
    ...
```

Workflows, similar to tasks, are defined as Python functions, but it's important to distinguish between task code and workflow code. Task code executes in a container using Kubernetes or other container orchestrators, allowing tasks to run any Python code. In contrast, the workflow code is executed in the Cadence Worker and does not operate within a container. To ensure seamless execution in the Cadence Worker, Uniflow imposes several constraints on the workflow code:
* Function Calling: The workflow can call task functions, other workflow functions, and built-in functions. However, callingother calling other functions, such as Python's standard modules, are prohibited.
* Starlark Compatibility: Workflow code must be Starlark compatible.

Despite the mentioned limitations, workflow code supports a fair amount of standard Python features that allow the authoring of highly dynamic workflows. Features included:
* Conditional Branching: Use a standard "if ... else ..." expression to decide which Task to run next based on the result of the previous task execution or workflow input arguments.
* Loops: Use a standard "for" expression for looping.
* Standard Types: Define new values via literals, e.g., {}, [], True, etc., and use them as task inputs.
* Utility Functions: Call standard utility functions, such as min, max, len, zip, etc.

# Built-in
**Built-in** is a function implemented in Go and integrated into the Cadence Worker. Like tasks, built-ins can be invoked from within workflows. However, unlike tasks, built-ins execute directly within the Cadence Worker without allocating separate resources for container execution, as tasks do. This feature makes built-ins ideal for handling lightweight transactional operations, while tasks are more suitable for resource-intensive workloads.

As noted in the Workflow section above, workflows are unable to call any standard or third-party Python modules. Built-ins cover this gap by implementing the required functionality using Cadence Go SDK. For example, here is how built-ins can cover the functionality of essential Python modules:
* Python:[ time.sleep](https://docs.python.org/3/library/time.html#time.sleep); Go:[ workflow.Sleep](https://pkg.go.dev/go.uber.org/cadence@v1.2.9/workflow#Sleep).
* Python:[ time.time](https://docs.python.org/3/library/time.html#time.time); Go:[ workflow.Now](https://pkg.go.dev/go.uber.org/cadence@v1.2.9/workflow#Now).
* Python:[ concurrent.futures](https://docs.python.org/3/library/concurrent.futures.html); Go:[ workflow.Go](https://pkg.go.dev/go.uber.org/cadence@v1.2.9/workflow#Go).
* Python:[ uuid](https://docs.python.org/3/library/uuid.html); Go:[ uuid](https://pkg.go.dev/github.com/google/uuid) with[ workflow.SideEffect](https://pkg.go.dev/go.uber.org/cadence@v1.2.9/workflow#SideEffect).

Similarly, we can integrate with various Uber services (e.g., Flipr, QueryRunner, Experiments, etc.) by wrapping their Go clients in Cadence Activities and invoking them via[ workflow.ExecuteActivity](https://pkg.go.dev/go.uber.org/cadence@v1.2.9/workflow#ExecuteActivity).

Built-ins, along with tasks, form what's known as the Unified Framework. This framework allows developers to seamlessly integrate both resource-intensive processes and lightweight transactional operations into a single, cohesive workflow. Here is what an end-to-end model retraining workflow can look like from the perspective of tasks and built-ins:

![Screenshot 2025-02-19 at 7 43 27 PM](https://github.com/user-attachments/assets/12087c60-44e8-41d4-93a7-c10d7deef9ab)

# Context
**Context** is the entry point object for the workflow operations, such as:
Running the workflow locally, as detailed below in the Local Run section.
Building the workflow's distribution package, which is later to be used to run the workflow in the production environment at scale, as detailed below in the Remote Run and Starlark Package section.

The context contains runtime information, such as workflow function, input arguments, and environment variables, and is usually initialized in the __main__ block. For example:

```
if __name__ == "__main__":
    ctx = uniflow.create_context()
    ctx.environ.update({
        "LOG_LEVEL": "DEBUG",
        "RAY_AIR_FULL_TRACEBACKS": "1",
    })
    ctx.run(train_workflow, dataset_id="cola", train_params={ ... })
```

# Local Run
As an ML engineer, your primary productivity tool is your workstation, such as a laptop or a development pod. Uniflow prioritizes local development, treating it as a first-class concern.

In Local Run mode, workflows can execute within the local Python interpreter just like any standard Python code. This approach creates a frictionless local development experience, as it doesn't require setting up any additional infrastructure on the developer's machine other than Python.

The same code can be executed both with the local runtime and with the production runtime, enabling a rapid development, deployment, and debug cycle.

# Remote Run
While Local Run mode is for rapid workflow development on smaller datasets, Remote Run mode is for running workflows at scale and in a reproducible manner.

Remote Run mode executes workflow code within the Cadence Worker, running task code in a container using Kubernetes or other container orchestrators. This mode enables reliable and scalable workflow execution against large datasets.

# Starlark Package
To execute a workflow in the Remote Run mode, one first needs to create a distribution package with workflow instructions that the production workflow executor can understand. Uniflow analyzes the user-defined workflow code, compiles it into the appropriate Starlark code, and then packages the results into a compressed tarball file for distribution. This file is known as the Starlark Package.

A Starlark Package is a compressed tarball file that delivers workflow code to the Cadence Workflow Worker for execution. This package includes *.star files containing the workflow logic and a manifest file containing metadata, such as the path to the entry point file and the associated function name.
