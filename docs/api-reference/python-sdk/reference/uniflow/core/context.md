---
sidebar_label: context
title: uniflow.core.context
---

Workflow execution context for local and remote runs.

This module provides the Context class and create_context() function for managing
workflow execution environments. It handles both local execution (for development
and testing) and remote execution (for production deployments on Cadence/Temporal).

The context system provides:

- Unified interface for local and remote workflow execution
- Environment variable management
- Command-line argument parsing
- Workflow validation and packaging
- Integration with Cadence and Temporal workflow engines

**Example**:

  Local workflow execution::
  
  from michelangelo.uniflow.core.context import create_context
  from michelangelo.uniflow.core.decorator import workflow
  
  @workflow()
  def my_workflow():
  return &quot;Hello, World!&quot;
  
  if __name__ == &quot;__main__&quot;:
  ctx = create_context()
  ctx.run(my_workflow)
  
  Remote workflow execution::
  
  # Command line:
  # python my_workflow.py remote-run \\
  #     --storage-url s3://bucket/storage \\
  #     --image my-image:latest
  
  ctx = create_context()  # Automatically detects remote-run mode
  ctx.run(my_workflow)

## Context Objects

```python
@dataclass(frozen=True)
class Context()
```

Represents the context for running a workflow, either locally or in-cluster.

**Attributes**:

- `_args` - Command-line arguments for the run.
- `_target` - The mode of the workflow execution. It can be &quot;local-run&quot; or
  &quot;remote-run&quot;.
- `environ` - Environment variables to set during execution.

#### is\_local\_run

```python
def is_local_run()
```

Check if the context is configured for local execution.

**Returns**:

  True if running in local mode, False for remote execution.

#### run

```python
def run(fn, *args, **kwargs)
```

Executes the workflow function in the specified context.

**Arguments**:

- `fn` - The workflow function to execute.
- `*args` - Positional arguments to pass to the function.
- `**kwargs` - Keyword arguments to pass to the function.

#### create\_context

```python
def create_context() -> Context
```

Create and configure the execution context based on command-line arguments.

Parses sys.argv to determine execution mode (local-run or remote-run) and
constructs an appropriate Context instance. If no mode is specified, defaults
to local-run.

**Returns**:

  A Context instance configured for the requested execution mode.
  

**Raises**:

- `AssertionError` - If an unsupported execution target is specified.
  

**Example**:

  Creating context for local execution::
  
  # python my_workflow.py
  # or: python my_workflow.py local-run
  ctx = create_context()
  assert ctx.is_local_run()
  
  Creating context for remote execution::
  
  # python my_workflow.py remote-run --storage-url s3://... --image ...
  ctx = create_context()
  assert not ctx.is_local_run()

