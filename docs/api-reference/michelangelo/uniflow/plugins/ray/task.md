---
sidebar_label: task
title: michelangelo.uniflow.plugins.ray.task
---

Ray task configuration and execution for Uniflow workflows.

This module provides task configuration for executing Uniflow workflows on Ray clusters.
It handles Ray cluster initialization, resource allocation, and lifecycle management
for distributed task execution.

Ray tasks support configurable resources for both head and worker nodes, including
CPU, memory, disk, and GPU allocations. The execution model initializes a Ray cluster
before running the task and properly shuts it down afterward.

## RayTask Objects

```python
@dataclass
class RayTask(TaskConfig)
```

Configuration for Ray-based task execution in Uniflow workflows.

This class defines resource specifications and runtime configuration for executing
tasks on Ray clusters. It manages the lifecycle of Ray cluster initialization and
shutdown through pre_run and post_run hooks.

Unlike SparkTask which uses driver/executor terminology, RayTask uses head/worker
terminology to describe the cluster nodes. The head node coordinates execution
while worker nodes perform distributed computation.

**Attributes**:

- `head_cpu` - Number of CPUs allocated to the head node.
- `head_memory` - Memory allocation for the head node (e.g., &quot;4G&quot;, &quot;512M&quot;).
- `head_disk` - Disk space allocation for the head node (e.g., &quot;10G&quot;).
- `head_gpu` - Number of GPUs allocated to the head node.
- `head_object_store_memory` - Object store memory for the head node in bytes.
- `worker_cpu` - Number of CPUs allocated per worker node.
- `worker_memory` - Memory allocation per worker node (e.g., &quot;4G&quot;, &quot;512M&quot;).
- `worker_disk` - Disk space allocation per worker node (e.g., &quot;10G&quot;).
- `worker_gpu` - Number of GPUs allocated per worker node.
- `worker_object_store_memory` - Object store memory per worker node in bytes.
- `head_memory`0 - Number of worker instances to launch.
- `head_memory`1 - If True, enables breakpoint debugging for the task.
- `head_memory`2 - Runtime environment configuration dict for Ray
  (packages, env vars, etc.).

#### get\_binding

```python
def get_binding() -> TaskBinding
```

Return the TaskBinding linking this config to its Starlark function.

**Returns**:

  TaskBinding that specifies the Starlark file and function for
  Ray task execution.

#### get\_config\_binding

```python
@classmethod
def get_config_binding(cls) -> TaskBinding
```

Return the TaskBinding for Ray configuration.

**Returns**:

  TaskBinding that specifies the Starlark file and function for
  Ray configuration.

#### pre\_run

```python
def pre_run()
```

Initialize the Ray cluster before task execution.

Reads Ray initialization parameters from the _RAY_INIT_KWARGS
environment variable and initializes the Ray runtime with those
parameters.

#### post\_run

```python
def post_run()
```

Shut down the Ray cluster after task execution.

Ensures proper cleanup of Ray resources by shutting down the Ray runtime.

