---
sidebar_label: io
title: michelangelo.uniflow.plugins.pandas.io
---

I/O handler for pandas DataFrames in Uniflow workflows.

Reads and writes DataFrames in Parquet format using PyArrow with zstd
compression, supporting local and remote filesystems via fsspec. Writes
are partitioned into part files (max 2 M rows per file, 1 M rows per
group) so large DataFrames can be read back in parallel by Spark or Ray.

PyArrow and fsspec are imported lazily inside write() and read() to avoid
circular-import issues when pandas itself is being initialised.

## PandasIO Objects

```python
class PandasIO(IO["pd.DataFrame"])
```

I/O handler for ``pandas.DataFrame`` objects.

Serialises DataFrames to Parquet via PyArrow (zstd compression) and
reads them back. Supports any filesystem accessible via fsspec —
local paths, ``s3://``, ``gcs://``, ``hdfs://``, etc.

**Arguments**:

- `storage_options` - Optional fsspec storage options forwarded to
  ``fsspec.core.url_to_fs``. Use this to pass credentials or
  endpoint overrides for remote filesystems (e.g.
- ````2 - &quot;...&quot;, &quot;secret&quot;: &quot;...&quot;}`` for S3).
  
  Example::
  
  import pandas as pd
  from michelangelo.uniflow.plugins.pandas import PandasIO
  
  io = PandasIO()
  df = pd.DataFrame([{&quot;x&quot;: 1}, {&quot;x&quot;: 2}])
  io.write(&quot;/tmp/mydata&quot;, df)
  loaded = io.read(&quot;/tmp/mydata&quot;, None)
  assert len(loaded) == 2
  
  .. note::
  ``PandasIO`` writes a **directory** of ``part-*.parquet`` files, not a
  single file. This differs from ``LocalFileSink`` (workflow pusher),
  which writes a single ``data.parquet`` via ``pandas.DataFrame.to_parquet()``.
  Do not mix the two read/write paths for the same dataset.

#### \_\_init\_\_

```python
def __init__(storage_options: dict[str, Any] | None = None) -> None
```

Initialise with optional fsspec storage options.

#### write

```python
def write(url: str, value: pd.DataFrame) -> None
```

Write a DataFrame to ``url`` in Parquet format.

Creates the target directory if absent. Large DataFrames are split
into ``part-{i}.parquet`` files (max ``_MAX_ROWS_PER_FILE`` rows each).

**Arguments**:

- `url` - Destination directory URL (local path or ``scheme://...``).
- `value` - The ``pandas.DataFrame`` to serialise.
  

**Returns**:

  ``None`` — no metadata is needed for the read path.
  

**Raises**:

- ``4 - If the destination directory cannot be created.
- ``5 - If pyarrow or fsspec is not installed.

#### read

```python
def read(url: str, _metadata: Any) -> pd.DataFrame
```

Read a DataFrame from a directory of Parquet files at ``url``.

**Arguments**:

- `url` - Source directory URL written by a previous ``write()`` call.
- `_metadata` - Unused — pass ``None``.
  

**Returns**:

  A ``pandas.DataFrame`` containing all rows from the Parquet files.
  

**Raises**:

- ``0 - If pyarrow or fsspec is not installed.

