"""Ray Data I/O configuration dataclasses shared across workflow tasks.

These classes configure how Ray Data reads and iterates over training datasets.
They are shared across multiple workflow tasks (e.g. ``tabular_trainer``,
future ``llm_trainer``) and are kept separate from task-specific schemas.
"""

from __future__ import annotations

from dataclasses import dataclass

from michelangelo.workflow.schema.exceptions import ConfigurationError

__all__ = [
    "BatchIterConfig",
    "DataloadingConfig",
    "ParquetReadConfig",
    "RayDataContextConfig",
    "WriteConfig",
]


def _require_positive_int(field_name: str, value: object) -> None:
    """Raise ``ConfigurationError`` unless ``value`` is a positive integer.

    Args:
        field_name: Name used in the error message.
        value: Value to check. ``bool`` is rejected even though it subclasses
            ``int``.

    Raises:
        ConfigurationError: If ``value`` is not an ``int`` greater than zero.
    """
    if isinstance(value, bool) or not isinstance(value, int) or value < 1:
        raise ConfigurationError(
            f"{field_name} must be a positive integer, got {value!r}."
        )


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
        override_num_blocks: Override the number of output blocks. Applied to
            every dataset read with this config. Mutually exclusive with
            ``override_num_blocks_per_dataset``.
        override_num_blocks_per_dataset: Per-dataset ``override_num_blocks``,
            keyed by dataset name (e.g. ``"train"``, ``"validation"``). Use
            this when datasets have different file counts so that a value
            tuned for one dataset is not applied to the others. A dataset
            with no entry uses Ray's default. Mutually exclusive with
            ``override_num_blocks``. Applied by the tabular native-transform
            task only; the tabular trainer does not apply it (a warning is
            logged if it is set there).
        shuffle: Set to ``"files"`` to randomly shuffle input file order.
        tensor_column_schema: Column name → ``{"dtype": ..., "shape": ...}``
            for serialised tensor columns.
        arrow_parquet_args: Additional kwargs forwarded to PyArrow's reader.

    Raises:
        ConfigurationError: If both ``override_num_blocks`` and
            ``override_num_blocks_per_dataset`` are set, if
            ``override_num_blocks_per_dataset`` is not a non-empty ``dict`` with
            ``str`` keys, or if any block count is not a positive integer.

    Example:
        ``ParquetReadConfig(num_cpus=2, shuffle="files")`` reserves 2 CPUs
        per read worker and shuffles input file order.
        ``ParquetReadConfig(override_num_blocks_per_dataset={"train": 64,
        "validation": 8})`` uses 64 read blocks for ``train`` and 8 for
        ``validation``.
    """

    num_cpus: float | None = None
    num_gpus: float | None = None
    memory: int | None = None
    concurrency: int | None = None
    override_num_blocks: int | None = None
    override_num_blocks_per_dataset: dict[str, int] | None = None
    shuffle: str | None = None
    tensor_column_schema: dict | None = None
    arrow_parquet_args: dict | None = None

    def __post_init__(self) -> None:
        """Validate the block-count overrides."""
        if (
            self.override_num_blocks is not None
            and self.override_num_blocks_per_dataset is not None
        ):
            raise ConfigurationError(
                "Set at most one of 'override_num_blocks' and "
                "'override_num_blocks_per_dataset'."
            )
        if self.override_num_blocks is not None:
            _require_positive_int("override_num_blocks", self.override_num_blocks)
        per_dataset = self.override_num_blocks_per_dataset
        if per_dataset is not None:
            if not isinstance(per_dataset, dict) or not per_dataset:
                raise ConfigurationError(
                    "override_num_blocks_per_dataset must be a non-empty dict "
                    f"mapping dataset name to block count, got {per_dataset!r}."
                )
            for name, value in per_dataset.items():
                if not isinstance(name, str):
                    raise ConfigurationError(
                        "override_num_blocks_per_dataset keys must be dataset "
                        f"names (str), got {name!r}."
                    )
                _require_positive_int(
                    f"override_num_blocks_per_dataset[{name!r}]", value
                )


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
        max_blocks_in_streaming_gen_buffer: Maps to Ray's
            ``DataContext._max_num_blocks_in_streaming_gen_buffer`` (Ray's
            default is 2). Ray's resource allocator reserves, per running
            task, this many average-sized output blocks against the
            operator's object-store budget, so with large blocks an operator
            can exhaust its budget well below its configured concurrency.
            Lowering it (e.g. to ``1``) reduces that reservation and admits
            more tasks, at the cost of a task blocking sooner once its
            un-consumed output blocks reach the limit. This is a private Ray
            attribute: if the installed Ray does not expose it, the setting
            is ignored and a warning is logged. ``None`` keeps Ray's default.

    Raises:
        ConfigurationError: If ``max_blocks_in_streaming_gen_buffer`` is set
            to something other than a positive integer.

    Example:
        ``RayDataContextConfig(min_block_size=32 * 1024 * 1024)`` sets a
        32 MiB target minimum block size, leaving every other setting at
        Ray's default.
    """

    min_block_size: int | None = None
    max_block_size: int | None = None
    retried_io_errors: list[str] | None = None
    object_store_memory_limit: int | None = None
    wait_for_min_actors_s: int | None = None
    max_blocks_in_streaming_gen_buffer: int | None = None

    def __post_init__(self) -> None:
        """Validate ``max_blocks_in_streaming_gen_buffer``."""
        if self.max_blocks_in_streaming_gen_buffer is not None:
            _require_positive_int(
                "max_blocks_in_streaming_gen_buffer",
                self.max_blocks_in_streaming_gen_buffer,
            )


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
        concurrency: Maximum number of write tasks Ray runs concurrently.
            Ray leaves this uncapped by default, so the write stage can claim
            any CPU the scheduler grants it even though write tasks are often
            short and mostly waiting on input. Ray bills one CPU per running
            task, so the surplus can starve upstream operators such as the
            reader. Size it from the rate at which blocks arrive and the
            per-task execution time (both reported by ``Dataset.stats()``),
            not from the number of available CPUs. ``None`` leaves the write
            stage uncapped.

    Raises:
        ConfigurationError: If ``concurrency`` is set to something other than
            a positive integer.

    Example:
        >>> WriteConfig(max_rows_per_file=1_000_000, concurrency=4)
        WriteConfig(max_rows_per_file=1000000, min_rows_per_file=None, concurrency=4)
    """

    max_rows_per_file: int | None = None
    min_rows_per_file: int | None = None
    concurrency: int | None = None

    def __post_init__(self) -> None:
        """Validate ``concurrency``."""
        if self.concurrency is not None:
            _require_positive_int("concurrency", self.concurrency)
