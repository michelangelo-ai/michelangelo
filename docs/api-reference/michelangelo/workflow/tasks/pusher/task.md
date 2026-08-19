---
sidebar_label: task
title: michelangelo.workflow.tasks.pusher.task
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

- ``0 - Top-level pusher configuration listing artifact/plugin pairs.
- ``1 - Mapping from artifact name to artifact value. Keys must
  match ``PusherPluginConfig.name`` for each item in config.
- ``4 - Backend used for artifact uploads. Required —
  pass a ``LocalStorageBackend``, ``MinioStorageBackend``, or any
  :class:``9
  subclass. Raises :class:``0 when ``None``.
- ``3 - Registry client injected into plugins that require
  one (e.g. ``ModelPusherPlugin``). Pass ``None`` for plugins that
  don&#x27;t need a registry, or when registry clients are specified
  directly on ``ModelPluginConfig.registry_clients``.
- ``0 - Plugin registry to resolve plugin names against. Defaults
  to ``default_registry`` when ``None``.
- ``5 - When ``True`` (default), the first plugin failure raises
  ``PusherPluginError`` and subsequent items are not processed.
  When ``False``, all items run and failures are recorded in
  ``PusherResult.error``.
- ``4 - Optional callback invoked on every plugin failure, regardless
  of ``fail_fast``. Signature: ``(artifact_name, plugin_name, exc)``.
  Exceptions raised by the callback are logged and suppressed.
  

**Returns**:

  List of :class:``9,
  one per ``config.items`` entry processed. In ``fail_fast=True`` mode
  the list is shorter than ``config.items`` when a failure occurs.
  

**Raises**:

- ``6 - If a name in ``config.items`` is absent from
  ``artifacts``.
- ``0 - If a plugin name is not registered, or the
  artifact type does not match the registered expected type.
- ``2 - If a plugin&#x27;s ``execute()`` raises and
  ``fail_fast=True``.
  
  Example::
  
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
  name=&quot;clf&quot;,
  model_plugin=ModelPluginConfig(model_name=&quot;my-classifier&quot;),
  ),
  ]),
  artifacts={
- ``7 - AssembledModel(raw_model=ModelArtifact(path=&quot;/tmp/raw&quot;))
  },
  registry_client=InMemoryRegistryClient(),
  )
  assert result[0].success

