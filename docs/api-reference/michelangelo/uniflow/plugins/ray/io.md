---
sidebar_label: io
title: michelangelo.uniflow.plugins.ray.io
---

I/O handlers for Ray datasets in Uniflow workflows.

Supports fsspec and PyArrow filesystem backends. Adds production-hardened data
quality filtering (skip zero-byte / empty parquet files) and a Polars fallback
for the PyArrow nested-data bug (https://github.com/ray-project/ray/issues/61675).

Filesystem backend is selected via ``UF_PLUGIN_RAY_USE_FSSPEC``:

- ``&quot;1&quot;`` — fsspec (flexible: local, S3, GCS, etc.). PyArrow accepts fsspec
  filesystems directly and wraps them transparently via ``FSSpecHandler``.
- ``&quot;0&quot;`` (default) — native PyArrow filesystem (S3 with MinIO credential support)

#### UF\_PLUGIN\_RAY\_USE\_FSSPEC

Environment variable: set to ``&quot;1&quot;`` to use fsspec instead of PyArrow.

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
(ray-project/ray#61675), the Polars fallback ``_ParquetPolarsDatasource``
retries the read. **Requires ``polars`` to be installed**
(``pip install michelangelo[ray-polars]``).

**Raises**:

- ``4 - If no parquet files are found at *url* on read.
  

**Example**:

  &gt;&gt;&gt; import ray, tempfile, pandas as pd
  &gt;&gt;&gt; ds = ray.data.from_pandas(pd.DataFrame([{&quot;x&quot;: 1}]))
  &gt;&gt;&gt; io = RayDatasetIO()
  &gt;&gt;&gt; dest = tempfile.mkdtemp()
  &gt;&gt;&gt; io.write(dest, ds)
  &gt;&gt;&gt; result = io.read(dest, None)
  &gt;&gt;&gt; result.count()
  1

#### write

```python
def write(url: str, value: Dataset) -> None
```

Write *value* to *url* as Parquet files.

**Arguments**:

- `url` - Destination directory path or URL (local, ``s3://``, etc.).
  Ray writes multiple shard files under this directory.
- `value` - Ray Dataset to write.
  

**Returns**:

  ``None`` — no metadata needed for the read path.

#### read

```python
def read(url: str, _metadata: Any | None) -> Dataset
```

Read a Ray Dataset from *url*, skipping empty parquet files.

**Arguments**:

- `url` - Source directory path or URL.
- `_metadata` - Unused; pass ``None``.
  

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

- `protocol` - URL scheme extracted from the target URL (e.g. ``&quot;s3&quot;``).
  

**Returns**:

  A ``pyarrow.fs.S3FileSystem`` for S3/MinIO, ``None`` otherwise.

