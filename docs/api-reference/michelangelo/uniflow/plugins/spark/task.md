---
sidebar_label: task
title: michelangelo.uniflow.plugins.spark.task
---

Spark task configuration and execution for Uniflow workflows.

This module provides task configuration for executing Uniflow workflows on
Spark clusters. It handles Spark session initialization, resource allocation,
and lifecycle management for distributed task execution.

Spark tasks support configurable resources for both driver and executor nodes,
including CPU, memory, disk, and GPU allocations. The execution model
initializes a Spark session with Hive support before running the task and
properly stops it afterward.

## SparkTask Objects

```python
@dataclass
class SparkTask(TaskConfig)
```

Configuration for Spark-based task execution in Uniflow workflows.

This class defines resource specifications and runtime configuration for executing
tasks on Spark clusters. It manages the lifecycle of Spark session initialization
and shutdown through pre_run and post_run hooks.

Unlike RayTask which uses head/worker terminology, SparkTask uses driver/executor
terminology to describe the cluster nodes. The driver coordinates execution while
executors perform distributed computation.

The class intentionally avoids defining default values for its properties. Instead,
defaults should be provided through the keyword arguments of the associated Starlark
function. This approach facilitates more flexible configuration management, allowing
runtime overrides of the default settings.

**Attributes**:

- `driver_cpu` - Number of CPUs allocated to the driver node.
- `driver_memory` - Memory allocation for the driver node (e.g., &quot;4G&quot;, &quot;512M&quot;).
- `driver_disk` - Disk space allocation for the driver node (e.g., &quot;10G&quot;).
- `driver_gpu` - Number of GPUs allocated to the driver node.
- `executor_cpu` - Number of CPUs allocated per executor.
- `executor_memory` - Memory allocation per executor (e.g., &quot;4G&quot;, &quot;512M&quot;).
- `executor_disk` - Disk space allocation per executor (e.g., &quot;10G&quot;).
- `executor_gpu` - Number of GPUs allocated per executor.
- `executor_instances` - Number of executor instances to launch.

#### get\_binding

```python
def get_binding() -> TaskBinding
```

Return the TaskBinding linking this config to its Starlark function.

**Returns**:

  TaskBinding that specifies the Starlark file and function for
  Spark task execution.

#### get\_config\_binding

```python
@classmethod
def get_config_binding(cls) -> TaskBinding
```

Return the TaskBinding for Spark configuration.

**Returns**:

  TaskBinding that specifies the Starlark file and function for
  Spark configuration.

#### pre\_run

```python
def pre_run()
```

Initialize the Spark session before task execution.

Creates a Spark session with Hive support enabled. Additional Spark
properties can be specified via the _SPARK_PROPERTIES environment
variable as comma-separated key=value pairs.

#### post\_run

```python
def post_run()
```

Stop the Spark session after task execution.

Ensures proper cleanup of Spark resources by stopping the active
session if one exists.

