"""I/O handler for pandas DataFrames in Uniflow workflows.

Reads and writes DataFrames in Parquet format using PyArrow with zstd
compression, supporting local and remote filesystems via fsspec. Writes
are partitioned into part files (max 2 M rows per file, 1 M rows per
group) so large DataFrames can be read back in parallel by Spark or Ray.

PyArrow and fsspec are imported lazily inside write() and read() to avoid
circular-import issues when pandas itself is being initialised.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

from michelangelo.uniflow.core.io_registry import IO

if TYPE_CHECKING:
    import pandas as pd

_logger = logging.getLogger(__name__)

__all__ = ["PandasIO"]

_MAX_ROWS_PER_FILE = 2_000_000
_MAX_ROWS_PER_GROUP = 1_000_000


class PandasIO(IO["pd.DataFrame"]):
    """I/O handler for ``pandas.DataFrame`` objects.

    Serialises DataFrames to Parquet via PyArrow (zstd compression) and
    reads them back. Supports any filesystem accessible via fsspec —
    local paths, ``s3://``, ``gcs://``, ``hdfs://``, etc.

    Args:
        storage_options: Optional fsspec storage options forwarded to
            ``fsspec.core.url_to_fs``. Use this to pass credentials or
            endpoint overrides for remote filesystems (e.g.
            ``{"key": "...", "secret": "..."}`` for S3).

    Example::

        import pandas as pd
        from michelangelo.uniflow.plugins.pandas import PandasIO

        io = PandasIO()
        df = pd.DataFrame([{"x": 1}, {"x": 2}])
        io.write("/tmp/mydata", df)
        loaded = io.read("/tmp/mydata", None)
        assert len(loaded) == 2

    .. note::
        ``PandasIO`` writes a **directory** of ``part-*.parquet`` files, not a
        single file. This differs from ``LocalFileSink`` (workflow pusher),
        which writes a single ``data.parquet`` via ``pandas.DataFrame.to_parquet()``.
        Do not mix the two read/write paths for the same dataset.
    """

    def __init__(self, storage_options: dict[str, Any] | None = None) -> None:
        """Initialise with optional fsspec storage options."""
        self._storage_options = storage_options or {}

    def write(self, url: str, value: pd.DataFrame) -> None:
        """Write a DataFrame to ``url`` in Parquet format.

        Creates the target directory if absent. Large DataFrames are split
        into ``part-{i}.parquet`` files (max ``_MAX_ROWS_PER_FILE`` rows each).

        Args:
            url: Destination directory URL (local path or ``scheme://...``).
            value: The ``pandas.DataFrame`` to serialise.

        Returns:
            ``None`` — no metadata is needed for the read path.

        Raises:
            RuntimeError: If the destination directory cannot be created.
            ImportError: If pyarrow or fsspec is not installed.
        """
        import fsspec.core
        import pyarrow as pa
        import pyarrow.dataset as ds

        fs, dir_path = fsspec.core.url_to_fs(url, **self._storage_options)
        _ensure_dir(fs, dir_path)

        table = pa.Table.from_pandas(value)
        fmt = ds.ParquetFileFormat()
        file_options = fmt.make_write_options(compression="zstd")

        ds.write_dataset(
            data=table,
            base_dir=dir_path,
            format=fmt,
            filesystem=fs,
            basename_template="part-{i}.parquet",
            file_options=file_options,
            max_rows_per_file=_MAX_ROWS_PER_FILE,
            max_rows_per_group=_MAX_ROWS_PER_GROUP,
        )
        _logger.info("PandasIO: wrote %d rows to '%s'.", len(value), url)
        return None

    def read(self, url: str, _metadata: Any) -> pd.DataFrame:
        """Read a DataFrame from a directory of Parquet files at ``url``.

        Args:
            url: Source directory URL written by a previous ``write()`` call.
            _metadata: Unused — pass ``None``.

        Returns:
            A ``pandas.DataFrame`` containing all rows from the Parquet files.

        Raises:
            ImportError: If pyarrow or fsspec is not installed.
        """
        import fsspec.core
        import pyarrow.dataset as ds

        fs, dir_path = fsspec.core.url_to_fs(url, **self._storage_options)
        dataset = ds.dataset(source=dir_path, format="parquet", filesystem=fs)
        df = dataset.to_table().to_pandas()
        _logger.info("PandasIO: read %d rows from '%s'.", len(df), url)
        return df


def _ensure_dir(fs: Any, path: str) -> None:
    """Create ``path`` on ``fs`` if it does not already exist.

    The ``AttributeError`` branch handles fsspec filesystem implementations
    that do not expose ``exists()`` (e.g. some custom or legacy backends).
    Those filesystems are probed via ``ls()`` and created via ``mkdir()``.
    """
    try:
        if not fs.exists(path):
            fs.mkdir(path, create_parents=True)
    except AttributeError:
        # Filesystem does not implement exists() — probe with ls() instead.
        try:
            fs.ls(path)
        except Exception:
            try:
                fs.mkdir(path, create_parents=True)
            except Exception as exc:
                raise RuntimeError(f"Failed to create directory '{path}'") from exc
