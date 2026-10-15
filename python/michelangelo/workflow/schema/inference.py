"""Configuration dataclasses for the ``tabular_inference`` workflow task.

Plain ``@dataclass`` with ``__post_init__`` validation and no Pydantic
dependency, matching ``michelangelo.workflow.schema.evaluator`` and
``michelangelo.workflow.schema.tabular_trainer``.

Field names are a wire format: they appear in serialized pipeline configs, so
renaming a field breaks existing configs. Unknown keys are rejected by the
UniFlow codec with a ``TypeError``, which is the intended loud failure for
options this package does not support.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.ray_data_io import (
    ParquetReadConfig,
    RayDataContextConfig,
    WriteConfig,
)

__all__ = [
    "BatchOptions",
    "ModelReference",
    "OfflineInferenceConfig",
    "PaddingOptions",
    "ParquetReadConfig",
    "RayDataContextConfig",
    "TabularInferenceConfig",
    "WriteConfig",
]


def _require_positive(name: str, value: float | None) -> None:
    """Raise ``ConfigurationError`` if ``value`` is set and not positive."""
    if value is not None and value <= 0:
        raise ConfigurationError(f"batch_options.{name} must be > 0, got {value!r}.")


@dataclass
class ModelReference:
    """Identifies a registered model to score with.

    Attributes:
        name: Model name in the model registry.
        revision_id: Revision of the model. ``None`` selects the latest
            revision the registry reports.

    Raises:
        ConfigurationError: If ``name`` is empty or ``revision_id`` is negative.

    Example:
        >>> ModelReference(name="housing_model", revision_id=3).name
        'housing_model'
    """

    name: str
    revision_id: int | None = None

    def __post_init__(self) -> None:
        """Validate the reference."""
        if not self.name:
            raise ConfigurationError("model.name must be a non-empty string.")
        if self.revision_id is not None and self.revision_id < 0:
            raise ConfigurationError(
                f"model.revision_id must be >= 0, got {self.revision_id!r}."
            )


@dataclass
class BatchOptions:
    """Ray processing options for batch inference.

    Distinct from ``native_transform.BatchOptions``, which has fewer fields.

    Attributes:
        batch_size: The desired number of rows in each batch. ``None`` lets Ray
            choose, which depends on the Ray version, so set it explicitly for
            reproducible batching. Ray requires it whenever ``num_gpus`` is set.
        num_gpus: GPUs to reserve for each parallel map worker. Fractional
            values are allowed, e.g. ``0.125``. ``None`` reserves none.
        concurrency: The number of workers in the actor pool. ``None`` lets Ray
            size the pool.
        num_cpus: CPUs to reserve for each parallel map worker. Values above
            ``1`` also enable a thread pool for the Arrow-to-NumPy conversion.
            Specifying both ``num_cpus`` and ``num_gpus`` is experimental in
            Ray and may cause scheduling or stability issues.
        max_concurrency: Ray actor ``max_concurrency`` for the map workers.
            Only forwarded when set.
        max_tasks_in_flight_per_actor: Maximum queued tasks per actor in
            Ray's actor pool. ``None`` leaves Ray's default.
        max_restarts: Times Ray restarts a dead inference actor before failing
            the job. ``None`` leaves Ray's default; ``-1`` restarts without
            limit.
        max_task_retries: Times an in-flight block is resubmitted after its
            actor dies. Only effective when ``max_restarts`` is not ``0``.
            ``None`` leaves Ray's default.

    Raises:
        ConfigurationError: If a size or count field is not positive, or a
            retry field is below ``-1``.

    Example:
        >>> BatchOptions(batch_size=4096, num_gpus=0.25).batch_size
        4096
    """

    batch_size: int | None = None
    num_gpus: float | None = None
    concurrency: int | None = None
    num_cpus: int | None = None
    max_concurrency: int | None = None
    max_tasks_in_flight_per_actor: int | None = None
    max_restarts: int | None = None
    max_task_retries: int | None = None

    def __post_init__(self) -> None:
        """Validate sizes and counts."""
        for name in (
            "batch_size",
            "num_gpus",
            "concurrency",
            "num_cpus",
            "max_concurrency",
            "max_tasks_in_flight_per_actor",
        ):
            _require_positive(name, getattr(self, name))
        for name in ("max_restarts", "max_task_retries"):
            value = getattr(self, name)
            if value is not None and value < -1:
                raise ConfigurationError(
                    f"batch_options.{name} must be >= -1, got {value!r}."
                )


@dataclass
class PaddingOptions:
    """Padding values for ragged input columns.

    Attributes:
        column_padding_values: Pad value per column name. A column that is not
            listed is padded with the sentinel for its dtype (see
            ``sentinel_for_numpy_dtype``), not with ``0``. A value must match
            the dtype of its column.

    Raises:
        ConfigurationError: If a key is not a string.

    Example:
        >>> PaddingOptions(column_padding_values={"clicks": -1}).column_padding_values
        {'clicks': -1}
    """

    column_padding_values: dict[str, Any] | None = None

    def __post_init__(self) -> None:
        """Validate the column names."""
        if self.column_padding_values is None:
            return
        bad = [k for k in self.column_padding_values if not isinstance(k, str)]
        if bad:
            raise ConfigurationError(
                f"padding_options.column_padding_values keys must be strings, "
                f"got {bad!r}."
            )


@dataclass
class OfflineInferenceConfig:
    """Scoring settings for one model in ``tabular_inference``.

    Attributes:
        model: The registered model to load. ``None`` scores the model passed
            to the task.
        input_columns: Columns fed to the model, in the order the model's
            input schema expects. ``None`` uses the model's input schema.
        batch_options: Ray processing options.
        padding_options: Padding values for ragged input columns.
        columns_to_keep: Restricts the output to exactly these input columns,
            plus the prediction and metadata columns, which are always kept.
            ``None`` keeps every input column. Label columns are not kept
            automatically: list them here if a downstream task, such as the
            evaluator, needs them, or they are dropped from the output.
        backend: Dotted path (``"package.module.factory"``) of a callable that
            takes a ``BackendContext`` and returns a ``ModelBackend``. ``None``
            picks a backend from the model package layout. A custom backend
            loads from a model package, so it cannot score a model object that
            was passed to the task in memory.

    Example:
        >>> cfg = OfflineInferenceConfig(columns_to_keep=["label"])
        >>> cfg.batch_options.batch_size is None
        True
    """

    model: ModelReference | None = None
    input_columns: list[str] | None = None
    batch_options: BatchOptions = field(default_factory=BatchOptions)
    padding_options: PaddingOptions = field(default_factory=PaddingOptions)
    columns_to_keep: list[str] | None = None
    backend: str | None = None


@dataclass
class TabularInferenceConfig:
    """Configuration for the ``tabular_inference`` workflow task.

    The field is named ``local_models`` because the models run in the task's own
    Ray cluster, as opposed to being called over the network.

    Attributes:
        local_models: One entry per model to score. Several entries produce the
            union of their predictions. A model looked up in the registry adds
            ``model_name`` and ``model_version`` columns so the rows can be told
            apart; a model passed to the task directly has no name to record. An
            empty or ``None`` list returns the inputs unchanged.
        parquet_read_config: kwargs forwarded to ``ray.data.read_parquet`` when
            loading the input datasets.
        write_config: Output write options for the scored datasets.
        ray_data_context: Ray Data block sizing and I/O retry tuning.

    Example:
        >>> TabularInferenceConfig(local_models=[OfflineInferenceConfig()])
        ... # doctest: +ELLIPSIS
        TabularInferenceConfig(local_models=[OfflineInferenceConfig(...)], ...)
    """

    local_models: list[OfflineInferenceConfig] | None = None
    parquet_read_config: ParquetReadConfig | None = None
    write_config: WriteConfig | None = None
    ray_data_context: RayDataContextConfig | None = None
