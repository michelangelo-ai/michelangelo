"""Declaration helper for pre-evaluation DataFrame preprocessors.

A preprocessor reshapes raw rows into the columns the metrics read -- exploding
a per-session array of predictions into one row per item, say. It runs before
any metric sees the data, which creates a chicken-and-egg problem for column
projection: the evaluator wants to read only the columns it needs from parquet,
but the columns the metrics name do not exist until after the transform. The
:func:`declare_inputs` decorator resolves it by having the preprocessor state
the raw columns it consumes.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Callable

if TYPE_CHECKING:
    import pandas as pd

__all__ = ["PreprocessorFn", "declare_inputs"]


PreprocessorFn = Callable[["pd.DataFrame"], "pd.DataFrame"]


def declare_inputs(columns: list[str]) -> Callable[[PreprocessorFn], PreprocessorFn]:
    """Attach an ``input_columns`` attribute to a pandas preprocessor function.

    The evaluator inspects this attribute on the callable named by
    ``EvaluatorConfig.data_preprocessor`` and pushes the listed columns down
    into the parquet read, so a source dataset with hundreds of unused feature
    columns costs only the handful the preprocessor actually reads.

    Args:
        columns: Raw source column names the preprocessor consumes. Must be
            non-empty -- a preprocessor that declares nothing would project
            every row down to zero columns.

    Returns:
        A decorator that stamps ``input_columns`` onto the function and
        returns it unchanged.

    Raises:
        ValueError: If ``columns`` is empty.

    Example:
        >>> @declare_inputs(["valid_mask", "label_arr", "pred_arr"])
        ... def explode(df):
        ...     return df
        >>> explode.input_columns
        ['valid_mask', 'label_arr', 'pred_arr']
    """
    if not columns:
        raise ValueError("declare_inputs requires a non-empty list of column names")

    def decorator(fn: PreprocessorFn) -> PreprocessorFn:
        # Copied, so a later mutation of the caller's list cannot change what
        # the evaluator reads from disk.
        fn.input_columns = list(columns)
        return fn

    return decorator
