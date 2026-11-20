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

from dataclasses import dataclass
from typing import Any

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.ray_data_io import (
    ParquetReadConfig,
    RayDataContextConfig,
    WriteConfig,
)
from michelangelo.workflow.schema.sampling import SamplingConfig

__all__ = [
    "BatchOptions",
    "ModelIdentifier",
    "OfflineInferenceConfig",
    "PaddingOptions",
    "ParquetReadConfig",
    "RayDataContextConfig",
    "SamplingConfig",
    "TabularInferenceConfig",
    "WriteConfig",
]


def _require_int(name: str, value: object, minimum: int) -> None:
    """Raise ``ConfigurationError`` unless ``value`` is unset or an int >= ``minimum``.

    ``bool`` is rejected even though it subclasses ``int``.
    """
    if value is None:
        return
    if isinstance(value, bool) or not isinstance(value, int) or value < minimum:
        raise ConfigurationError(
            f"{name} must be an integer >= {minimum}, got {value!r}."
        )


def _require_number(name: str, value: object, minimum: float) -> None:
    """Raise ``ConfigurationError`` unless ``value`` is unset or a number >= minimum."""
    if value is None:
        return
    if (
        isinstance(value, bool)
        or not isinstance(value, (int, float))
        or value < minimum
    ):
        raise ConfigurationError(
            f"{name} must be a number >= {minimum}, got {value!r}."
        )


@dataclass
class ModelIdentifier:
    """Identifies a registered model to score with.

    Attributes:
        name: Model name in the model registry.
        revision_id: Revision of the model. ``None`` selects the latest
            registered version of the model.

    Raises:
        ConfigurationError: If ``name`` is not a non-blank string or
            ``revision_id`` is not an integer of at least ``0``.

    Example:
        >>> ModelIdentifier(name="housing_model", revision_id=3).name
        'housing_model'
    """

    name: str
    revision_id: int | None = None

    def __post_init__(self) -> None:
        """Validate the reference."""
        if not isinstance(self.name, str) or not self.name.strip():
            raise ConfigurationError(
                f"model.name must be a non-blank string, got {self.name!r}."
            )
        _require_int("model.revision_id", self.revision_id, 0)


@dataclass
class BatchOptions:
    """Ray processing options for batch inference.

    Distinct from ``native_transform.BatchOptions``, which has fewer fields.

    Attributes:
        batch_size: The desired number of rows in each batch. ``None`` lets Ray
            choose, which depends on the Ray version, so set it explicitly for
            reproducible batching. Required whenever ``num_gpus`` is set.
        num_gpus: GPUs to reserve for each parallel map worker. Fractional
            values are allowed, e.g. ``0.125``. ``0`` and ``None`` both reserve
            none.
        concurrency: The number of workers in the actor pool. ``None`` lets Ray
            size the pool.
        num_cpus: CPUs to reserve for each parallel map worker. Fractional
            values are allowed. Values of ``2`` or more also enable a thread
            pool, of ``int(num_cpus)`` threads, for the Arrow-to-NumPy
            conversion.
            Specifying both ``num_cpus`` and ``num_gpus`` is experimental in
            Ray and may cause scheduling or stability issues. ``0`` and
            ``None`` both reserve none.
        max_concurrency: Ray actor ``max_concurrency`` for the map workers.
            Only forwarded when set.
        max_tasks_in_flight_per_actor: Maximum queued tasks per actor in
            Ray's actor pool. ``None`` leaves Ray's default.
        max_restarts: Times Ray restarts a dead inference actor over its
            lifetime before failing the job, so a crashed worker (an
            out-of-memory kill, a transient GPU fault) does not fail a long
            run. ``None`` keeps Ray Data's default of restarting without limit;
            ``0`` fails the job on the first actor death.
        max_task_retries: Times a block is resubmitted after its actor dies,
            so a block that crashes its actor every time fails the job instead
            of retrying forever. Only effective when ``max_restarts`` is not
            ``0``. ``None`` means ``3``; ``-1`` retries without limit.

    Raises:
        ConfigurationError: If ``batch_size``, ``concurrency``,
            ``max_concurrency`` or ``max_tasks_in_flight_per_actor`` is not a
            positive integer, ``num_gpus`` or ``num_cpus`` is not a number of
            at least ``0``, a retry field is not an integer of at least
            ``-1``, or ``num_gpus`` is set without ``batch_size``.

    Example:
        >>> BatchOptions(batch_size=4096, num_gpus=0.25).batch_size
        4096
    """

    batch_size: int | None = None
    num_gpus: float | None = None
    concurrency: int | None = None
    num_cpus: float | None = None
    max_concurrency: int | None = None
    max_tasks_in_flight_per_actor: int | None = None
    max_restarts: int | None = None
    max_task_retries: int | None = None

    def __post_init__(self) -> None:
        """Validate sizes, resource reservations and retry counts."""
        for name in (
            "batch_size",
            "concurrency",
            "max_concurrency",
            "max_tasks_in_flight_per_actor",
        ):
            _require_int(f"batch_options.{name}", getattr(self, name), 1)
        for name in ("num_gpus", "num_cpus"):
            _require_number(f"batch_options.{name}", getattr(self, name), 0)
        for name in ("max_restarts", "max_task_retries"):
            _require_int(f"batch_options.{name}", getattr(self, name), -1)
        if self.num_gpus and self.batch_size is None:
            raise ConfigurationError(
                "batch_options.batch_size is required when num_gpus is set."
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
        model: The registered model to score, looked up in the registry even
            when the task is also given a model. Its ``training_framework``
            label picks the artifact: the raw artifact for ``custom`` models
            and the deployable artifact for every other model. ``None`` scores
            the model passed to the task, which at most one entry may do.
        input_columns: Columns fed to the model, in the order the model's
            input schema expects. ``None`` or an empty list uses the model's
            input schema.
        batch_options: Ray processing options. ``None`` uses the defaults.
        padding_options: Padding values for ragged input columns. ``None``
            uses the defaults.
        columns_to_keep: Restricts the output to exactly these input columns,
            plus the prediction and metadata columns, which are always kept.
            ``None`` keeps every input column. Label columns are not kept
            automatically: list them here if a downstream task, such as the
            evaluator, needs them, or they are dropped from the output.

    Example:
        >>> cfg = OfflineInferenceConfig(columns_to_keep=["label"])
        >>> cfg.batch_options is None
        True
    """

    model: ModelIdentifier | None = None
    input_columns: list[str] | None = None
    batch_options: BatchOptions | None = None
    padding_options: PaddingOptions | None = None
    columns_to_keep: list[str] | None = None


@dataclass
class TabularInferenceConfig:
    """Configuration for the ``tabular_inference`` workflow task.

    The field is named ``local_models`` because the models run in the task's own
    Ray cluster, as opposed to being called over the network.

    Attributes:
        local_models: One entry per model to score. Several entries produce the
            union of their predictions: one output row per input row per
            entry, with every entry's prediction columns, null where another
            entry produced them. A model looked up in the registry adds
            ``model_name`` and ``model_version`` columns so the rows can be told
            apart; a model passed to the task directly has no name to record. An
            empty or ``None`` list returns the inputs unchanged.
        sampling: Scores only a fraction of each dataset's rows. ``None`` scores
            every row.
        parquet_read_config: kwargs forwarded to ``ray.data.read_parquet`` when
            loading the input datasets.
        write_config: Output write options for the scored datasets.
        ray_data_context: Ray Data block sizing and I/O retry tuning.
        max_errored_blocks: How many Ray Data blocks may fail before the task
            fails. ``None`` keeps Ray's default of ``0``, which fails on the
            first errored block.

            The budget applies to each dataset separately, because each one
            runs as its own Ray Data execution, so a task scoring ``K``
            datasets can drop up to ``K * max_errored_blocks`` blocks. A block
            is usually one batch of about ``batch_options.batch_size`` rows,
            but a failed read or write can drop a whole file.

            Failed blocks are left out of the output with no placeholder rows,
            and the dropped row count is not recorded, so only set this when
            downstream tasks tolerate incomplete data. A worker that crashes is
            restarted and its block retried (see ``BatchOptions.max_restarts``)
            rather than counted here. It is a field of this config rather than
            of ``ray_data_context``, which training and transform tasks share
            and where dropping data is never acceptable.

    Raises:
        ConfigurationError: If ``max_errored_blocks`` is not ``None`` or an
            integer of at least ``0``.

    Example:
        >>> TabularInferenceConfig(local_models=[OfflineInferenceConfig()])
        ... # doctest: +ELLIPSIS
        TabularInferenceConfig(local_models=[OfflineInferenceConfig(...)], ...)
    """

    local_models: list[OfflineInferenceConfig] | None = None
    sampling: SamplingConfig | None = None
    parquet_read_config: ParquetReadConfig | None = None
    write_config: WriteConfig | None = None
    ray_data_context: RayDataContextConfig | None = None
    max_errored_blocks: int | None = None

    def __post_init__(self) -> None:
        """Validate ``max_errored_blocks``."""
        value = self.max_errored_blocks
        if value is None:
            return
        if isinstance(value, bool) or not isinstance(value, int) or value < 0:
            raise ConfigurationError(
                f"max_errored_blocks must be an integer >= 0, got {value!r}."
            )
