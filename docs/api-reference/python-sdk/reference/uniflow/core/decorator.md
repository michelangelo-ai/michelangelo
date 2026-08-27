---
sidebar_label: decorator
title: uniflow.core.decorator
---

Task and workflow decorators for Uniflow.

This module provides the core decorators for defining tasks and workflows in Uniflow.
Tasks are units of computation that can be executed locally or distributed, while
workflows orchestrate multiple tasks together.

The @task decorator wraps functions with execution logic including caching, retry
handling, I/O management, and resource configuration. The @workflow decorator marks
functions as workflow entry points.

**Example**:

  Basic task definition::
  
  from michelangelo.uniflow.core import task
  from michelangelo.uniflow.plugins.ray import RayTask
  
  @task(config=RayTask(head_cpu=2, head_memory=&quot;4Gi&quot;))
  def process_data(input_file: str) -&gt; dict:
  # Process data and return results
  return {&quot;status&quot;: &quot;complete&quot;}
  
  Workflow with multiple tasks::
  
  @workflow()
  def my_workflow():
  data = load_data()
  result = process_data(data)
  return result

## TaskFunction Objects

```python
class TaskFunction(Generic[P, R])
```

Executable task wrapper for decorated functions.

TaskFunction wraps a callable with Uniflow execution logic including caching,
retry handling, argument serialization/deserialization, and lifecycle hooks.
It manages task context, resource configuration, and result persistence.

This class is typically created by the @task decorator and should not be
instantiated directly.

**Attributes**:

- `fn` - The wrapped Python function.
- `config` - Task configuration specifying execution environment.
- `alias` - Optional alternative name for the task.
- `io` - I/O registry for serialization operations.
- `cache_enabled` - Whether to enable result caching.
- `cache_version` - Optional version identifier for cached results.
- `retry_attempts` - Number of times to retry failed executions.
- `image_spec` - Optional container image specification.
  

**Example**:

  &gt;&gt;&gt; @task(config=RayTask(head_cpu=1))
  ... def my_task(x: int) -&gt; int:
  ...     return x * 2
  &gt;&gt;&gt; result = my_task(5)  # Executes wrapped function

#### \_\_init\_\_

```python
def __init__(*,
             fn: Callable[P, R],
             config: TaskConfig,
             alias: Optional[str],
             io: IORegistry,
             cache_enabled: bool = False,
             cache_version: Optional[str] = None,
             retry_attempts: int = DEFAULT_RETRY_ATTEMPTS,
             image_spec: Optional[ImageSpec] = None)
```

Initialize a TaskFunction.

**Arguments**:

- `fn` - The function to wrap.
- `config` - Task configuration defining execution environment.
- `alias` - Optional alternative task name.
- `io` - I/O registry for serialization.
- `cache_enabled` - Enable result caching. Defaults to False.
- `cache_version` - Optional cache version identifier.
- `retry_attempts` - Number of retry attempts. Defaults to 0.
- `image_spec` - Optional container image specification.

#### image\_spec

```python
@property
def image_spec() -> Optional[ImageSpec]
```

Get the container image specification for this task.

**Returns**:

  The ImageSpec if specified, None otherwise.

#### fn

```python
@property
def fn() -> Callable[P, R]
```

Get the wrapped function.

**Returns**:

  The original decorated function.

#### \_\_call\_\_

```python
def __call__(*args: P.args, **kwargs: P.kwargs) -> R
```

Execute the task with the given arguments.

Manages the complete task execution lifecycle:
1. Checks for nested task calls (executes directly)
2. Logs task invocation and arguments
3. Sets up task context
4. Runs pre-execution hooks
5. Deserializes input arguments
6. Executes the wrapped function
7. Serializes and persists results
8. Runs post-execution hooks
9. Cleans up task context

**Arguments**:

- `*args` - Positional arguments to pass to the wrapped function.
- `**kwargs` - Keyword arguments to pass to the wrapped function.
  Special keyword _uf_result_url can be used to specify
  where to write the result.
  

**Returns**:

  The result of executing the wrapped function, wrapped in a Ref.
  

**Raises**:

  Any exception raised by the wrapped function or lifecycle hooks.

#### with\_overrides

```python
def with_overrides(
        *,
        alias: Optional[str] = None,
        config: Optional[TaskConfig] = None,
        retry_attempts: Optional[int] = None) -> "TaskFunction[P, R]"
```

Create a new TaskFunction with overridden configuration.

This method allows creating a variant of the task with different configuration
while sharing the same function, I/O registry, and cache settings. Useful for
running the same task with different resource allocations.

**Arguments**:

- `alias` - Optional alternative task name. If not provided, uses original alias.
- `config` - Optional task configuration. If provided, this configuration will be
  merged with the original configuration. For example, if the original
  config specifies head_cpu=4 and head_memory=&quot;16Gi&quot;, and the new
  config specifies head_cpu=8, the result will have head_cpu=8 and
  head_memory=&quot;16Gi&quot;.
- `retry_attempts` - Optional retry count. If not provided, uses original value.
  

**Returns**:

  A new TaskFunction instance with the specified overrides.
  

**Example**:

  &gt;&gt;&gt; @task(config=RayTask(head_cpu=2))
  ... def my_task(x):
  ...     return x * 2
  &gt;&gt;&gt; high_cpu_task = my_task.with_overrides(
  ...     alias=&quot;my_task_8cpu&quot;,
  ...     config=RayTask(head_cpu=8)
  ... )

#### task

```python
def task(config: TaskConfig,
         alias: Optional[str] = None,
         io: IORegistry = default_io,
         cache_enabled: bool = False,
         cache_version: Optional[str] = None,
         retry_attempts: int = DEFAULT_RETRY_ATTEMPTS,
         image_spec: Optional[ImageSpec] = None)
```

Decorator for defining a Uniflow task.

Wraps a function to make it executable as a Uniflow task with caching,
retry handling, and resource configuration. Tasks can be executed locally
or distributed across Ray/Spark clusters depending on the config.

**Arguments**:

- `config` - Task configuration defining execution environment (e.g., RayTask,
  SparkTask). Specifies resources like CPU, memory, and GPU allocation.
- `alias` - Optional alternative task name. If not provided, uses function name.
- `io` - I/O registry for serialization. Defaults to default_io.
- `cache_enabled` - Enable result caching. When True, the task checks for cached
  results before execution. If found, returns cached result. If not found,
  executes and caches the result. Defaults to False.
- `cache_version` - Optional version identifier for cached results. When None,
  version is calculated from the Docker image ID. Use this to maintain
  multiple cache versions for the same task.
- `retry_attempts` - Number of times to retry failed executions. Defaults to 0
  (no retries).
- `image_spec` - Optional container image specification. Allows specifying custom
  container images and build targets for the task execution environment.
  

**Returns**:

  A decorator that converts a function into a TaskFunction.
  

**Example**:

  Basic task with caching::
  
  @task(config=RayTask(head_cpu=2), cache_enabled=True)
  def process_data(input_path: str) -&gt; dict:
  # Process data
  return {&quot;status&quot;: &quot;complete&quot;}
  
  Task with custom image::
  
  @task(
  config=RayTask(head_cpu=4, head_memory=&quot;8Gi&quot;),
  image_spec=ImageSpec(
  container_image=&quot;my-image:latest&quot;,
  recipe=&quot;bazel://path/to:target&quot;
  )
  )
  def train_model(data: pd.DataFrame) -&gt; Model:
  # Train model
  return trained_model
  
  Task with alias and retry::
  
  @task(
  config=SparkTask(driver_cpu=2, executor_cpu=4),
  alias=&quot;preprocess_v2&quot;,
  retry_attempts=3
  )
  def preprocess(df: DataFrame) -&gt; DataFrame:
  # Preprocess DataFrame
  return processed_df

#### workflow

```python
def workflow()
```

Decorator for defining a Uniflow workflow.

Marks a function as a workflow entry point. Workflows orchestrate multiple
tasks together and define the overall execution flow. Unlike tasks, workflows
are always executed locally and serve as the coordination layer.

**Returns**:

  A decorator that marks a function as a workflow.
  

**Example**:

  Simple workflow::
  
  @workflow()
  def my_workflow(input_file: str):
  # Load data
  data = load_task(input_file)
  
  # Process data
  result = process_task(data)
  
  # Save results
  save_task(result)
  
  return result
  
  Workflow with multiple stages::
  
  @workflow()
  def training_pipeline(dataset_path: str, model_type: str):
  # Data preparation stage
  raw_data = load_data(dataset_path)
  clean_data = clean_data_task(raw_data)
  
  # Training stage
  model = train_model_task(clean_data, model_type)
  
  # Evaluation stage
  metrics = evaluate_model_task(model, clean_data)
  
  return {&quot;model&quot;: model, &quot;metrics&quot;: metrics}

#### star\_plugin

```python
def star_plugin(binding: str)
```

Decorator for Starlark plugin functions.

Marks a Python function as a Starlark plugin, making it available for
invocation from Starlark workflow definitions. Plugins bridge Python
functionality into the Starlark execution environment.

**Arguments**:

- `binding` - The binding name to use in Starlark.
  

**Returns**:

  A decorator that marks a function as a Starlark plugin.
  

**Example**:

  &gt;&gt;&gt; @star_plugin(binding=&quot;custom_transform&quot;)
  ... def transform_data(data: dict) -&gt; dict:
  ...     # Transform logic
  ...     return transformed

#### is\_star\_plugin

```python
def is_star_plugin(fn) -> bool
```

Check if a function is a Starlark plugin.

**Arguments**:

- `fn` - Function to check.
  

**Returns**:

  True if the function is marked as a Starlark plugin, False otherwise.

#### is\_workflow

```python
def is_workflow(fn) -> bool
```

Check if a function is a workflow.

**Arguments**:

- `fn` - Function to check.
  

**Returns**:

  True if the function is marked as a workflow, False otherwise.

#### get\_star\_plugin\_binding

```python
def get_star_plugin_binding(fn) -> str
```

Get the Starlark binding name for a plugin function.

**Arguments**:

- `fn` - Function to get binding from.
  

**Returns**:

  The binding name string.
  

**Raises**:

- `AttributeError` - If the function is not a Starlark plugin.

#### write\_task\_result

```python
def write_task_result(url: str, value)
```

Write task result to a file at the specified URL.

Serializes the value to JSON using the codec system and writes to the
specified filesystem URL.

**Arguments**:

- `url` - Filesystem URL where to write the result.
- `value` - The value to serialize and write.
  

**Raises**:

- `IOError` - If writing to the URL fails.

