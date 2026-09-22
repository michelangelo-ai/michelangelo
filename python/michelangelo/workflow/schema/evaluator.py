"""Configuration dataclasses for the evaluator workflow task.

These classes are the canonical configuration schema for the evaluator task.
Mirrors the structure of ``michelangelo.workflow.schema.tabular_trainer`` --
plain ``@dataclass`` with ``__post_init__`` validation; no Pydantic dependency.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Any

from michelangelo.workflow.schema.exceptions import ConfigurationError

__all__ = [
    "ColumnMapping",
    "CustomEvalConfig",
    "DataAnalyticsConfig",
    "EvaluationConfig",
    "EvaluatorConfig",
    "Metric",
    "MetricConfig",
    "SanityCheck",
    "SanityCheckOperator",
    "Step",
    "StepConfig",
]


@dataclass
class Metric:
    """A named metric with free-form constructor parameters.

    Used by the legacy step-based evaluation path (:class:`EvaluationConfig`).
    The TorchMetrics path uses :class:`MetricConfig` instead.

    Attributes:
        name: Metric name.
        parameters: Constructor parameters for the metric.

    Example:
        >>> Metric(name="auc", parameters={"num_classes": 2}).name
        'auc'
    """

    name: str
    parameters: dict[str, Any] = field(default_factory=dict)


@dataclass
class EvaluationConfig:
    """Label / prediction columns and metrics for step-based evaluation.

    Attributes:
        labels: Label column names.
        prediction_cols: Prediction column names.
        metrics: Metrics to compute.
    """

    labels: list[str] = field(default_factory=list)
    prediction_cols: list[str] = field(default_factory=list)
    metrics: list[Metric] = field(default_factory=list)


@dataclass
class CustomEvalConfig:
    """Free-form arguments forwarded to a user-supplied evaluation step.

    Attributes:
        args: Arbitrary keyword arguments for the custom processor.
    """

    args: dict[str, Any] = field(default_factory=dict)


@dataclass
class StepConfig:
    """Configuration for one evaluation step.

    Exactly one of ``evaluation_config`` or ``custom_config`` must be set.

    Attributes:
        evaluation_config: Built-in metric evaluation.
        custom_config: User-supplied processor arguments.

    Raises:
        ConfigurationError: If neither or both fields are set.

    Example:
        >>> cfg = StepConfig(custom_config=CustomEvalConfig(args={}))
        >>> cfg.evaluation_config is None
        True
    """

    evaluation_config: EvaluationConfig | None = None
    custom_config: CustomEvalConfig | None = None

    def __post_init__(self) -> None:
        """Enforce the evaluation_config / custom_config one-of."""
        set_fields = [
            name
            for name, value in (
                ("evaluation_config", self.evaluation_config),
                ("custom_config", self.custom_config),
            )
            if value is not None
        ]
        if len(set_fields) != 1:
            got = ", ".join(set_fields) if set_fields else "neither"
            raise ConfigurationError(
                "StepConfig requires exactly one of 'evaluation_config' or "
                f"'custom_config'; got {got}."
            )


@dataclass
class Step:
    """A single evaluation step in the legacy step-based pipeline.

    Attributes:
        processor: Import path to the step processor.
        name: Optional step name.
        run_after: Optional name of the step this one depends on.
        config: Optional step configuration.
    """

    processor: str
    name: str | None = None
    run_after: str | None = None
    config: StepConfig | None = None


@dataclass
class ColumnMapping:
    """Column mapping configuration for a metric.

    Attributes:
        prediction_col: Column name for predictions (arrays for multi-label).
        target_col: Column name for targets/labels (arrays for multi-label).
        index_col: Column name for query/group indexes (ranking metrics).
        sample_weight_col: Optional column name for sample weights.
        extra_cols: Optional dict mapping logical names to DataFrame column
            names. Values are cast to float tensors and forwarded as keyword
            arguments only to metrics whose ``update()`` accepts ``**kwargs``.
            Standard TorchMetrics (e.g. AUROC) are called without these extra
            arguments, so it is safe to share a ``ColumnMapping`` between
            standard and custom metrics. Example:
            ``{"multiplier": "px_multiplier_base", "segment": "trip_distance"}``.
            The custom metric's ``update()`` should accept ``**kwargs``.

    Example:
        >>> m = ColumnMapping(prediction_col="p", target_col="t")
        >>> m == ColumnMapping(prediction_col="p", target_col="t")
        True
        >>> isinstance(hash(m), int)
        True
    """

    prediction_col: str
    target_col: str
    index_col: str | None = None
    sample_weight_col: str | None = None
    extra_cols: dict[str, str] | None = None

    def __hash__(self) -> int:
        """Hash on all fields so a mapping can key a dict of metric groups."""
        extra_cols_hash = (
            tuple(sorted(self.extra_cols.items())) if self.extra_cols else None
        )
        return hash(
            (
                self.prediction_col,
                self.target_col,
                self.index_col,
                self.sample_weight_col,
                extra_cols_hash,
            )
        )

    def __eq__(self, other: object) -> bool:
        """Compare all fields; non-ColumnMapping operands are never equal."""
        if not isinstance(other, ColumnMapping):
            return False
        return (
            self.prediction_col == other.prediction_col
            and self.target_col == other.target_col
            and self.index_col == other.index_col
            and self.sample_weight_col == other.sample_weight_col
            and self.extra_cols == other.extra_cols
        )


@dataclass
class MetricConfig:
    """Configuration for a single TorchMetrics-based metric.

    Attributes:
        name: Name of the metric instance (used as key in results).
        metric: Full import path to the metric, e.g.
            ``'torchmetrics.classification.MulticlassAUROC'``.
        params: Parameters passed to the metric constructor.
        columns: Column mapping for DataFrame evaluation.
        filter_expr: Optional pandas query string applied before this metric is
            computed. Applied per-metric, so different metrics can filter on
            different conditions, e.g. ``"trip_basket_size_usd >= 0"``.
    """

    name: str
    metric: str
    params: dict[str, Any] = field(default_factory=dict)
    columns: ColumnMapping | None = None
    filter_expr: str | None = None


class SanityCheckOperator(str, Enum):
    """Comparison applied between a metric value and its threshold.

    Read as ``value <operator> threshold`` -- ``">="`` with ``threshold=0.55``
    passes when the metric is at least 0.55.

    QUOTE THESE IN YAML. A plain YAML scalar may not begin with ``>``: it
    introduces a folded block scalar, so ``operator: >`` silently parses as an
    empty string and ``operator: >=`` is a scanner error. Write
    ``operator: ">="``. ``<`` and ``<=`` parse bare, but quoting all four keeps
    the config uniform.

    Example:
        >>> SanityCheckOperator.GTE.value
        '>='
    """

    GT = ">"
    GTE = ">="
    LT = "<"
    LTE = "<="


@dataclass
class SanityCheck:
    """A single threshold assertion on an evaluated metric.

    Checks run against *every scope* of a dataset they target: the aggregate
    GLOBAL metrics, plus every surviving segment when ``segment_columns`` is
    set. One bad segment fails the run even if GLOBAL passes -- see
    :attr:`EvaluatorConfig.segment_columns` for why ``min_segment_row_count``
    must be set above zero before gating segments.

    Attributes:
        metric: Name of the metric to gate on. Must match a
            ``MetricConfig.name`` declared in ``EvaluatorConfig.metrics``; a
            name that matches nothing is a configuration error, raised before
            any data is read.
        threshold: Value to compare against. With ``use_abs=True`` and a
            ``"<"``/``"<="`` operator, must be positive (for ``"<="``,
            non-negative) -- ``abs(value)`` is never negative, so a
            non-positive threshold there can never be satisfied.
        operator: Comparison direction, read as ``value <operator> threshold``.
        use_abs: Compare ``abs(value)`` instead of ``value``. Intended for
            two-sided metrics such as bias, where ``abs(bias) <= 0.3`` is the
            meaningful bound.
        datasets: Which evaluated datasets this check applies to. Defaults to
            the held-out ``test`` split; train/validation metrics are still
            reported, just not gated, unless named here.

    Raises:
        ConfigurationError: If ``operator`` is empty or unrecognised, or if
            ``use_abs`` is combined with a threshold that can never be met.

    Example:
        >>> SanityCheck(metric="auc", threshold=0.55).operator.value
        '>='
    """

    metric: str
    threshold: float
    operator: SanityCheckOperator = SanityCheckOperator.GTE
    use_abs: bool = False
    datasets: list[str] = field(default_factory=lambda: ["test"])

    def __post_init__(self) -> None:
        """Normalise ``operator`` and reject thresholds ``use_abs`` cannot satisfy."""
        self.operator = self._normalize_operator(self.operator)
        self._validate_abs_threshold()

    @staticmethod
    def _normalize_operator(value: SanityCheckOperator | str) -> SanityCheckOperator:
        """Tolerate surrounding whitespace, and name YAML's block-scalar trap.

        An unquoted ``operator: >`` reaches us as an empty string rather than
        ``">"``, which would otherwise surface as an opaque "not a valid
        enumeration member" error pointing at nothing the author wrote.
        """
        if isinstance(value, SanityCheckOperator):
            return value
        if isinstance(value, str):
            stripped = value.strip()
            if not stripped:
                raise ConfigurationError(
                    'operator is empty. In YAML a plain scalar cannot start with ">" '
                    "-- it opens a folded block scalar, so `operator: >` parses as an "
                    'empty string. Quote it: `operator: ">"` (likewise '
                    '`operator: ">="`).'
                )
            try:
                return SanityCheckOperator(stripped)
            except ValueError as exc:
                valid = ", ".join(repr(op.value) for op in SanityCheckOperator)
                raise ConfigurationError(
                    f"operator {stripped!r} is not a valid comparison; "
                    f"expected one of {valid}."
                ) from exc
        raise ConfigurationError(
            "operator must be a string or SanityCheckOperator; "
            f"got {type(value).__name__}."
        )

    def _validate_abs_threshold(self) -> None:
        """Reject a threshold that ``use_abs`` can never satisfy.

        ``abs(value)`` is never negative, so ``|value| <= -0.1`` or
        ``|value| < 0`` can never be true regardless of the data -- a config
        error, not a legitimate strict gate, and one worth catching now rather
        than as a permanently-failing pipeline.
        """
        if not self.use_abs or self.operator not in (
            SanityCheckOperator.LT,
            SanityCheckOperator.LTE,
        ):
            return
        unsatisfiable = (
            self.threshold < 0
            if self.operator == SanityCheckOperator.LTE
            else self.threshold <= 0
        )
        if unsatisfiable:
            required = ">= 0" if self.operator == SanityCheckOperator.LTE else "> 0"
            raise ConfigurationError(
                f"sanity_check on {self.metric!r} is unsatisfiable: |value| "
                f"{self.operator.value} {self.threshold:g} can never be true, since "
                f"abs() is never negative. Use a threshold {required} with "
                "use_abs=True."
            )


@dataclass
class EvaluatorConfig:
    """Configuration for the evaluator framework.

    Attributes:
        metrics: Metric configurations for TorchMetrics-based evaluation.
        steps: Evaluation steps (legacy step-based evaluation).
        segment_columns: Column names to include for segmented evaluation, e.g.
            ``['region_id', 'city_id', 'datestr']``. These columns are included
            in the dataset load and available for computing segment-level
            metrics and for metadata extraction.
        enable_global_segment_evaluation: Whether to compute global metrics
            alongside segment metrics when ``segment_columns`` is specified.
            When True, adds a ``"GLOBAL"`` row with all segment columns set to
            ``"GLOBAL"``. When False, only segment-level metrics are computed.
        min_segment_row_count: Minimum rows required per segment. Segments with
            fewer rows are filtered out. Default 0 (no filtering).
        data_preprocessor: Fully-qualified import path to a callable taking a
            pandas DataFrame and returning a pandas DataFrame, applied per Ray
            batch (via ``map_batches(batch_format="pandas")``) right after the
            dataset is loaded and before metrics evaluation. The callable MUST
            expose an ``input_columns`` attribute (a list of column names)
            declaring exactly which raw columns it needs from the source
            dataset, so the evaluator can push column projection down to the
            parquet read. Use the ``declare_inputs`` decorator to attach it.
            Typical use case: per-row arrays that must be exploded (and masked)
            into multiple rows before scalar TorchMetrics apply.
        sanity_checks: Threshold assertions on evaluated metrics, applied to
            every scope -- the aggregate GLOBAL metrics plus every surviving
            segment when ``segment_columns`` is set -- so one bad segment fails
            the run even if GLOBAL passes. Any breach fails the evaluator task,
            naming the metric, scope, threshold and actual value, so a model
            that trained to garbage stops here instead of flowing on to the
            pusher. Checks are validated against the configured metric names
            before any data is read.

    Raises:
        ConfigurationError: If ``sanity_checks`` is set without ``metrics``, if
            a check names an unknown metric, or if checks are combined with
            ``segment_columns`` while ``min_segment_row_count`` is 0.

    Example:
        >>> EvaluatorConfig().enable_global_segment_evaluation
        True
    """

    metrics: list[MetricConfig] = field(default_factory=list)
    steps: list[Step] = field(default_factory=list)
    segment_columns: list[str] = field(default_factory=list)
    enable_global_segment_evaluation: bool = True
    min_segment_row_count: int = 0
    data_preprocessor: str | None = None
    sanity_checks: list[SanityCheck] = field(default_factory=list)

    def __post_init__(self) -> None:
        """Validate sanity checks against the configured metrics."""
        if not self.sanity_checks:
            return
        if not self.metrics:
            raise ConfigurationError(
                "sanity_checks require 'metrics' (the TorchMetrics evaluation path); "
                "the step-based 'steps' path does not produce gateable metric values."
            )
        known = {m.name for m in self.metrics}
        unknown = sorted({c.metric for c in self.sanity_checks} - known)
        if unknown:
            raise ConfigurationError(
                f"sanity_checks reference unknown metric(s): {', '.join(unknown)}. "
                f"Declared metrics: {', '.join(sorted(known))}."
            )
        if self.segment_columns and self.min_segment_row_count <= 0:
            raise ConfigurationError(
                "sanity_checks combined with 'segment_columns' require "
                "'min_segment_row_count' > 0, so a single-row segment cannot gate on a "
                "degenerate metric (e.g. single-class AUROC -> NaN)."
            )


@dataclass
class DataAnalyticsConfig:
    """Per-column / per-segment data statistics on the evaluator's splits.

    Data profiling over the train/validation/test splits, independent of
    model-quality metrics.

    Attributes:
        enabled: Pipeline-author opt-in; the sole switch for this feature.
        stats_sampling_ratio: Fraction of rows to sample from each split before
            computing stats. Defaults to 1.0 (profile everything).
        segment_columns: Columns to segment/group stats by (e.g. region, device
            type), in addition to the pipeline metadata columns and the
            synthetic ``dataset_category`` column.
        skip_compute_stats: When True, drop all columns except the always-kept
            segment/metadata/dataset_category columns (no per-feature stats).
        skip_compute_stats_columns: Glob patterns for columns to exclude from
            stats computation.
        allow_compute_stats_columns: Glob patterns; when non-empty, only
            matching columns (plus always-kept columns) are kept.

    Raises:
        ConfigurationError: If ``stats_sampling_ratio`` is outside ``(0, 1]``.

    Example:
        >>> DataAnalyticsConfig().stats_sampling_ratio
        1.0
    """

    enabled: bool = False
    stats_sampling_ratio: float = 1.0
    segment_columns: list[str] = field(default_factory=list)
    skip_compute_stats: bool = False
    skip_compute_stats_columns: list[str] = field(default_factory=list)
    allow_compute_stats_columns: list[str] = field(default_factory=list)

    def __post_init__(self) -> None:
        """Reject a sampling ratio that would profile nothing."""
        if not 0.0 < self.stats_sampling_ratio <= 1.0:
            raise ConfigurationError(
                "stats_sampling_ratio must be in (0, 1]; got "
                f"{self.stats_sampling_ratio!r}. "
                "A ratio of 0 samples every split down to empty."
            )
