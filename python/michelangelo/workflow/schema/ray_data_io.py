"""Ray Data I/O configuration dataclasses shared across workflow tasks.

These classes configure how Ray Data reads and iterates over training datasets.
They are shared across multiple workflow tasks (e.g. ``tabular_trainer``,
future ``llm_trainer``) and are kept separate from task-specific schemas.
"""

from __future__ import annotations

from dataclasses import dataclass

__all__ = [
    "BatchIterConfig",
    "DataloadingConfig",
    "ParquetReadConfig",
    "RayDataContextConfig",
    "WriteConfig",
]


@dataclass
class ParquetReadConfig:
    """Subset of ``ray.data.read_parquet`` kwargs forwarded at read time.

    This is a curated subset of the full ``ray.data.read_parquet`` API —
    resource knobs and schema hints only. Fields that overlap with the
    tabular_trainer's own column management (``columns``, ``paths``) and
    Ray-version-specific placement logic are intentionally omitted. OSS
    pins a single Ray version so the internal ``<2.50`` branch is unused.

    Column projection is derived automatically from ``input_columns``,
    ``labels``, and ``metadata_columns`` — do not include ``columns`` here.

    See: https://docs.ray.io/en/latest/data/api/input_output.html#ray.data.read_parquet

    Attributes:
        num_cpus: CPUs to reserve per parallel read worker.
        num_gpus: GPUs to reserve per parallel read worker.
        memory: Heap memory in bytes per read worker.
        concurrency: Maximum number of concurrent Ray read tasks.
        override_num_blocks: Override the number of output blocks.
        shuffle: Set to ``"files"`` to randomly shuffle input file order.
        tensor_column_schema: Column name → ``{"dtype": ..., "shape": ...}``
            for serialised tensor columns.
        arrow_parquet_args: Additional kwargs forwarded to PyArrow's reader.

    Example:
        >>> ParquetReadConfig(num_cpus=2, shuffle="files")
        ParquetReadConfig(num_cpus=2, ...)
    """

    num_cpus: float | None = None
    num_gpus: float | None = None
    memory: int | None = None
    concurrency: int | None = None
    override_num_blocks: int | None = None
    shuffle: str | None = None
    tensor_column_schema: dict | None = None
    arrow_parquet_args: dict | None = None


@dataclass
class BatchIterConfig:
    """Configuration for ``ray.data.Dataset.iter_torch_batches``.

    Attributes:
        batch_size: Number of samples per batch. Required.
        num_shuffle_batches: Number of batches to buffer for local
            shuffling. ``0`` disables local shuffle.
        collate_fn: Dotted import path to a collate function. When set,
            the function is resolved at training time via ``get_module_attr``
            and passed as ``collate_fn`` to ``iter_torch_batches``.

    Example:
        >>> BatchIterConfig(batch_size=64, num_shuffle_batches=4)
        BatchIterConfig(batch_size=64, num_shuffle_batches=4, collate_fn=None)
    """

    batch_size: int
    num_shuffle_batches: int = 0
    collate_fn: str | None = None


@dataclass
class DataloadingConfig:
    """Container for Ray Data read and batch iteration settings.

    Attributes:
        parquet_read_config: kwargs forwarded to ``ray.data.read_parquet``.
        batch_iter_config: Batch size, shuffle, and collate settings.

    Example:
        >>> DataloadingConfig(batch_iter_config=BatchIterConfig(batch_size=32))
        DataloadingConfig(...)
    """

    parquet_read_config: ParquetReadConfig | None = None
    batch_iter_config: BatchIterConfig | None = None


@dataclass
class RayDataContextConfig:
    """Ray ``DataContext`` tuning for workflow tasks that use Ray Data I/O.

    Forwarded to
    :func:`~michelangelo.uniflow.plugins.ray.data_context.set_ray_data_context`.

    Attributes:
        min_block_size: Target minimum Ray Data block size in bytes. ``None``
            uses Ray's default.
        max_block_size: Target maximum Ray Data block size in bytes. ``None``
            uses Ray's default.
        retried_io_errors: Extra error-message substrings appended to Ray's
            ``retried_io_errors``. ``None`` uses the built-in patterns in
            :data:`~michelangelo.uniflow.plugins.ray.data_context.RETRIED_IO_ERRORS`;
            pass ``[]`` to skip adding extras beyond Ray's defaults.
        object_store_memory_limit: Upper bound in bytes on the object store
            memory the streaming executor may use for buffered (pending)
            blocks across all operators. ``None`` leaves Ray's default
            (unbounded, i.e. capped only by the physical object store).
        wait_for_min_actors_s: Seconds the executor blocks for an actor-pool
            operator's actors to finish provisioning before it begins
            scheduling upstream read tasks. ``None`` keeps Ray's default (no
            wait).

    Example:
        >>> RayDataContextConfig(min_block_size=32 * 1024 * 1024)
        RayDataContextConfig(min_block_size=33554432, ...)
    """

    min_block_size: int | None = None
    max_block_size: int | None = None
    retried_io_errors: list[str] | None = None
    object_store_memory_limit: int | None = None
    wait_for_min_actors_s: int | None = None


@dataclass
class WriteConfig:
    """Output file sizing for Ray Dataset write operations.

    These parameters are format-agnostic (supported by ``write_parquet``,
    ``write_csv``, ``write_json``, etc.) via Ray's ``FileBasedDatasink``.

    Attributes:
        max_rows_per_file: Max rows per output file. Caps individual file
            size by splitting large blocks, preventing oversized files in
            downstream steps. ``None`` uses Ray's default.
        min_rows_per_file: Min rows per output file. Buffers small blocks
            into fewer, larger files, reducing write task overhead and
            storage metadata calls. ``None`` uses Ray's default.

    Example:
        >>> WriteConfig(max_rows_per_file=1_000_000)
        WriteConfig(max_rows_per_file=1000000, min_rows_per_file=None)
    """

    max_rows_per_file: int | None = None
    min_rows_per_file: int | None = None
