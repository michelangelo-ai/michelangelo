---
sidebar_label: experiment_store
title: michelangelo.lib.trainer.torch.pytorch_lightning.experiment_store
---

Default `schema.ExperimentStore` implementation for auto-resume.

Public module: users import and construct `FsspecExperimentStore` and
pass it as `LightningTrainerParam.experiment_store` to opt into auto-resume.
See `michelangelo.lib.trainer.torch.pytorch_lightning.schema.ExperimentStore`
for the seam it implements.

## FsspecExperimentStore Objects

```python
class FsspecExperimentStore()
```

Filesystem-backed `schema.ExperimentStore` using an fsspec marker file.

Persists a small JSON marker at a deterministic path so a re-run with the
same `RunConfig(storage_path=..., name=...)` can locate and resume the
previous run's experiment directory. Works with any fsspec scheme the
`storage_path` selects (local, `s3://`, `gs://`, ...).

The marker lives at `{storage_path}/.michelangelo_resume/{run_name}.json`
rather than inside the Ray experiment directory, because that path is
derivable purely from the stable `(storage_path, run_name)` identity and
does not depend on Ray's (possibly timestamp-suffixed) experiment directory
name. It inherits the same permissions and lifecycle as the run data it sits
beside.

Neither method raises: `track` logs and swallows any write failure,
and `locate_resumable` returns `None` on a missing, corrupt, or
unreadable marker. Staleness is not treated as an error — the trainer only
seeds a resume when the recorded directory still holds a restorable Ray
Train checkpoint, and otherwise starts fresh.

**Attributes**:

- `_MARKER_DIR` - Subdirectory (under `storage_path`) holding marker files.
- `_SCHEMA_VERSION` - Marker payload schema version, for forward evolution.

#### \_\_init\_\_

```python
def __init__(storage_options: dict | None = None) -> None
```

Initialize the store.

**Arguments**:

- `storage_options` - Optional keyword arguments forwarded to
  `fsspec.core.url_to_fs` (e.g. credentials or endpoint
  overrides for the backing filesystem). Kept as a plain dict so
  the store remains picklable for transport to Ray workers.

#### track

```python
def track(*, storage_path: str, run_name: str, experiment_path: str) -> None
```

Write the marker recording this run's experiment directory.

Best-effort: any failure is logged and swallowed so a failed marker
write can never fail an otherwise-successful training run. The payload
is written to a temporary sibling file and atomically renamed into
place, so a crash mid-write can never leave a partial/corrupt marker
that a later `locate_resumable` would read.

**Arguments**:

- `storage_path` - The driver's `RunConfig.storage_path`.
- `run_name` - The driver's `RunConfig.name`.
- `experiment_path` - Absolute path (scheme-qualified for remote
  filesystems, e.g. `s3://bucket/runs/my_run`) of this run's Ray
  Train experiment directory.
  

**Returns**:

`None`.

#### locate\_resumable

```python
def locate_resumable(*, storage_path: str, run_name: str) -> str | None
```

Read the marker and return the recorded experiment path, or `None`.

Returns `None` (never raises) when the marker is missing, corrupt, or
unreadable, or when it carries no `experiment_path`. A missing or
corrupt marker is the normal "nothing to resume yet" case and is logged
at `DEBUG`; only genuinely unexpected filesystem errors are logged at
`WARNING`.

**Arguments**:

- `storage_path` - The driver's `RunConfig.storage_path`.
- `run_name` - The driver's `RunConfig.name`.

**Returns**:

The recorded candidate experiment directory, or `None`.

