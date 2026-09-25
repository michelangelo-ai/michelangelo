"""Evaluator workflow task: score datasets and render evaluation reports.

:func:`evaluate` is the pipeline entry point. It drives
:class:`~michelangelo.lib.evaluator.evaluator.TorchMetricEvaluator` over one or
more datasets, optionally segmenting the results, gates the run on the
configured sanity checks, and returns the per-dataset metric tables alongside a
combined ``EvaluationReport``.

Work is pushed onto Ray wherever the dataset is the expensive part: column
projection is pushed into the parquet read, the preprocessor runs per batch on
workers, and segment metrics are computed by a distributed ``groupby``. Only
the aggregate row is materialized on the driver.
"""

from __future__ import annotations

import importlib
import logging
from typing import TYPE_CHECKING, Any

import pandas as pd
import ray

from michelangelo.api.v2.util import generate_random_name
from michelangelo.lib.evaluator.chart_data import (
    chart_segment_labels,
    compute_chart_data_for_group,
    filter_chart_df_by_segments,
    filter_ray_to_surviving_segments,
    filter_small_segments,
)
from michelangelo.lib.evaluator.evaluator import TorchMetricEvaluator
from michelangelo.lib.evaluator.report_generator import (
    DatasetResult,
    build_combined_report,
    generate_evaluation_report,
)
from michelangelo.lib.evaluator.sanity import (
    GLOBAL_SCOPE,
    FailureLogBudget,
    format_failures,
    run_sanity_checks,
    segment_scope_label,
    validate_sanity_checks,
    verify_all_checks_ran,
)
from michelangelo.lib.exceptions import (
    ConfigurationError as LibConfigurationError,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.tasks.evaluator.exceptions import SanityCheckFailedError
from michelangelo.workflow.variables import MessageVariable
from michelangelo.workflow.variables.types import EvaluationMetrics, EvaluationResult

if TYPE_CHECKING:
    from michelangelo.lib.evaluator.sanity import SanityCheckResult
    from michelangelo.workflow.schema.evaluator import EvaluatorConfig
    from michelangelo.workflow.tasks.evaluator.preprocessor import PreprocessorFn
    from michelangelo.workflow.variables import DatasetVariable

_logger = logging.getLogger(__name__)

__all__ = ["evaluate"]

# Segment values are sorted during the distributed groupby, and sorting a column
# holding nulls raises. Nulls become their own explicit segment instead.
_NULL_SEGMENT = "UNKNOWN"

# Row count each segment carries out of the groupby, so segments too thin to
# conclude anything from can be dropped before they reach a report.
_ROW_COUNT_COLUMN = "_segment_row_count"

# Name of the combined report -- the one a pusher publishes. Per-dataset reports
# are keyed by dataset name alongside it.
COMBINED_REPORT_KEY = "performance_evaluation_report"


def evaluate(
    config: EvaluatorConfig,
    datasets: dict[str, DatasetVariable],
) -> EvaluationResult:
    """Evaluate one or more datasets and render their reports.

    Args:
        config: Evaluator configuration: the metrics to compute, the columns to
            segment by, an optional pre-evaluation preprocessor, and the sanity
            checks to gate on.
        datasets: Mapping of dataset name (e.g. ``"train"``, ``"validation"``,
            ``"test"``) to the dataset holding its predictions and labels.
            ``None`` values are skipped, and an empty ``"test"`` dataset is
            treated as absent rather than as an error -- test splits are
            optional in most pipelines.

    Returns:
        An :class:`~michelangelo.workflow.variables.types.EvaluationResult`
        holding one :class:`~michelangelo.workflow.variables.types.EvaluationMetrics`
        per evaluated dataset and one report per dataset, plus the combined
        report under ``"performance_evaluation_report"``.

    Raises:
        ConfigurationError: If the configuration cannot produce a usable
            evaluation -- no metrics declared, sanity checks naming a metric
            that will never be computed or a dataset that is never evaluated,
            or a ``data_preprocessor`` that does not resolve to a callable
            carrying ``input_columns``. This is always
            ``michelangelo.workflow.schema.exceptions.ConfigurationError``:
            the evaluator library's own ``ConfigurationError`` is translated,
            so callers catch one type rather than two.
        ValueError: If a dataset is missing a column the read or the metrics
            require.
        SanityCheckFailedError: If a configured check breaches its threshold.
            Raised only after every report has been written.

    Example:
        >>> from michelangelo.workflow.schema.evaluator import (
        ...     ColumnMapping,
        ...     EvaluatorConfig,
        ...     MetricConfig,
        ... )
        >>> config = EvaluatorConfig(
        ...     metrics=[
        ...         MetricConfig(
        ...             name="auc",
        ...             metric="torchmetrics.classification.BinaryAUROC",
        ...             columns=ColumnMapping(
        ...                 prediction_col="score", target_col="label"
        ...             ),
        ...         )
        ...     ]
        ... )
        >>> [m.name for m in config.metrics]
        ['auc']
    """
    _logger.info("Starting evaluator")

    if config.steps:
        raise ConfigurationError(
            "Step-based evaluation (`steps`) is not supported by this evaluator. "
            "Declare the metrics to compute under `metrics` instead."
        )

    if not config.metrics:
        raise ConfigurationError(
            "EvaluatorConfig declares no metrics, so there is nothing to evaluate. "
            "Add at least one entry to `metrics`."
        )

    return _evaluate_with_torchmetrics(config, datasets)


def _as_config_error(fn: Any, *args: Any) -> Any:
    """Call a library validator, re-raising its error as a workflow one.

    ``michelangelo.lib.evaluator`` raises its own ``ConfigurationError``, which
    is a different class from the workflow layer's. Callers of :func:`evaluate`
    should not have to catch both, so the library's is translated here.

    Args:
        fn: Validator to call.
        *args: Positional arguments to pass through.

    Returns:
        Whatever the validator returned.

    Raises:
        ConfigurationError: Carrying the library error's message.
    """
    try:
        return fn(*args)
    except LibConfigurationError as err:
        raise ConfigurationError(str(err)) from err


def _resolve_preprocessor(config: EvaluatorConfig) -> PreprocessorFn | None:
    """Resolve ``config.data_preprocessor`` to a decorated callable.

    Args:
        config: Evaluator configuration. ``None`` or an empty
            ``data_preprocessor`` means no preprocessor is configured.

    Returns:
        The resolved preprocessor, or ``None`` when none is configured.

    Raises:
        ConfigurationError: If the path does not import, does not resolve to a
            callable, or resolves to one missing ``input_columns``.
    """
    path = config.data_preprocessor
    if not path:
        return None

    _logger.info("Resolving data_preprocessor: %s", path)
    try:
        module_path, attr_name = path.rsplit(".", 1)
        preprocessor = getattr(importlib.import_module(module_path), attr_name)
    except (ImportError, AttributeError, ValueError) as err:
        raise ConfigurationError(
            f"data_preprocessor {path!r} could not be imported: {err}"
        ) from err

    if not callable(preprocessor):
        raise ConfigurationError(
            f"data_preprocessor {path!r} resolved to a non-callable "
            f"{type(preprocessor).__name__}."
        )
    if not hasattr(preprocessor, "input_columns"):
        raise ConfigurationError(
            f"data_preprocessor {path!r} is missing an `input_columns` attribute. "
            "Decorate the function with "
            "`michelangelo.workflow.tasks.evaluator.declare_inputs([...])` so the "
            "evaluator knows which raw columns to read from parquet."
        )
    return preprocessor


def _fill_segment_nulls(
    batch: pd.DataFrame, segment_columns: list[str]
) -> pd.DataFrame:
    """Replace nulls in the segment columns with :data:`_NULL_SEGMENT`.

    Args:
        batch: One Ray batch, in pandas format.
        segment_columns: Segment column names. Names absent from the batch are
            ignored.

    Returns:
        The same batch with nulls filled in place.
    """
    for column in segment_columns:
        if column in batch.columns:
            batch[column] = batch[column].fillna(_NULL_SEGMENT)
    return batch


def _evaluate_segment(
    group_df: pd.DataFrame,
    evaluator: TorchMetricEvaluator,
    segment_columns: list[str],
) -> pd.DataFrame:
    """Compute one segment's metrics, as a single result row.

    Args:
        group_df: Every row belonging to one segment.
        evaluator: Evaluator to score the group with.
        segment_columns: Segment column names, carried back onto the result so
            the row identifies which segment it describes.

    Returns:
        A one-row DataFrame of metric values, the segment's column values, and
        its row count under :data:`_ROW_COUNT_COLUMN`.
    """
    results = evaluator.evaluate(group_df)

    # Every row in the group shares the segment's values, so the first row is
    # representative.
    for column in segment_columns:
        results[column] = group_df[column].iloc[0]

    results[_ROW_COUNT_COLUMN] = len(group_df)
    return pd.DataFrame([results])


def _load_dataset(
    dataset: DatasetVariable,
    dataset_name: str,
    load_columns: set[str],
    *,
    has_preprocessor: bool,
) -> Any | None:
    """Read one dataset from storage, projected to ``load_columns``.

    Args:
        dataset: The dataset variable to load.
        dataset_name: Name the dataset is keyed by, used in messages.
        load_columns: Columns to read. Projection is pushed down into the
            parquet read.
        has_preprocessor: Whether a preprocessor drove the column list, which
            only changes how a missing column is explained.

    Returns:
        The loaded Ray dataset, or ``None`` when an optional ``"test"`` split
        turned out to be absent or empty.

    Raises:
        ValueError: If a non-test dataset is empty, or any dataset is missing a
            column that was asked for.
    """
    dataset.load_ray_dataset(columns=list(load_columns))
    ray_dataset = dataset.value

    if ray_dataset is None or not ray_dataset.take(1):
        # A test split is optional in most pipelines; anything else being empty
        # means an upstream task produced nothing and the run should stop.
        if dataset_name == "test":
            _logger.info("Test dataset is empty after loading, skipping evaluation")
            return None
        raise ValueError(f"Dataset {dataset_name} loaded but is None/empty")

    available_columns = set(ray_dataset.schema().names)
    missing = load_columns - available_columns
    if missing:
        source = (
            "required for loading by the configured data_preprocessor"
            if has_preprocessor
            else "required by the configured metrics"
        )
        raise ValueError(
            f"Dataset {dataset_name} is missing columns {source}: {sorted(missing)}. "
            f"Available columns: {sorted(available_columns)}"
        )
    return ray_dataset


def _apply_preprocessor(
    ray_dataset: Any,
    preprocessor: PreprocessorFn,
    dataset_name: str,
    required_columns: set[str],
) -> Any:
    """Map the preprocessor over the dataset and re-check the metric columns.

    Args:
        ray_dataset: Dataset to transform.
        preprocessor: Pandas-in / pandas-out callable, applied per batch.
        dataset_name: Name the dataset is keyed by, used in messages.
        required_columns: Columns the metrics and segmentation need to exist
            once the transform has run.

    Returns:
        The transformed dataset.

    Raises:
        ValueError: If the transform does not produce every required column.
    """
    _logger.info("Applying data_preprocessor to dataset %s", dataset_name)
    ray_dataset = ray_dataset.map_batches(preprocessor, batch_format="pandas")

    # The schema is only knowable once the transform is declared, so the metric
    # columns are checked here rather than alongside the read.
    post_transform_columns = set(ray_dataset.schema().names)
    missing = required_columns - post_transform_columns
    if missing:
        raise ValueError(
            f"After applying the data_preprocessor, dataset {dataset_name} is missing "
            f"columns required by metrics/segmentation: {sorted(missing)}. "
            f"Post-transform columns: {sorted(post_transform_columns)}"
        )
    return ray_dataset


def _evaluate_segmented(
    ray_dataset: Any,
    evaluator: TorchMetricEvaluator,
    config: EvaluatorConfig,
) -> tuple[pd.DataFrame, pd.DataFrame, list[tuple[str, dict[str, Any]]]]:
    """Evaluate a dataset per segment, and optionally as a whole.

    Args:
        ray_dataset: Dataset to evaluate, already preprocessed.
        evaluator: Evaluator to score each segment with.
        config: Evaluator configuration supplying the segment columns, the
            minimum segment size, and whether to add the aggregate row.

    Returns:
        A ``(summary_df, chart_df, scopes)`` triple. ``scopes`` pairs each
        gateable scope label with its metrics, aggregate first.
    """
    segment_columns = list(config.segment_columns)
    _logger.info("Computing segmented metrics, grouping by %s", segment_columns)

    ray_dataset = ray_dataset.map_batches(
        lambda batch: _fill_segment_nulls(batch, segment_columns),
        batch_format="pandas",
    )

    segment_df = (
        ray_dataset.groupby(segment_columns)
        .map_groups(
            lambda group_df: _evaluate_segment(group_df, evaluator, segment_columns)
        )
        .to_pandas()
    )
    _logger.info("Computed %d segment(s)", len(segment_df))
    segment_df = filter_small_segments(segment_df, config.min_segment_row_count)

    scopes: list[tuple[str, dict[str, Any]]] = []
    if config.sanity_checks:
        # Post-filter: segments too thin to survive are never gated. Segment
        # columns are stripped so a metric sharing a name with a segment column
        # is not compared against that column's own value.
        segment_column_set = set(segment_columns)
        scopes.extend(
            (
                segment_scope_label(row, segment_columns),
                {k: v for k, v in row.items() if k not in segment_column_set},
            )
            for row in segment_df.to_dict(orient="records")
        )

    # Curves are computed only for segments that survived, so the groupby does
    # not spend work on segments no report will show.
    surviving = filter_ray_to_surviving_segments(
        ray_dataset, segment_df, segment_columns, config.min_segment_row_count
    )
    segment_chart_df = (
        surviving.groupby(segment_columns)
        .map_groups(
            lambda group_df, cols=segment_columns: compute_chart_data_for_group(
                group_df, evaluator.config, cols
            ),
            batch_format="pandas",
        )
        .to_pandas()
    )
    # Safety net: a no-op when the pre-filter above ran, still needed when
    # min_segment_row_count is 0 and filter_ray_to_surviving_segments passes
    # the dataset straight through.
    segment_chart_df = filter_chart_df_by_segments(
        segment_chart_df, chart_segment_labels(segment_df, segment_columns)
    )

    global_chart_df = pd.DataFrame()
    if config.enable_global_segment_evaluation:
        global_df = ray_dataset.to_pandas()
        _logger.info("Aggregate row over %d rows", len(global_df))
        global_results = evaluator.evaluate(global_df)
        if config.sanity_checks:
            # Snapshot the metrics before segment columns are stamped in below,
            # and lead with the aggregate so it heads any failure list.
            scopes.insert(0, (GLOBAL_SCOPE, dict(global_results)))

        global_chart_df = compute_chart_data_for_group(global_df, evaluator.config)
        global_chart_df["segment"] = "GLOBAL"
        del global_df

        for column in segment_columns:
            global_results[column] = "GLOBAL"
        summary_df = pd.concat(
            [pd.DataFrame([global_results]), segment_df], ignore_index=True
        )
    else:
        summary_df = segment_df

    chart_frames = [df for df in (global_chart_df, segment_chart_df) if not df.empty]
    chart_df = (
        pd.concat(chart_frames, ignore_index=True) if chart_frames else pd.DataFrame()
    )
    return summary_df, chart_df, scopes


def _evaluate_whole(
    ray_dataset: Any,
    evaluator: TorchMetricEvaluator,
    config: EvaluatorConfig,
) -> tuple[pd.DataFrame, pd.DataFrame, list[tuple[str, dict[str, Any]]]]:
    """Evaluate a dataset as a single aggregate, with no segmentation.

    Args:
        ray_dataset: Dataset to evaluate, already preprocessed.
        evaluator: Evaluator to score the dataset with.
        config: Evaluator configuration, read for the sanity checks.

    Returns:
        A ``(summary_df, chart_df, scopes)`` triple, where ``scopes`` holds at
        most the single aggregate scope.
    """
    df = ray_dataset.to_pandas()
    _logger.info("Computing metrics over %d rows (no segmentation)", len(df))

    results = evaluator.evaluate(df)
    # Without segmentation the whole-dataset result is the aggregate, and the
    # only scope there is to gate on.
    scopes = [(GLOBAL_SCOPE, dict(results))] if config.sanity_checks else []

    chart_df = compute_chart_data_for_group(df, evaluator.config)
    chart_df["segment"] = "GLOBAL"
    return pd.DataFrame([results]), chart_df, scopes


def _save_summary(summary_df: pd.DataFrame) -> EvaluationMetrics:
    """Persist a summary table and wrap it for a downstream task.

    Args:
        summary_df: One row per scope, one column per metric.

    Returns:
        The metrics holder. ``instance_metrics`` stays ``None``: the
        TorchMetrics path aggregates and never scores individual rows.
    """
    from michelangelo.workflow.variables import DatasetVariable

    summary_var = DatasetVariable.create(ray.data.from_pandas(summary_df))
    summary_var.save()
    return EvaluationMetrics(summary_metrics=summary_var)


def _gate_on_sanity_checks(
    config: EvaluatorConfig,
    sanity_results: list[SanityCheckResult],
    evaluated_datasets: list[str],
) -> None:
    """Fail the run if any configured check breached its threshold.

    Called only after every report is written, so an operator debugging a
    breach still has the report that explains it.

    Args:
        config: Evaluator configuration holding the checks.
        sanity_results: Every check outcome, across datasets and scopes.
        evaluated_datasets: Names of the datasets that were actually evaluated.

    Raises:
        SanityCheckFailedError: If any check failed.
    """
    _as_config_error(verify_all_checks_ran, config.sanity_checks, evaluated_datasets)
    if any(not result.passed for result in sanity_results):
        raise SanityCheckFailedError(format_failures(sanity_results))
    _logger.info("All %d sanity check(s) passed", len(sanity_results))


def _evaluate_with_torchmetrics(
    config: EvaluatorConfig,
    datasets: dict[str, DatasetVariable],
) -> EvaluationResult:
    """Run the TorchMetrics evaluation over every dataset.

    Args:
        config: Validated evaluator configuration.
        datasets: Datasets to evaluate, keyed by name.

    Returns:
        The populated evaluation result.

    Raises:
        SanityCheckFailedError: If a configured check breached its threshold.
    """
    evaluator = TorchMetricEvaluator(config)

    # Validated before a single byte is read: a typo'd metric name should fail
    # in seconds, not after a multi-gigabyte evaluation.
    _as_config_error(
        validate_sanity_checks, config, evaluator.get_metric_names(), datasets.keys()
    )

    # Columns the metrics and segmentation need *after* any preprocessor ran.
    required_columns = evaluator.get_required_columns()
    if config.segment_columns:
        required_columns = required_columns | set(config.segment_columns)

    # With a preprocessor, the read is driven by its declared inputs instead --
    # the metric columns do not exist until the transform has run.
    preprocessor = _resolve_preprocessor(config)
    load_columns = (
        set(preprocessor.input_columns)
        if preprocessor is not None
        else required_columns
    )
    _logger.info("Reading %d column(s) from source datasets", len(load_columns))

    metrics: dict[str, EvaluationMetrics] = {}
    reports: dict[str, MessageVariable] = {}
    results: list[DatasetResult] = []
    segment_columns = list(config.segment_columns) if config.segment_columns else None

    # Failures accumulate across datasets and are acted on only once every
    # report is saved. The log budget is shared for the whole run rather than
    # reset per scope: a metric broken globally is usually broken in every
    # segment, and logging each one buries the signal.
    sanity_results: list[SanityCheckResult] = []
    evaluated_datasets: list[str] = []
    log_budget = FailureLogBudget()

    for dataset_name, dataset in datasets.items():
        if not dataset:
            _logger.warning("Dataset %s is None, skipping", dataset_name)
            continue

        _logger.info("Evaluating dataset: %s", dataset_name)
        ray_dataset = _load_dataset(
            dataset,
            dataset_name,
            load_columns,
            has_preprocessor=preprocessor is not None,
        )
        if ray_dataset is None:
            continue

        if preprocessor is not None:
            ray_dataset = _apply_preprocessor(
                ray_dataset, preprocessor, dataset_name, required_columns
            )

        if config.segment_columns:
            summary_df, chart_df, scopes = _evaluate_segmented(
                ray_dataset, evaluator, config
            )
        else:
            summary_df, chart_df, scopes = _evaluate_whole(
                ray_dataset, evaluator, config
            )
        del ray_dataset

        _logger.info(
            "Dataset %s evaluated: %d row(s) x %d column(s)",
            dataset_name,
            len(summary_df),
            len(summary_df.columns),
        )
        metrics[dataset_name] = _save_summary(summary_df)
        evaluated_datasets.append(dataset_name)

        if config.sanity_checks:
            sanity_results.extend(
                _run_dataset_sanity_checks(config, dataset_name, scopes, log_budget)
            )

        results.append(
            DatasetResult(name=dataset_name, summary_df=summary_df, chart_df=chart_df)
        )
        # Per-dataset report, keyed by dataset name. Only the combined report is
        # normally published, but a comparator walking `metrics` expects to find
        # a report under each dataset's own key.
        per_dataset_report = generate_evaluation_report(
            summary_df=summary_df,
            segment_columns=segment_columns,
            title=f"Evaluation Report - {dataset_name}",
            chart_df=chart_df,
        )
        per_dataset_report_var = MessageVariable.create(per_dataset_report)
        per_dataset_report_var.save()
        reports[dataset_name] = per_dataset_report_var

    if results:
        combined = build_combined_report(results, segment_columns)
        # Named here so a model plugin can stamp the name into the (immutable)
        # model spec while the report entity is created under the same name.
        combined.metadata.name = generate_random_name("evaluation-report")
        combined_report_var = MessageVariable.create(combined)
        combined_report_var.save()
        reports[COMBINED_REPORT_KEY] = combined_report_var

    if config.sanity_checks:
        _gate_on_sanity_checks(config, sanity_results, evaluated_datasets)

    return EvaluationResult(metrics=metrics, reports=reports)


def _run_dataset_sanity_checks(
    config: EvaluatorConfig,
    dataset_name: str,
    scopes: list[tuple[str, dict[str, Any]]],
    log_budget: FailureLogBudget,
) -> list[SanityCheckResult]:
    """Run every configured check against one dataset's scopes.

    Args:
        config: Evaluator configuration holding the checks.
        dataset_name: Dataset the scopes belong to.
        scopes: ``(label, metrics)`` pairs to gate, aggregate first.
        log_budget: Run-wide budget capping how many failures are logged.

    Returns:
        One result per check and scope.

    Raises:
        ConfigurationError: If a check targets this dataset but no scope was
            computed for it.
    """
    targets_dataset = any(
        dataset_name in check.datasets for check in config.sanity_checks
    )
    if not scopes and targets_dataset:
        # enable_global_segment_evaluation=False, combined with
        # min_segment_row_count dropping every segment, leaves nothing to gate.
        # The dataset was still evaluated, so verify_all_checks_ran would not
        # catch it, and the run would go green having gated nothing.
        raise ConfigurationError(
            f"Sanity checks target dataset {dataset_name!r}, but no scope (GLOBAL or "
            "segment) was computed for it -- check enable_global_segment_evaluation "
            "and min_segment_row_count. A check with nothing to gate on is a "
            "configuration error, not a pass."
        )

    results: list[SanityCheckResult] = []
    for scope_label, scope_metrics in scopes:
        results.extend(
            run_sanity_checks(
                dataset_name,
                scope_label,
                scope_metrics,
                config.sanity_checks,
                log_budget,
            )
        )
    _logger.info(
        "Dataset %s: %d check(s) across %d scope(s), %d breached",
        dataset_name,
        len(results),
        len(scopes),
        sum(1 for result in results if not result.passed),
    )
    return results
