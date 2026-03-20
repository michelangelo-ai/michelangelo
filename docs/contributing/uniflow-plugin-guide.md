# Developing Uniflow Plugins

This guide walks you through the end-to-end process of building a new Uniflow plugin — from the Go worker plugin that runs on Cadence/Temporal, to the Starlark orchestration layer, to the Python `TaskConfig` that users interact with.

## Architecture Overview

Uniflow is Michelangelo's workflow execution system. Users write Python workflows using `@uniflow.task` and `@uniflow.workflow` decorators. At submission time, these Python workflows are **transpiled to Starlark** — a simplified Python-like scripting language — and executed remotely on a Cadence/Temporal worker.

**Plugins** extend Starlark with domain-specific capabilities (creating Ray clusters, submitting Spark jobs, triggering pipeline runs, etc.). A plugin has three layers:

| Layer         | Language | Location                              | Purpose                                                        |
| ------------- | -------- | ------------------------------------- | -------------------------------------------------------------- |
| Worker Plugin | Go       | `go/worker/plugins/<name>/`           | Executes activities on Cadence/Temporal (remote mode)          |
| Orchestration | Starlark | `python/.../plugins/<name>/task.star` | Orchestration logic — calls plugin builtins, manages lifecycle |
| Task Config   | Python   | `python/.../plugins/<name>/task.py`   | User-facing configuration dataclass                            |

## Step-by-Step Guide

This guide uses the **Ray plugin** as a reference example. Each section shows the real code and explains the pattern you should follow.

### Step 1: Create the Go Worker Plugin

Create a new directory: `go/worker/plugins/<your_plugin>/`

#### 1.1 Define the Plugin Entry Point (`plugin.go`)

Every plugin implements the `service.IPlugin` interface with three methods:

- `ID()` — returns the plugin's string identifier (used in Starlark `load("@plugin", "<id>")`)
- `Create()` — returns a new Starlark module instance for each workflow execution
- `Register()` — registers any Cadence/Temporal activities (optional)

```go
const pluginID = "ray"

type plugin struct{}

func (r *plugin) ID() string                              { return pluginID }
func (r *plugin) Create(_ service.RunInfo) starlark.Value { return newModule() }
func (r *plugin) Register(_ worker.Registry)              {}
```

> See full implementation: [`go/worker/plugins/ray/plugin.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/worker/plugins/ray/plugin.go)

Key points:

- `pluginID` (`"ray"`) is what Starlark code uses in `load("@plugin", "ray")`.

#### 1.2 Implement the Starlark Module (`starlark_module.go`)

The module is a Go struct that implements `starlark.HasAttrs`. It exposes builtin functions as attributes that Starlark code can call.

**Reference: `go/worker/plugins/ray/starlark_module.go`** (simplified)

```go
type module struct {
    attributes map[string]starlark.Value
}

func newModule() starlark.Value {
    m := &module{}
    m.attributes = map[string]starlark.Value{
        "create_cluster": starlark.NewBuiltin("create_cluster", m.createCluster).BindReceiver(m),
        "create_job":     starlark.NewBuiltin("create_job", m.createJob).BindReceiver(m),
        ...
    }
    return m
}

// Must also implement: String(), Type(), Freeze(), Truth(), Hash(), Attr(), AttrNames()
```

> See full implementation: [`go/worker/plugins/ray/starlark_module.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/worker/plugins/ray/starlark_module.go)

Each builtin function follows a consistent pattern:

1. Extract the workflow context from the Starlark thread
2. Parse arguments using `starlark.UnpackArgs()`
3. Convert Starlark values to Go types using `utils.AsGo()`
4. Execute a Go activity via `workflow.ExecuteActivity()`
5. Convert the result back to Starlark using `utils.AsStar()`

### Step 2: Register the Plugin

Add your plugin to the registration hub at [`go/worker/starlark/module.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/worker/starlark/module.go):

```go
func RegisterYourPlugin(registry map[string]service.IPlugin) {
    registry[yourplugin.Plugin.ID()] = yourplugin.Plugin
}

var Module = fx.Options(
    // ... existing plugins ...
    fx.Invoke(RegisterYourPlugin),
)
```

### Step 3: Write the Starlark Orchestration File

Create `python/michelangelo/uniflow/plugins/<your_plugin>/task.star`. This file contains the orchestration logic that calls your Go plugin builtins.

**Reference: `python/michelangelo/uniflow/plugins/ray/task.star`**

```python
load("@plugin", "ray")  # "ray" must match Go plugin's ID()

def task(task_path, head_cpu = "8", head_memory = "32Gi", ...):
    def callable(*args, **kwargs):
        cluster_response = ray.create_cluster(cluster_spec)  # calls Go builtin
        job = ray.create_job(entrypoint, ray_job_namespace=ns, ray_job_name=name)
        ...
        return result
    return callable
```

> See full implementation: [`python/michelangelo/uniflow/plugins/ray/task.star`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/uniflow/plugins/ray/task.star)

Key points:

- `load("@plugin", "ray")` makes the Go plugin available as a Starlark module.
- The function names called on the module (e.g., `ray.create_cluster`) must match the attribute names registered in `newModule()` in your Go code.
- The `.star` file handles orchestration: sequencing calls, retries, caching, progress reporting, and cleanup.

### Step 4: Create the Python TaskConfig

Create `python/michelangelo/uniflow/plugins/<your_plugin>/task.py`. This is the user-facing dataclass that configures how the plugin executes.

**Reference: `python/michelangelo/uniflow/plugins/ray/task.py`**

```python
_binding = TaskBinding(
    star_file=Path(__file__).resolve().parent / "task.star",
    function="task",        # function name in task.star
    export="__ray_task",    # alias for generated load() statements
)

@dataclass
class RayTask(TaskConfig):
    head_cpu: Optional[int] = None
    head_memory: Optional[str] = None
    # ...

    def get_binding(self) -> TaskBinding:        return _binding
    def get_config_binding(cls) -> TaskBinding:  return _config_binding
    def pre_run(self):   ray.init(...)     # setup before task
    def post_run(self):  ray.shutdown()    # cleanup after task
```

> See full implementation: [`python/michelangelo/uniflow/plugins/ray/task.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/uniflow/plugins/ray/task.py)

**`TaskConfig` requires four abstract methods:**

| Method                 | Purpose                                                               |
| ---------------------- | --------------------------------------------------------------------- |
| `get_binding()`        | Links instance to the Starlark function that wraps task execution     |
| `get_config_binding()` | Links class to the Starlark function for config overrides             |
| `pre_run()`            | Lifecycle hook: setup before task execution (e.g., init cluster)      |
| `post_run()`           | Lifecycle hook: cleanup after task execution (e.g., shutdown cluster) |

**`TaskBinding` fields:**

| Field       | Purpose                                                                                |
| ----------- | -------------------------------------------------------------------------------------- |
| `star_file` | Path to the `.star` file containing the orchestration function                         |
| `function`  | Name of the Starlark function in that file                                             |
| `export`    | Alias for the generated `load()` statement (use `__` prefix to avoid naming conflicts) |

Dataclass fields with non-`None` values are automatically converted to Starlark keyword arguments during transpilation via `to_keywords()`.

> See base class: [`python/michelangelo/uniflow/core/task_config.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/uniflow/core/task_config.py)

### Step 5: User-Facing API

After completing the steps above, users can use your plugin like this:

```python
from michelangelo import uniflow
from michelangelo.uniflow.plugins.your_plugin.task import YourTask

@uniflow.task(config=YourTask(head_cpu=4, head_memory="16Gi"))
def my_task(data):
    # User's task code runs inside the configured environment
    ...

@uniflow.workflow()
def my_workflow():
    my_task(data)
```

When the workflow is submitted, Uniflow transpiles this to Starlark, which calls your `.star` orchestration function, which in turn calls your Go plugin builtins on the remote worker.

## File Checklist

When creating a new plugin, you'll touch these files:

**New files to create:**

| File                                               | Purpose                                |
| -------------------------------------------------- | -------------------------------------- |
| `go/worker/plugins/<name>/plugin.go`               | Plugin entry point                     |
| `go/worker/plugins/<name>/starlark_module.go`      | Starlark module with builtin functions |
| `go/worker/plugins/<name>/starlark_module_test.go` | Unit tests                             |
| `go/worker/plugins/<name>/BUILD.bazel`             | Build configuration                    |
| `python/.../uniflow/plugins/<name>/task.star`      | Starlark orchestration logic           |
| `python/.../uniflow/plugins/<name>/task.py`        | Python TaskConfig dataclass            |
| `python/.../uniflow/plugins/<name>/__init__.py`    | Package exports                        |

**Existing files to modify:**

| File                           | Change                                         |
| ------------------------------ | ---------------------------------------------- |
| `go/worker/starlark/module.go` | Add `Register<Name>Plugin()` and `fx.Invoke()` |

## Further Reading

- [Starlark Worker README](https://github.com/michelangelo-ai/michelangelo/blob/main/go/worker/starlark/README.md) — running and testing Starlark workflows locally
- [Pipeline Plugin README](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/uniflow/plugins/pipeline/README.md) — detailed API reference for the pipeline plugin
- [Starlark Language Spec](https://github.com/google/starlark-go/blob/master/doc/spec.md) — Starlark language reference
