"""Configuration dataclasses for the ``tabular_native_transform`` workflow task.

Plain ``@dataclass`` with no Pydantic dependency, matching
``michelangelo.workflow.schema.tabular_trainer`` and
``michelangelo.workflow.schema.assembler``.
"""

from __future__ import annotations

from dataclasses import dataclass, field

from michelangelo.workflow.schema.common import (
    IncrementalTrainingConfig,
    TrainingTypeConfig,
)
from michelangelo.workflow.schema.ray_data_io import (
    ParquetReadConfig,
    RayDataContextConfig,
    WriteConfig,
)

__all__ = [
    "BatchOptions",
    "IncrementalTrainingConfig",
    "ParquetReadConfig",
    "RayDataContextConfig",
    "TabularNativeTransformConfig",
    "TrainingTypeConfig",
    "WriteConfig",
]


@dataclass
class BatchOptions:
    """Ray processing options for ``tabular_native_transform``.

    Attributes:
        batch_size: The desired number of rows in each batch.
        num_gpus: The number of GPUs to reserve for each parallel map
            worker. For example, ``1`` to request 1 GPU, or ``0.125`` for
            fractional allocation.
        concurrency: The number of workers to use concurrently.
        num_cpus: The number of CPUs to reserve for each parallel map
            worker. Specifying both ``num_cpus`` and ``num_gpus`` for map
            tasks is experimental and may result in scheduling or stability
            issues.

    Example:
        >>> BatchOptions(batch_size=20_000, num_gpus=0.25)
        BatchOptions(batch_size=20000, num_gpus=0.25, concurrency=None, num_cpus=None)
    """

    batch_size: int | None = None
    num_gpus: float | None = None
    concurrency: int | None = None
    num_cpus: int | None = None


@dataclass
class TabularNativeTransformConfig:
    """Configuration for the ``tabular_native_transform`` workflow task.

    Attributes:
        transform_spec: Either an inlined transform spec dict, or a string
            file path to a YAML spec file resolved via
            :func:`~michelangelo.workflow.tasks.tabular_native_transform.utils.resolve_data_file_path`.
            May be ``None`` when ``incremental_training`` supplies a
            baseline spec to reuse (``TrainingTypeConfig.INCREMENTAL``).
        batch_options: Batch processing options for Ray operations.
        parquet_read_config: kwargs forwarded to ``ray.data.read_parquet``
            when loading the input datasets. Use this to tune read
            parallelism (e.g. ``override_num_blocks``, ``concurrency``) or
            per-read-worker resources (``num_cpus``, ``num_gpus``,
            ``memory``).
        write_config: Output write config for the transformed datasets.
        ray_data_context: Ray Data block sizing and I/O retry pattern
            tuning.
        incremental_training: Incremental training configuration.

    Example:
        ``TabularNativeTransformConfig(transform_spec={"transform_specs": []})``
        runs with an empty inlined transform spec and every other setting at
        its default.
    """

    transform_spec: str | dict | None = None
    batch_options: BatchOptions = field(default_factory=BatchOptions)
    parquet_read_config: ParquetReadConfig | None = None
    write_config: WriteConfig | None = None
    ray_data_context: RayDataContextConfig | None = None
    incremental_training: IncrementalTrainingConfig | None = None
