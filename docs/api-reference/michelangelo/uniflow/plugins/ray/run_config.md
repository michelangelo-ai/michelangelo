---
sidebar_label: run_config
title: michelangelo.uniflow.plugins.ray.run_config
---

Ray Train ``RunConfig`` helper defaulted to UniFlow-managed storage.

Any Ray-based workflow task (trainer, and future tasks with their own Ray
Train steps) can call :func:`create_run_config` instead of hand-rolling its
own ``storage_path``/``storage_filesystem`` defaulting from a task-specific
``storage_backend`` parameter. Centralizing this here keeps &quot;where does Ray
Train write checkpoints&quot; in one shared place as the task catalog grows,
rather than each task re-deriving it independently.

#### create\_run\_config

```python
def create_run_config(**kwargs) -> ray.train.RunConfig
```

Build a ``ray.train.RunConfig`` defaulted to UniFlow-managed storage.

Resolves ``storage_path``/``storage_filesystem`` from the same
``UF_STORAGE_URL`` environment variable that ``DatasetVariable`` and
``ModelVariable`` already use for their own storage location, via the
existing :func:``2 filesystem
resolver (native PyArrow S3, or fsspec when
``UF_PLUGIN_RAY_USE_FSSPEC=1``). Falls back to a local temp directory
when ``UF_STORAGE_URL`` is unset, so local/sandbox runs keep working
without extra configuration.

**Arguments**:

- ``7 - Any ``ray.train.RunConfig`` keyword argument. Explicitly
  passing ``storage_path`` and/or ``storage_filesystem`` overrides
  the ``UF_STORAGE_URL``-derived default for that field.
  

**Returns**:

  A ``ray.train.RunConfig`` with ``storage_path``/``storage_filesystem``
  defaulted as described above.

