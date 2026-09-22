"""Metric instantiation, dtype resolution, and tensor extraction.

Two metrics share a ``MetricCollection`` only when they want the same columns,
the same filter, *and* the same tensor dtypes. Dtype is resolved per metric --
from an explicit ``_dtypes`` attribute, from a ``_metric_type`` tag, or by
inferring the task from the metric's import path -- so a regression metric
cannot land in a collection of classification metrics that happen to read the
same columns and silently cast their targets to float.
"""

from __future__ import annotations

import contextlib
import importlib
import inspect
import logging
from typing import TYPE_CHECKING, Any

import numpy as np
import pandas as pd
import torch

if TYPE_CHECKING:
    from michelangelo.workflow.schema.evaluator import ColumnMapping, EvaluatorConfig

__all__ = [
    "TASK_TYPE_TO_DTYPES",
    "cast_array_to_dtype",
    "create_metric_collections",
    "create_metric_from_path",
    "encode_categorical_column_to_tensor",
    "extract_data_for_collection",
    "infer_task_type_from_path",
    "metric_state_for_logging",
    "resolve_dtypes_for_metric",
]

_logger = logging.getLogger(__name__)


# User-facing shorthand for tagging a metric's task type. Each entry resolves
# to (pred_dtype, target_dtype). target_dtype=None means the metric does not
# consume a target column (e.g. aggregation metrics).
#
# Custom metrics declare intent via a `_metric_type` class attribute; the
# bundling layer then keys collections by the resolved dtypes, not the tag.
TASK_TYPE_TO_DTYPES: dict[str, tuple[torch.dtype, torch.dtype | None]] = {
    "regression": (torch.float32, torch.float32),
    "classification": (torch.float32, torch.long),
    "retrieval": (torch.float32, torch.long),
    "aggregation": (torch.float32, None),
}


def cast_array_to_dtype(array_data: np.ndarray, dtype: torch.dtype) -> torch.Tensor:
    """Convert a numpy array to a torch tensor with an explicit dtype.

    Args:
        array_data: Source array. Copied, so the tensor never aliases a
            DataFrame column.
        dtype: Target torch dtype.

    Returns:
        The converted tensor.
    """
    return torch.from_numpy(array_data.copy()).to(dtype)


def encode_categorical_column_to_tensor(
    column_data: np.ndarray,
    target_dtype: torch.dtype = torch.long,
    factorize_strings: bool = True,
) -> torch.Tensor:
    """Encode a categorical column to a torch tensor, handling string data.

    Args:
        column_data: Input numpy array, typically a DataFrame column.
        target_dtype: Target torch dtype (e.g. ``torch.long``, ``torch.float``).
        factorize_strings: Convert string/object data to numeric via
            ``pandas.factorize``, which gives each distinct string a stable
            integer code.

    Returns:
        The converted tensor.

    Example:
        >>> encode_categorical_column_to_tensor(np.array(["a", "b", "a", "c"]))
        tensor([0, 1, 0, 2])
        >>> encode_categorical_column_to_tensor(
        ...     np.array([1.0, 2.5]), torch.float, factorize_strings=False
        ... )
        tensor([1.0000, 2.5000])
    """
    is_string_type = column_data.dtype == np.object_ or column_data.dtype.kind in (
        "U",
        "S",
        "O",
    )

    if is_string_type and factorize_strings:
        # factorize returns (codes, uniques); the codes are a 1:1 string -> int map.
        numeric_data, _ = pd.factorize(column_data)
        return torch.from_numpy(numeric_data.copy()).to(target_dtype)
    return torch.from_numpy(column_data.copy()).to(target_dtype)


def _log_data_dtype_details(
    pred_dtype: torch.dtype,
    target_dtype: torch.dtype | None,
    data: dict[str, torch.Tensor],
) -> None:
    """Log resolved dtypes and tensor shapes for a collection's extracted data."""
    target_shape = data["targets"].shape if "targets" in data else None
    target_actual = data["targets"].dtype if "targets" in data else None
    _logger.info(
        "Extracted data with pred_dtype=%s, target_dtype=%s: "
        "pred(dtype=%s, shape=%s), target(dtype=%s, shape=%s)",
        pred_dtype,
        target_dtype,
        data["predictions"].dtype,
        data["predictions"].shape,
        target_actual,
        target_shape,
    )


def extract_data_for_collection(
    df: pd.DataFrame,
    column_mapping: ColumnMapping,
    pred_dtype: torch.dtype,
    target_dtype: torch.dtype | None,
) -> dict[str, torch.Tensor]:
    """Extract the tensors one ``MetricCollection`` needs from a DataFrame.

    Casting is driven by the explicit ``(pred_dtype, target_dtype)`` pair the
    bundling layer resolved, so this function never has to pick a default.

    Args:
        df: Rows to extract from, already filtered by the metric's
            ``filter_expr``.
        column_mapping: Which columns feed predictions, targets, and extras.
        pred_dtype: Dtype for the prediction tensor.
        target_dtype: Dtype for the target tensor. ``None`` marks an
            aggregation-style collection that does not read the target column.

    Returns:
        A dict with ``predictions``, and optionally ``targets``, ``indexes``,
        ``sample_weights``, and ``extra_cols``.
    """
    data = {}

    pred_array = df[column_mapping.prediction_col].values
    if len(pred_array) > 0 and isinstance(pred_array[0], (list, np.ndarray)):
        pred_array = np.vstack(pred_array)
    data["predictions"] = cast_array_to_dtype(pred_array, pred_dtype)

    if target_dtype is not None:
        target_array = df[column_mapping.target_col].values
        if len(target_array) > 0 and isinstance(target_array[0], (list, np.ndarray)):
            target_array = np.vstack(target_array)
        data["targets"] = cast_array_to_dtype(target_array, target_dtype)

    _log_data_dtype_details(pred_dtype, target_dtype, data)

    if column_mapping.index_col and column_mapping.index_col in df.columns:
        data["indexes"] = encode_categorical_column_to_tensor(
            df[column_mapping.index_col].values,
            target_dtype=torch.long,
            factorize_strings=True,
        )

    if (
        column_mapping.sample_weight_col
        and column_mapping.sample_weight_col in df.columns
    ):
        data["sample_weights"] = encode_categorical_column_to_tensor(
            df[column_mapping.sample_weight_col].values,
            target_dtype=torch.float,
            factorize_strings=False,
        )

    if column_mapping.extra_cols:
        extra = {}
        for logical_name, col_name in column_mapping.extra_cols.items():
            if col_name in df.columns:
                extra[logical_name] = torch.from_numpy(
                    df[col_name].values.copy()
                ).float()
        if extra:
            data["extra_cols"] = extra

    return data


def infer_task_type_from_path(metric_path: str) -> str:
    """Infer a metric's task type from its import path.

    Args:
        metric_path: Dotted import path of the metric class.

    Returns:
        One of ``"regression"``, ``"classification"``, ``"retrieval"``,
        ``"aggregation"``. Unknown paths fall back to ``"classification"``;
        tag the class with ``_metric_type`` to override.

    Example:
        >>> infer_task_type_from_path("torchmetrics.classification.Accuracy")
        'classification'
        >>> infer_task_type_from_path("torchmetrics.retrieval.RetrievalMAP")
        'retrieval'
    """
    for token, task in (
        (".classification.", "classification"),
        (".regression.", "regression"),
        (".retrieval.", "retrieval"),
        (".aggregation.", "aggregation"),
    ):
        if token in metric_path:
            return task

    # Resolve the class and check its module hierarchy, which handles shorthand
    # paths like torchmetrics.MeanSquaredError.
    try:
        module_path, class_name = metric_path.rsplit(".", 1)
        module = importlib.import_module(module_path)
        metric_class = getattr(module, class_name)
        actual_module = getattr(metric_class, "__module__", "") or ""
        for token, task in (
            (".regression", "regression"),
            (".classification", "classification"),
            (".retrieval", "retrieval"),
            (".aggregation", "aggregation"),
        ):
            if token in actual_module:
                return task
    except Exception as err:
        _logger.debug(
            "Could not resolve metric class for path %r to infer task type "
            "(%s: %s); falling back to 'classification'",
            metric_path,
            type(err).__name__,
            err,
        )

    return "classification"


def resolve_dtypes_for_metric(
    metric_path: str,
    metric_instance: Any,
) -> tuple[torch.dtype, torch.dtype | None]:
    """Resolve the ``(pred_dtype, target_dtype)`` pair for one metric.

    Resolution order:

    1. A ``_dtypes`` class attribute -- a direct override for combinations no
       task-type shorthand covers.
    2. A ``_metric_type`` class attribute naming a key of
       :data:`TASK_TYPE_TO_DTYPES`. This is the recommended way for a custom
       metric to declare intent.
    3. Path-based task inference, mapped through :data:`TASK_TYPE_TO_DTYPES`.

    Args:
        metric_path: Dotted import path the metric was created from.
        metric_instance: The instantiated metric.

    Returns:
        The resolved dtype pair. ``target_dtype`` is ``None`` for aggregation
        metrics.
    """
    cls = type(metric_instance)

    explicit_dtypes = getattr(cls, "_dtypes", None)
    if explicit_dtypes is not None:
        _logger.debug(
            "Resolved dtypes for %r via explicit `_dtypes` on %s: %s",
            metric_path,
            cls.__name__,
            explicit_dtypes,
        )
        return explicit_dtypes

    explicit_type = getattr(cls, "_metric_type", None)
    if explicit_type in TASK_TYPE_TO_DTYPES:
        dtypes = TASK_TYPE_TO_DTYPES[explicit_type]
        _logger.debug(
            "Resolved dtypes for %r via `_metric_type=%r` on %s: %s",
            metric_path,
            explicit_type,
            cls.__name__,
            dtypes,
        )
        return dtypes

    inferred_task = infer_task_type_from_path(metric_path)
    dtypes = TASK_TYPE_TO_DTYPES[inferred_task]
    _logger.debug(
        "Resolved dtypes for %r via path inference (task=%r): %s",
        metric_path,
        inferred_task,
        dtypes,
    )
    return dtypes


def create_metric_from_path(metric_path: str, params: dict[str, Any]) -> Any:
    """Create a metric instance from a dotted import path.

    Args:
        metric_path: Import path such as
            ``"torchmetrics.classification.MulticlassAUROC"``.
        params: Keyword arguments for the metric constructor.

    Returns:
        The instantiated metric.
    """
    module_path, class_name = metric_path.rsplit(".", 1)
    module = importlib.import_module(module_path)
    metric_class = getattr(module, class_name)

    # Coerce ints and numeric strings to float for parameters whose signature
    # default is a float. A YAML float like 2.0 loses its type through proto
    # Struct serialization -- arriving as int 2, or as the literal "2.0e0" when
    # YAML 1.1 misparses scientific notation -- and torchmetrics validators
    # (e.g. FBetaScore.beta) do strict isinstance(x, float) checks.
    coerced = dict(params)
    try:
        sig = inspect.signature(metric_class)
        for name, parameter in sig.parameters.items():
            if name not in coerced or not isinstance(parameter.default, float):
                continue
            value = coerced[name]
            if isinstance(value, bool):
                continue
            if isinstance(value, int):
                coerced[name] = float(value)
            elif isinstance(value, str):
                with contextlib.suppress(ValueError):
                    coerced[name] = float(value)
    except (TypeError, ValueError):
        pass

    return metric_class(**coerced)


def metric_state_for_logging(metric: Any) -> dict[str, Any]:
    """Read a metric's accumulated internal state, for debug logging.

    TorchMetrics registers state via ``add_state(persistent=False)``, so
    ``state_dict()`` is usually empty. This reads the tensor attributes
    directly instead, surfacing counters like ``tp``, ``fp``, and ``total``.

    Args:
        metric: The metric to inspect.

    Returns:
        Attribute name -> scalar or list value, for every tensor attribute.
    """
    state = {}

    for name in dir(metric):
        if name.startswith("_"):
            continue
        try:
            attr = getattr(metric, name)
            if isinstance(attr, torch.Tensor):
                state[name] = attr.item() if attr.numel() == 1 else attr.tolist()
        except Exception:
            continue

    return state


def create_metric_collections(
    config: EvaluatorConfig,
) -> tuple[dict[tuple, Any], dict[tuple, tuple[torch.dtype, torch.dtype | None]]]:
    """Group the configured metrics into TorchMetrics ``MetricCollection``s.

    The bundling key is ``(ColumnMapping, filter_expr, pred_dtype,
    target_dtype)``. Two metrics co-bundle only when they want the same data
    *and* the same tensor dtypes.

    Args:
        config: Evaluator configuration holding the metric list.

    Returns:
        A ``(collections, dtype_map)`` pair, both keyed by the bundling key.

    Raises:
        ImportError: If torchmetrics is not installed.
    """
    try:
        from torchmetrics import MetricCollection
    except ImportError as err:
        raise ImportError(
            "torchmetrics is required by the evaluator but is not installed; "
            "install michelangelo with the 'evaluator' extra."
        ) from err

    grouped: dict[tuple, dict[str, Any]] = {}

    for metric_config in config.metrics:
        metric_instance = create_metric_from_path(
            metric_config.metric, metric_config.params
        )
        pred_dtype, target_dtype = resolve_dtypes_for_metric(
            metric_config.metric, metric_instance
        )
        collection_key = (
            metric_config.columns,
            metric_config.filter_expr,
            pred_dtype,
            target_dtype,
        )
        grouped.setdefault(collection_key, {})[metric_config.name] = metric_instance

    metric_collections: dict[tuple, Any] = {}
    dtype_map: dict[tuple, tuple[torch.dtype, torch.dtype | None]] = {}

    for collection_key, metrics in grouped.items():
        metric_collections[collection_key] = MetricCollection(metrics)
        dtype_map[collection_key] = (collection_key[2], collection_key[3])
        _logger.debug(
            "Created MetricCollection with pred_dtype=%s, target_dtype=%s, "
            "num_metrics=%d, columns=%s, filter_expr=%s",
            collection_key[2],
            collection_key[3],
            len(metrics),
            collection_key[0],
            collection_key[1],
        )

    return metric_collections, dtype_map
