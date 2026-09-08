---
sidebar_label: io
title: michelangelo.uniflow.plugins.spark.io
---

I/O handlers for Spark DataFrames in Uniflow workflows.

This module provides I/O functionality for reading and writing Spark DataFrames in
Uniflow workflows. It handles S3A filesystem configuration for MinIO compatibility
and supports Parquet format for data persistence. S3A credentials and endpoint are
read from the standard `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and
`AWS_ENDPOINT_URL` environment variables; path-style access is enabled automatically
for MinIO compatibility.

**Example**:

```python
from michelangelo.uniflow.plugins.spark.io import SparkIO
from pyspark.sql import SparkSession

spark = SparkSession.builder.getOrCreate()
df = spark.createDataFrame([(1, "a"), (2, "b")], ["id", "label"])

io_handler = SparkIO()
io_handler.write("s3://bucket/data.parquet", df)
loaded_df = io_handler.read("s3://bucket/data.parquet", None)
```

#### read\_data

```python
def read_data(url: str) -> DataFrame
```

Read a Spark DataFrame from a Parquet file.

**Arguments**:

- `url` - The URL or path to read from. Supports local paths and S3 URLs.

**Returns**:

  The loaded Spark DataFrame.

## SparkIO Objects

```python
class SparkIO(IO[DataFrame])
```

I/O handler for Spark DataFrame objects.

This class provides read and write operations for Spark DataFrames, storing them
in Parquet format. It supports local filesystem paths and S3 URLs via S3A protocol.

The implementation expands tilde (~) paths and uses the active Spark session for
all I/O operations.

#### write

```python
def write(url: str, value: DataFrame) -> Optional[Any]
```

Write a Spark DataFrame to the specified URL in Parquet format.

**Arguments**:

- `url` - Target URL where the DataFrame should be written. Supports local paths
  (including ~-prefixed paths) and S3 URLs.
- `value` - The Spark DataFrame to write.

**Returns**:

  None. This implementation does not return metadata.

#### read

```python
def read(url: str, _metadata) -> DataFrame
```

Read a Spark DataFrame from the specified URL.

**Arguments**:

- `url` - Source URL from which to read the DataFrame. Supports local paths
  (including ~-prefixed paths) and S3 URLs.
- `_metadata` - Optional metadata from write operation. Currently unused.

**Returns**:

  The loaded Spark DataFrame.

#### write\_data

```python
@staticmethod
def write_data(url: str, data: DataFrame)
```

Write DataFrame to Parquet format at the given URL.

**Arguments**:

- `url` - Target URL for writing. Tilde paths are expanded.
- `data` - The Spark DataFrame to write.

#### read\_data

```python
@staticmethod
def read_data(url: str) -> DataFrame
```

Read DataFrame from Parquet format at the given URL.

**Arguments**:

- `url` - Source URL for reading. Tilde paths are expanded.

**Returns**:

  The loaded Spark DataFrame.
