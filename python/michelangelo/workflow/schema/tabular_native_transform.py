"""Configuration dataclasses for the ``tabular_native_transform`` workflow task.

Plain ``@dataclass`` with no Pydantic dependency, matching
``michelangelo.workflow.schema.tabular_trainer`` and
``michelangelo.workflow.schema.assembler``.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum

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
    "TrainingType",
    "WriteConfig",
]


class TrainingType(str, Enum):
    """Incremental-training mode for ``tabular_native_transform``.

    Attributes:
        INVALID: Not set; treated the same as a fresh (non-incremental) run.
        BASE: A base run whose fitted transform can later be reused/refit by
            an ``INCREMENTAL`` run.
        INCREMENTAL: Reuse (and optionally selectively refit) a base run's
            fitted transform spec and feature statistics.

    Example:
        >>> TrainingType.INCREMENTAL.value
        'INCREMENTAL'
    """

    INVALID = "INVALID"
    BASE = "BASE"
    INCREMENTAL = "INCREMENTAL"


@dataclass
class IncrementalTrainingConfig:
    """Incremental-training configuration for ``tabular_native_transform``.

    Scoped to only the fields this task uses. Internal's equivalent
    configuration is shared with a model-initializer task and carries extra
    fields (``load_optimizer_weights``, ``fused_model_submodule``) that are
    meaningless here — this config intentionally omits them rather than
    porting unused surface area.

    Attributes:
        training_type: Whether this run is a base run, an incremental run,
            or neither (``INVALID``).
        baseline_model_uri: URI of the base run's raw model package, as
            returned by ``StorageBackend.upload()`` — passed directly to
            ``StorageBackend.download()`` to retrieve
            ``transform_spec.yaml``/``transform_feature_stats.yaml`` from its
            metadata directory. Required when ``training_type ==
            TrainingType.INCREMENTAL``.
        enforce_full_reuse: When ``True`` and ``training_type ==
            TrainingType.INCREMENTAL``, every layer in the (optional)
            inlined ``transform_spec`` must use
            :attr:`~michelangelo.lib.native_transform.torch.transform_layer_spec.TransformerMode.REUSE`
            (or the default ``INVALID``, which behaves as ``REUSE``) — no
            refitting allowed. This is the only supported setting for now.

    Example:
        >>> IncrementalTrainingConfig(
        ...     training_type=TrainingType.INCREMENTAL,
        ...     baseline_model_uri="s3://bucket/models/base-run/",
        ... ).enforce_full_reuse
        True
    """

    training_type: TrainingType = TrainingType.INVALID
    baseline_model_uri: str | None = None
    enforce_full_reuse: bool = True


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
            baseline spec to reuse (``TrainingType.INCREMENTAL``).
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
        >>> TabularNativeTransformConfig(transform_spec={"transform_specs": []})
        TabularNativeTransformConfig(transform_spec={'transform_specs': []}, ...)
    """

    transform_spec: str | dict | None = None
    batch_options: BatchOptions = field(default_factory=BatchOptions)
    parquet_read_config: ParquetReadConfig | None = None
    write_config: WriteConfig | None = None
    ray_data_context: RayDataContextConfig | None = None
    incremental_training: IncrementalTrainingConfig | None = None
