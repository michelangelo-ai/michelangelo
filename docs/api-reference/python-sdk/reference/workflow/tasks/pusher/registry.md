---
sidebar_label: registry
title: workflow.tasks.pusher.registry
---

Plugin registry for mapping plugin names to their implementations.

## PluginRegistry Objects

```python
class PluginRegistry()
```

Registry mapping plugin names to their implementation class and artifact type.

The open source library ships a ``default_registry`` pre-populated with
the three built-in plugins (populated by ``plugins/__init__.py``). Downstream
packages call ``extend()`` to create a child registry and register their own
plugins without mutating the shared default.

Lookups fall through to the parent registry when a name is not found
locally, forming a chain: child registry → ``default_registry``.

**Arguments**:

- `parent` - Optional parent registry to inherit registrations from. When
  a name is absent locally, lookup continues in the parent.
  

**Example**:

```python
from michelangelo.workflow.tasks.pusher.registry import default_registry

custom_registry = default_registry.extend()
custom_registry.register(
    "my_plugin",
    MyPlugin,
    MyArtifactType,
)
plugin_class, artifact_type = custom_registry.get("model_plugin")
```

#### \_\_init\_\_

```python
def __init__(parent: PluginRegistry | None = None) -> None
```

Initialise with an optional parent registry.

#### register

```python
def register(name: str,
             plugin_class: type[PusherPluginBase],
             artifact_type: type | tuple[type, ...] | None = None) -> None
```

Register a plugin under a given name.

Registering an already-registered name in the same instance raises an
error. To override a parent-registered plugin, register the same name
in a child registry created via ``extend()``.

**Arguments**:

- `name` - Plugin identifier used in ``PusherPluginConfig`` as the
  typed field name or as the ``plugin_name`` extension value.
- `plugin_class` - Concrete subclass of ``PusherPluginBase``.
- `artifact_type` - Expected Python type (or tuple of types) of the
  artifact value. When provided, ``push()`` validates
  ``isinstance(artifact, artifact_type)`` before invoking the
  plugin. Pass ``None`` for config-only plugins or when the
  plugin accepts any artifact type.
  

**Raises**:

- `ValueError` - If ``name`` is already registered in this instance.
  Overrides must go through a child registry via ``extend()``.
  

**Example**:

```python
>>> registry = PluginRegistry()
>>> # registry.register("model_plugin", ModelPusherPlugin, AssembledModel)
```

#### get

```python
def get(
    name: str
) -> tuple[type[PusherPluginBase], type | tuple[type, ...] | None]
```

Look up a plugin by name, falling through to the parent if needed.

**Arguments**:

- `name` - Plugin name to look up.
  

**Returns**:

  A tuple of ``(plugin_class, artifact_type)``. ``artifact_type``
  may be ``None`` for config-only plugins.
  

**Raises**:

- `ConfigurationError` - If ``name`` is not found in this registry or
  any ancestor registry.
  

**Example**:

```python
>>> registry = PluginRegistry()
>>> registry.get("unknown")  # doctest: +IGNORE_EXCEPTION_DETAIL
Traceback (most recent call last):
    ...
michelangelo.workflow.schema.exceptions.ConfigurationError: ...
```

#### registered\_names

```python
def registered_names() -> list[str]
```

Return all plugin names visible from this registry, including parents.

**Returns**:

  Sorted list of plugin name strings from this instance and all
  ancestor registries.
  

**Example**:

```python
>>> registry = PluginRegistry()
>>> registry.registered_names()
[]
```

#### extend

```python
def extend() -> PluginRegistry
```

Create a child registry that inherits all registrations from this one.

Lookups on the child fall through to this registry for any name not
registered locally. Registering the same name in the child overrides
the parent's registration without mutating it.

**Returns**:

  A new ``PluginRegistry`` whose parent is this registry.
  

**Example**:

```python
from michelangelo.workflow.tasks.pusher.registry import default_registry

provider_registry = default_registry.extend()
provider_registry.register(
    "model_plugin",
    ProviderModelPusherPlugin,
    AssembledModel,
)
```

