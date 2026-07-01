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
