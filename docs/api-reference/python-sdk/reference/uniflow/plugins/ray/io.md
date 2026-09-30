---
sidebar_label: io
title: uniflow.plugins.ray.io
---

I/O handlers for Ray datasets in Uniflow workflows.

Supports fsspec and PyArrow filesystem backends. Adds production-hardened data
quality filtering (skip zero-byte / empty parquet files) and a Polars fallback
for the PyArrow nested-data bug (https://github.com/ray-project/ray/issues/61675).

Filesystem backend is selected via ``UF_PLUGIN_RAY_USE_FSSPEC``:

- ``"1"`` — fsspec (flexible: local, S3, GCS, etc.). PyArrow accepts fsspec
  filesystems directly and wraps them transparently via ``FSSpecHandler``.
- ``"0"`` (default) — native PyArrow filesystem (S3 with MinIO credential support)

#### UF\_PLUGIN\_RAY\_USE\_FSSPEC

Environment variable: set to ``"1"`` to use fsspec instead of PyArrow.

#### UF\_PLUGIN\_RAY\_FILTER\_WORKERS

Environment variable: maximum parallel workers for empty-file filtering.

Default: 64.

## RayDatasetIO Objects

```python
class RayDatasetIO(IO[Dataset])
```

I/O handler for Ray Dataset objects stored as Parquet.

On **write**: delegates to ``Dataset.write_parquet`` with the configured filesystem.

On **read**:

1. ``filter_empty_data()`` lists all parquet files, discards zero-byte files,
and parallel-checks remaining files for non-empty row groups.
2. ``ray.data.read_parquet`` reads the survivors.
3. If PyArrow raises ``ArrowNotImplementedError`` on nested columns
(https://github.com/ray-project/ray/issues/61675), the Polars fallback
``_ParquetPolarsDatasource`` retries the read. **Requires ``polars`` to
be installed** (``pip install michelangelo[ray-polars]``).

**Raises**:

- `FileNotFoundError` - If no parquet files are found at *url* on read.
  

**Example**:

```python
>>> import ray, tempfile, pandas as pd
>>> ds = ray.data.from_pandas(pd.DataFrame([{"x": 1}]))
>>> io = RayDatasetIO()
>>> dest = tempfile.mkdtemp()
>>> io.write(dest, ds)
>>> result = io.read(dest, None)
>>> result.count()
1
```

#### write

```python
def write(url: str, value: Dataset, **write_kwargs: Any) -> None
```

Write *value* to *url* as Parquet files.

**Arguments**:

- `url` - Destination directory path or URL (local, ``s3://``, etc.).
  Ray writes multiple shard files under this directory.
- `value` - Ray Dataset to write.
- `**write_kwargs` - Additional kwargs forwarded to
  ``Dataset.write_parquet`` (e.g. ``max_rows_per_file``,
  ``min_rows_per_file``). Must not set ``filesystem``, which
  this method always supplies itself.
  

**Returns**:

  ``None`` — no metadata needed for the read path.

#### read

```python
def read(url: str, _metadata: Any | None, **read_kwargs: Any) -> Dataset
```

Read a Ray Dataset from *url*, skipping empty parquet files.

**Arguments**:

- `url` - Source directory path or URL.
- `_metadata` - Unused; pass ``None``.
- `**read_kwargs` - Additional kwargs forwarded to
  ``ray.data.read_parquet`` (e.g. as produced by
  ``parquet_read_config_to_kwargs``). Must not set
  ``filesystem`` or ``file_extensions``, which this method
  always supplies itself.
  

**Returns**:

  Ray Dataset loaded from Parquet shards under *url*.
  

**Raises**:

- `FileNotFoundError` - If no non-empty parquet files exist at *url*.

#### filter\_empty\_data

```python
@staticmethod
def filter_empty_data(url: str) -> list[str]
```

Return non-empty parquet file paths under *url*.

Steps:

1. ``fs.find(detail=True)`` — bulk listing (single round-trip).
2. Discard zero-byte files immediately.
3. Parallel-check remaining files for row groups (up to
``UF_PLUGIN_RAY_FILTER_WORKERS`` workers, default 64).

**Arguments**:

- `url` - Directory path or URL containing parquet files.
  

**Returns**:

  List of paths that contain at least one parquet row group.

#### resolve\_fs

```python
def resolve_fs(protocol: str) -> Any
```

Return a PyArrow filesystem for *protocol*, or ``None`` for local paths.

**Arguments**:

- `protocol` - URL scheme extracted from the target URL (e.g. ``"s3"``).
  

**Returns**:

  A ``pyarrow.fs.S3FileSystem`` for S3/MinIO, ``None`` otherwise.

