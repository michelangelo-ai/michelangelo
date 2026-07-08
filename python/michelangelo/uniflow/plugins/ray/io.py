"""I/O handlers for Ray datasets in Uniflow workflows.

Supports fsspec and PyArrow filesystem backends. Adds production-hardened data
quality filtering (skip zero-byte / empty parquet files) and a Polars fallback
for the PyArrow nested-data bug (https://github.com/ray-project/ray/issues/61675).

Filesystem backend is selected via ``UF_PLUGIN_RAY_USE_FSSPEC``:

- ``"1"`` — fsspec (flexible: local, S3, GCS, etc.). PyArrow accepts fsspec
  filesystems directly and wraps them transparently via ``FSSpecHandler``.
- ``"0"`` (default) — native PyArrow filesystem (S3 with MinIO credential support)
"""

from __future__ import annotations

import concurrent.futures
import logging
import os
from typing import Any

import fsspec.core
import ray
from ray.data import Dataset, ReadTask
from ray.data.block import BlockMetadata

from michelangelo.uniflow.core.io_registry import IO

_logger = logging.getLogger(__name__)

UF_PLUGIN_RAY_USE_FSSPEC = "UF_PLUGIN_RAY_USE_FSSPEC"
"""Environment variable: set to ``"1"`` to use fsspec instead of PyArrow."""

UF_PLUGIN_RAY_FILTER_WORKERS = "UF_PLUGIN_RAY_FILTER_WORKERS"
"""Environment variable: maximum parallel workers for empty-file filtering.

Default: 64.
"""

_FILTER_WORKERS_DEFAULT = 64

# Substring of the PyArrow error raised on nested list/struct columns in Ray workers.
# Kept as a fallback alongside isinstance(exc, ArrowNotImplementedError) in case
# PyArrow refactors its exception hierarchy in a future release.
# https://github.com/ray-project/ray/issues/61675
_NESTED_CHUNKED_ARRAY_ERROR = (
    "Nested data conversions not implemented for chunked array"
)

__all__ = ["RayDatasetIO"]


def _has_row_groups(path: str, fs: Any) -> bool:
    """Return True if *path* is a parquet file with at least one row group."""
    import pyarrow.parquet as pq

    try:
        return pq.read_metadata(path, filesystem=fs).num_row_groups > 0
    except OSError:
        raise
    except Exception:
        _logger.warning("Skipping unreadable parquet file: %s", path, exc_info=True)
        return False


def _chunk_list(lst: list[str], num_chunks: int) -> list[list[str]]:
    """Split *lst* into *num_chunks* sublists (as evenly as possible)."""
    if num_chunks <= 0:
        num_chunks = 1
    n = len(lst)
    if n == 0:
        return []
    size = max(1, (n + num_chunks - 1) // num_chunks)
    return [lst[i : i + size] for i in range(0, n, size)]


class _ParquetPolarsDatasource:
    """Polars-based parquet reader for Ray — fallback for the PyArrow nested-data bug.

    Triggered automatically by ``RayDatasetIO.read()`` when nested list/struct
    columns cause ``ray.data.read_parquet`` to raise ``ArrowNotImplementedError``.
    Reads each file via Polars (which handles nested types correctly), converts to
    an Arrow table, and yields it as a Ray block.

    See: https://github.com/ray-project/ray/issues/61675
    """

    def __init__(self, url: str, paths: list[str]) -> None:
        self._url = url
        self._paths = paths

    def get_read_tasks(self, parallelism: int) -> list[Any]:
        tasks = []
        for chunk in _chunk_list(self._paths, max(1, parallelism)):
            url, paths = self._url, chunk

            def read_fn(_url: str = url, _paths: list[str] = paths):
                import fsspec.core

                try:
                    import polars as pl
                except ImportError as exc:
                    raise ImportError(
                        "Polars is required for the PyArrow nested-data fallback. "
                        "Install it with: pip install michelangelo[ray-polars]"
                    ) from exc

                fs, _ = fsspec.core.url_to_fs(_url)
                for path in _paths:
                    with fs.open(path, "rb") as f:
                        yield pl.read_parquet(f).to_arrow()

            tasks.append(
                ReadTask(
                    read_fn,
                    BlockMetadata(
                        num_rows=None,
                        size_bytes=None,
                        schema=None,
                        input_files=paths,
                        exec_stats=None,
                    ),
                )
            )
        return tasks


class RayDatasetIO(IO[Dataset]):
    """I/O handler for Ray Dataset objects stored as Parquet.

    On **write**: delegates to ``Dataset.write_parquet`` with the configured filesystem.

    On **read**:

    1. ``filter_empty_data()`` lists all parquet files, discards zero-byte files,
       and parallel-checks remaining files for non-empty row groups.
    2. ``ray.data.read_parquet`` reads the survivors.
    3. If PyArrow raises ``ArrowNotImplementedError`` on nested columns
       (ray-project/ray#61675), the Polars fallback ``_ParquetPolarsDatasource``
       retries the read. **Requires ``polars`` to be installed**
       (``pip install michelangelo[ray-polars]``).

    Raises:
        FileNotFoundError: If no parquet files are found at *url* on read.

    Example:
        >>> import ray, tempfile, pandas as pd
        >>> ds = ray.data.from_pandas(pd.DataFrame([{"x": 1}]))
        >>> io = RayDatasetIO()
        >>> dest = tempfile.mkdtemp()
        >>> io.write(dest, ds)
        >>> result = io.read(dest, None)
        >>> result.count()
        1
    """

    def write(self, url: str, value: Dataset) -> None:
        """Write *value* to *url* as Parquet files.

        Args:
            url: Destination directory path or URL (local, ``s3://``, etc.).
                Ray writes multiple shard files under this directory.
            value: Ray Dataset to write.

        Returns:
            ``None`` — no metadata needed for the read path.
        """
        fs, root = _fs_path(url)
        value.write_parquet(root, filesystem=fs)
        _logger.info("RayDatasetIO: wrote dataset to '%s'.", url)
        return None

    def read(self, url: str, _metadata: Any | None) -> Dataset:
        """Read a Ray Dataset from *url*, skipping empty parquet files.

        Args:
            url: Source directory path or URL.
            _metadata: Unused; pass ``None``.

        Returns:
            Ray Dataset loaded from Parquet shards under *url*.

        Raises:
            FileNotFoundError: If no non-empty parquet files exist at *url*.
        """
        fs, _ = _fs_path(url)
        paths = RayDatasetIO.filter_empty_data(url)
        if not paths:
            raise FileNotFoundError(f"RayDatasetIO: no parquet files found at '{url}'.")
        try:
            ds = ray.data.read_parquet(
                paths, filesystem=fs, file_extensions=["parquet"]
            )
            _logger.info("RayDatasetIO: read %d file(s) from '%s'.", len(paths), url)
            return ds
        except Exception as exc:
            import pyarrow.lib

            if isinstance(exc, pyarrow.lib.ArrowNotImplementedError) or (
                _NESTED_CHUNKED_ARRAY_ERROR in str(exc)
            ):
                _logger.info(
                    "RayDatasetIO: PyArrow nested-data error, falling back to Polars."
                )
                return RayDatasetIO._read_parquet_fallback(url, paths)
            raise

    @staticmethod
    def filter_empty_data(url: str) -> list[str]:
        """Return non-empty parquet file paths under *url*.

        Steps:

        1. ``fs.find(detail=True)`` — bulk listing (single round-trip).
        2. Discard zero-byte files immediately.
        3. Parallel-check remaining files for row groups (up to
           ``UF_PLUGIN_RAY_FILTER_WORKERS`` workers, default 64).

        Args:
            url: Directory path or URL containing parquet files.

        Returns:
            List of paths that contain at least one parquet row group.
        """
        fsspec_fs, path = fsspec.core.url_to_fs(url)
        file_info = fsspec_fs.find(path, detail=True)
        parquet_files = {
            p: info for p, info in file_info.items() if p.endswith(".parquet")
        }

        if not parquet_files:
            _logger.warning("No parquet files found at %s", url)
            return []

        candidates = [p for p, info in parquet_files.items() if info.get("size", 0) > 0]
        skipped = len(parquet_files) - len(candidates)
        _logger.info(
            "Found %d parquet file(s) at %s, %d zero-byte skipped, %d to check.",
            len(parquet_files),
            url,
            skipped,
            len(candidates),
        )

        if not candidates:
            return []

        max_workers = min(
            int(os.environ.get(UF_PLUGIN_RAY_FILTER_WORKERS, _FILTER_WORKERS_DEFAULT)),
            len(candidates),
        )
        with concurrent.futures.ThreadPoolExecutor(max_workers=max_workers) as pool:
            has_data = list(
                pool.map(lambda f: _has_row_groups(f, fsspec_fs), candidates)
            )
        non_empty = [f for f, ok in zip(candidates, has_data) if ok]
        _logger.info(
            "Row-group check: %d have data, %d empty.",
            len(non_empty),
            len(candidates) - len(non_empty),
        )
        return non_empty

    @staticmethod
    def _read_parquet_fallback(url: str, paths: list[str]) -> Dataset:
        """Read *paths* via Polars — fallback for the PyArrow nested-data bug."""
        return ray.data.read_datasource(_ParquetPolarsDatasource(url, paths))


def _fs_path(url: str) -> tuple[Any, str]:
    """Return a (filesystem, path) tuple for *url*.

    When ``UF_PLUGIN_RAY_USE_FSSPEC=1``, returns an fsspec filesystem. PyArrow
    accepts fsspec filesystems directly and wraps them transparently at the C++
    layer via ``FSSpecHandler`` — no manual wrapping required.
    """
    if os.environ.get(UF_PLUGIN_RAY_USE_FSSPEC, "0") == "1":
        return fsspec.core.url_to_fs(url)
    return resolve_fs(url.split("://")[0]), url


def resolve_fs(protocol: str) -> Any:
    """Return a PyArrow filesystem for *protocol*, or ``None`` for local paths.

    Args:
        protocol: URL scheme extracted from the target URL (e.g. ``"s3"``).

    Returns:
        A ``pyarrow.fs.S3FileSystem`` for S3/MinIO, ``None`` otherwise.
    """
    if protocol == "s3":
        import pyarrow.fs

        return pyarrow.fs.S3FileSystem(
            access_key=os.getenv("AWS_ACCESS_KEY_ID"),
            secret_key=os.getenv("AWS_SECRET_ACCESS_KEY"),
            endpoint_override=os.getenv("AWS_ENDPOINT_URL"),
            allow_bucket_creation=os.getenv("S3_ALLOW_BUCKET_CREATION", "").lower()
            == "true",
        )
    return None
