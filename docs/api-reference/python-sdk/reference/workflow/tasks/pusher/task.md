---
sidebar_label: task
title: workflow.tasks.pusher.task
---

Top-level push() dispatch function for the Michelangelo pusher.

#### push

```python
def push(
    config: PusherConfig,
    artifacts: dict[str, Any],
    *,
    storage_backend: StorageBackend | None = None,
    registry_client: ModelRegistryClient | None = None,
    registry: PluginRegistry | None = None,
    fail_fast: bool = True,
    on_error: Callable[[str, str, Exception], None] | None = None
) -> list[PusherResult]
```

Push one or more artifacts using their configured plugins.

Iterates ``config.items`` in order, resolves each artifact from
``artifacts`` by name, instantiates the matching plugin, and calls
``execute()``. Infrastructure dependencies (``storage_backend``,
``registry_client``) are injected into every plugin.

**Arguments**:

- `config` - Top-level pusher configuration listing artifact/plugin pairs.
- `artifacts` - Mapping from artifact name to artifact value. Keys must
  match ``PusherPluginConfig.name`` for each item in config.
- `storage_backend` - Backend used for artifact uploads. Required —
  pass a ``LocalStorageBackend``, ``MinioStorageBackend``, or any
  `StorageBackend`
  subclass. Raises `ConfigurationError` when ``None``.
- `registry_client` - Registry client injected into plugins that require
  one (e.g. ``ModelPusherPlugin``). Pass ``None`` for plugins that
  don't need a registry, or when registry clients are specified
  directly on ``ModelPluginConfig.registry_clients``.
- `registry` - Plugin registry to resolve plugin names against. Defaults
  to ``default_registry`` when ``None``.
- `fail_fast` - When ``True`` (default), the first plugin failure raises
  ``PusherPluginError`` and subsequent items are not processed.
  When ``False``, all items run and failures are recorded in
  ``PusherResult.error``.
- `on_error` - Optional callback invoked on every plugin failure, regardless
  of ``fail_fast``. Signature: ``(artifact_name, plugin_name, exc)``.
  Exceptions raised by the callback are logged and suppressed.
  

**Returns**:

  List of `PusherResult`,
  one per ``config.items`` entry processed. In ``fail_fast=True`` mode
  the list is shorter than ``config.items`` when a failure occurs.
  

**Raises**:

- `ArtifactNotFoundError` - If a name in ``config.items`` is absent from
  ``artifacts``.
- `ConfigurationError` - If a plugin name is not registered, or the
  artifact type does not match the registered expected type.
- `PusherPluginError` - If a plugin's ``execute()`` raises and
  ``fail_fast=True``.
  

**Example**:

```python
import tempfile

from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend
from michelangelo.lib.model_manager.registry.client import (
    InMemoryRegistryClient,
)
from michelangelo.workflow.schema.pusher import (
    ModelPluginConfig, PusherConfig, PusherPluginConfig,
)
from michelangelo.workflow.tasks.pusher import push
from michelangelo.workflow.variables.types import AssembledModel, ModelArtifact

result = push(
    config=PusherConfig(items=[
        PusherPluginConfig(
            name="clf",
            model_plugin=ModelPluginConfig(model_name="my-classifier"),
        ),
    ]),
    artifacts={
        # path must point at an existing packaged model (file or directory)
        "clf": AssembledModel(raw_model=ModelArtifact(path="/tmp/raw"))
    },
    storage_backend=LocalStorageBackend(tempfile.mkdtemp()),
    registry_client=InMemoryRegistryClient(),
)
assert result[0].success
```

