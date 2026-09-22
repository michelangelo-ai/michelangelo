"""TorchMetrics-based evaluator driven by :class:`EvaluatorConfig`.

:class:`TorchMetricEvaluator` groups the configured metrics into TorchMetrics
``MetricCollection``s keyed by the data they need, then evaluates a pandas
DataFrame against all of them in one pass.
"""

from __future__ import annotations

import inspect
import logging
from pathlib import Path
from typing import TYPE_CHECKING, Any

import torch

from michelangelo.lib.evaluator.chart_data import apply_filter_expr
from michelangelo.lib.evaluator.config import (
    load_config_from_dict,
    load_config_from_yaml,
)
from michelangelo.lib.evaluator.metrics import (
    create_metric_collections,
    extract_data_for_collection,
    metric_state_for_logging,
)
from michelangelo.workflow.schema.evaluator import EvaluatorConfig

if TYPE_CHECKING:
    import pandas as pd

    from michelangelo.workflow.schema.evaluator import ColumnMapping

__all__ = ["TorchMetricEvaluator", "create_evaluator"]

_logger = logging.getLogger(__name__)

_AGGREGATION_METRIC_NAMES = frozenset(
    {"MeanMetric", "SumMetric", "MinMetric", "MaxMetric"}
)


def _is_aggregation_only(collection: Any) -> bool:
    """Report whether every metric in a collection consumes predictions only.

    Args:
        collection: The ``MetricCollection`` to inspect.

    Returns:
        Whether the collection holds nothing but aggregation metrics.
    """
    try:
        from torchmetrics.aggregation import (
            MaxMetric,
            MeanMetric,
            MinMetric,
            SumMetric,
        )

        aggregations = (MeanMetric, SumMetric, MinMetric, MaxMetric)
        return all(isinstance(metric, aggregations) for metric in collection.values())
    except ImportError:
        # Fall back to name comparison if the aggregation module ever moves.
        return all(
            type(metric).__name__ in _AGGREGATION_METRIC_NAMES
            for metric in collection.values()
        )


def _update_collection(collection: Any, data: dict[str, torch.Tensor]) -> None:
    """Feed one batch of extracted tensors into every metric in a collection.

    Args:
        collection: The ``MetricCollection`` to update.
        data: Tensors from :func:`extract_data_for_collection`.
    """
    extra_kwargs = data.get("extra_cols", {})

    if _is_aggregation_only(collection):
        for metric in collection.values():
            metric.update(data["predictions"])
        return

    has_custom_metric = any(
        not type(metric).__module__.startswith("torchmetrics")
        for metric in collection.values()
    )
    if extra_kwargs and has_custom_metric:
        # Forward extra_cols only to custom (non-torchmetrics) metrics whose
        # update() accepts **kwargs. Standard TorchMetrics are always called
        # without extra arguments, even when their signature happens to have
        # **kwargs.
        base_args = [data["predictions"], data["targets"]]
        if "indexes" in data:
            base_args.append(data["indexes"])
        for metric in collection.values():
            if type(metric).__module__.startswith("torchmetrics"):
                metric.update(*base_args)
                continue
            sig = inspect.signature(metric.update)
            accepts_var_keyword = any(
                param.kind == inspect.Parameter.VAR_KEYWORD
                for param in sig.parameters.values()
            )
            if accepts_var_keyword:
                metric.update(*base_args, **extra_kwargs)
            else:
                metric.update(*base_args)
    elif "indexes" in data:
        collection.update(data["predictions"], data["targets"], data["indexes"])
    else:
        collection.update(data["predictions"], data["targets"])


def _scalarize(collection_results: dict[str, Any]) -> dict[str, Any]:
    """Unwrap single-element tensors into Python scalars.

    Args:
        collection_results: Raw output of ``MetricCollection.compute()``.

    Returns:
        The same mapping, with 1-element tensors replaced by their values.
        Multi-element tensors -- a per-class AUROC, say -- are left alone.
    """
    results = {}
    for key, value in collection_results.items():
        if isinstance(value, torch.Tensor) and value.numel() == 1:
            results[key] = value.item()
        else:
            results[key] = value
    return results


class TorchMetricEvaluator:
    """Evaluate a pandas DataFrame against a config's metrics.

    Metrics are organized into TorchMetrics ``MetricCollection``s by their
    column configuration, filter, and tensor dtypes, so every metric that wants
    the same data computes from one extraction.
    """

    def __init__(self, config: EvaluatorConfig | dict[str, Any] | str | Path):
        """Initialize the evaluator.

        Args:
            config: An :class:`EvaluatorConfig`, a dict in the same shape, or a
                path to a YAML config file.

        Raises:
            TypeError: If ``config`` is none of those.
        """
        if isinstance(config, EvaluatorConfig):
            self.config = config
        elif isinstance(config, (str, Path)):
            self.config = load_config_from_yaml(str(config))
        elif isinstance(config, dict):
            self.config = load_config_from_dict(config)
        else:
            raise TypeError(f"Invalid config type: {type(config)}")

        # Collections are keyed by (columns, filter, pred_dtype, target_dtype);
        # dtype_map mirrors those keys with the resolved dtype pair used to
        # extract tensors.
        self.collections, self.dtype_map = create_metric_collections(self.config)

        _logger.info(
            "TorchMetricEvaluator initialized with %d metric collections",
            len(self.collections),
        )

    def evaluate(self, df: pd.DataFrame) -> dict[str, Any]:
        """Compute every configured metric over a DataFrame.

        Each collection is updated, computed, and reset in turn, so metric state
        for all collections is never held at once.

        Args:
            df: The rows to evaluate.

        Returns:
            Metric name -> computed value.
        """
        results: dict[str, Any] = {}

        # Collections routinely share a filter_expr; query each distinct one
        # once. The cache is only valid for `df`, so it is scoped to this call.
        filter_cache: dict[str, pd.DataFrame | None] = {}

        for collection_key, collection in self.collections.items():
            column_mapping, filter_expr, pred_dtype, target_dtype = collection_key
            if column_mapping is None:
                continue

            collection.reset()

            _logger.debug(
                "Processing collection: pred_dtype=%s, target_dtype=%s, "
                "columns=%s, filter_expr=%s, metrics=%s",
                pred_dtype,
                target_dtype,
                column_mapping,
                filter_expr,
                list(collection.keys()),
            )

            # Apply the per-metric filter before extracting tensors. The default
            # on_error="raise" makes an unfilterable metric a hard error.
            df_filtered = apply_filter_expr(df, filter_expr, cache=filter_cache)
            if filter_expr:
                _logger.debug(
                    "Applied filter_expr %r: %d -> %d rows",
                    filter_expr,
                    len(df),
                    len(df_filtered),
                )

            data = extract_data_for_collection(
                df_filtered, column_mapping, pred_dtype, target_dtype
            )
            _update_collection(collection, data)
            del data  # Free the tensors now that the update has consumed them.

            results.update(_scalarize(collection.compute()))
            # Free internal state; retrieval metrics hold every sample.
            collection.reset()

        return results

    def compute(self) -> dict[str, Any]:
        """Compute every collection from its currently accumulated state.

        Use this when collections were updated incrementally rather than through
        :meth:`evaluate`.

        Returns:
            Metric name -> computed value.
        """
        results: dict[str, Any] = {}
        for collection in self.collections.values():
            # TorchMetrics registers state with persistent=False, so
            # state_dict() is empty; read the buffers directly instead.
            if _logger.isEnabledFor(logging.DEBUG):
                for name, metric in collection.items():
                    _logger.debug(
                        "metric=%s state=%s", name, metric_state_for_logging(metric)
                    )
            results.update(_scalarize(collection.compute()))

        return results

    def get_metric_names(self) -> list[str]:
        """List every metric name this evaluator will compute.

        Returns:
            The metric names, in collection order.
        """
        names: list[str] = []
        for collection in self.collections.values():
            names.extend(list(collection.keys()))
        return names

    def validate_dataset(self, df: pd.DataFrame) -> None:
        """Check that a DataFrame carries every column the metrics need.

        Args:
            df: The DataFrame to validate.

        Raises:
            ValueError: If any required column is missing, naming all of them.
        """
        available_columns = set(df.columns)
        missing_columns = []

        for collection_key in self.collections:
            column_mapping = collection_key[0]
            if column_mapping is None:
                continue

            if column_mapping.prediction_col not in available_columns:
                missing_columns.append(
                    f"prediction_col:{column_mapping.prediction_col}"
                )
            if column_mapping.target_col not in available_columns:
                missing_columns.append(f"target_col:{column_mapping.target_col}")
            if (
                column_mapping.index_col
                and column_mapping.index_col not in available_columns
            ):
                missing_columns.append(f"index_col:{column_mapping.index_col}")
            if (
                column_mapping.sample_weight_col
                and column_mapping.sample_weight_col not in available_columns
            ):
                missing_columns.append(
                    f"sample_weight_col:{column_mapping.sample_weight_col}"
                )

        if missing_columns:
            raise ValueError(f"Missing required columns: {missing_columns}")

    def get_required_columns(self) -> set[str]:
        """Collect every column the configured metrics read.

        Returns:
            The column names that must be present in the input data, including
            the config's segment columns.
        """
        columns: set[str] = set()

        for collection_key in self.collections:
            column_mapping = collection_key[0]
            if column_mapping is None:
                continue

            columns.add(column_mapping.prediction_col)
            columns.add(column_mapping.target_col)
            if column_mapping.index_col:
                columns.add(column_mapping.index_col)
            if column_mapping.sample_weight_col:
                columns.add(column_mapping.sample_weight_col)
            if column_mapping.extra_cols:
                columns.update(column_mapping.extra_cols.values())

        if self.config.segment_columns:
            columns = columns.union(set(self.config.segment_columns))

        return columns

    def get_column_mappings(self) -> list[ColumnMapping]:
        """List the column mappings the configured metrics use.

        Returns:
            One mapping per collection that has one.
        """
        return [key[0] for key in self.collections if key[0] is not None]

    def __repr__(self) -> str:
        """Render the evaluator's metric and collection counts."""
        metric_count = sum(
            len(list(collection.keys())) for collection in self.collections.values()
        )
        return (
            f"TorchMetricEvaluator(metrics={metric_count}, "
            f"collections={len(self.collections)})"
        )


def create_evaluator(
    config: EvaluatorConfig | dict[str, Any] | str | Path,
) -> TorchMetricEvaluator:
    """Create a :class:`TorchMetricEvaluator`.

    Args:
        config: An :class:`EvaluatorConfig`, a dict in the same shape, or a path
            to a YAML config file.

    Returns:
        The initialized evaluator.
    """
    return TorchMetricEvaluator(config)
