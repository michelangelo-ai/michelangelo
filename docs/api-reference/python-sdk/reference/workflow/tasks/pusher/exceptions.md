---
sidebar_label: exceptions
title: michelangelo.workflow.tasks.pusher.exceptions
---

Runtime exception hierarchy for the pusher module.

Defines `PusherError` and its runtime subclasses (`ArtifactNotFoundError`,
`PusherPluginError`). These are raised during `push()` execution.

`ConfigurationError` is a schema-layer exception defined in
`michelangelo.workflow.schema.exceptions` and re-exported here for backwards
compatibility, so `from michelangelo.workflow.tasks.pusher.exceptions import
ConfigurationError` works. It is intentionally *not* a subclass of
`PusherError`, since it's raised by config dataclass `__post_init__`
validation before any push execution begins, not by the runtime.
`michelangelo.workflow.schema.exceptions` isn't part of this generated
reference yet, so `ConfigurationError` itself isn't documented on this page.

## PusherError Objects

```python
class PusherError(Exception)
```

Base exception class for all pusher runtime errors.

All exceptions raised by the pusher module at execution time inherit from
this class, allowing callers to catch the full family with a single
`except PusherError` clause.

Note that `ConfigurationError` (raised at config-validation time) is
intentionally *not* a subclass of `PusherError` — see the module
description above for why.

## ArtifactNotFoundError Objects

```python
class ArtifactNotFoundError(PusherError)
```

Raised when an artifact named in config is absent from the artifacts dict.

**Arguments**:

- `name` - The artifact name that was expected.
- `available` - The artifact names that are actually present in the dict.

**Example**:

```python
>>> err = ArtifactNotFoundError("model", ["dataset", "report"])
>>> "model" in str(err)
True
```

#### \_\_init\_\_

```python
def __init__(name: str, available: list[str]) -> None
```

Initialize with the missing artifact name and available names.

## PusherPluginError Objects

```python
class PusherPluginError(PusherError)
```

Raised when a plugin's `execute()` raises an unexpected exception.

This exception is raised by `push()` when `fail_fast=True` and a
plugin raises. The original exception is chained via the `__cause__`
attribute so the full stack trace is preserved.

**Arguments**:

- `artifact_name` - The artifact name the plugin was processing.
- `plugin_name` - The name of the plugin that raised.

**Example**:

```python
>>> err = PusherPluginError("model", "model_plugin")
>>> "model_plugin" in str(err)
True
```

#### \_\_init\_\_

```python
def __init__(artifact_name: str, plugin_name: str) -> None
```

Initialize with the artifact name and plugin name that failed.
