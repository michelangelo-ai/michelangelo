"""Fused sample data from native-transform + predictor model metadata."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    import numpy as np


def fuse_sample_data(
    native_transform_input: list[dict[str, np.ndarray]] | None,
    predictor_input: list[dict[str, np.ndarray]] | None,
    columns_to_keep: list[str] | None = None,
) -> list[dict[str, Any]]:
    """Fuse sample data from a native-transform model and its predictor.

    Rows are paired by position (``zip``): the fused row set is truncated to
    the shorter of the two inputs. For each retained column, the
    native-transform row wins on a name collision.

    Args:
        native_transform_input: Sample data rows from the native-transform
            model, or ``None``/empty when there is no transform stage.
        predictor_input: Sample data rows from the predictor model, or
            ``None``/empty.
        columns_to_keep: Column names to include in each fused row. Defaults
            to the union of both inputs' first-row keys when ``None``.

    Returns:
        A list of fused sample rows. When either side is empty, the two
        row lists are concatenated as-is (no fusion is attempted).

    Raises:
        ValueError: If a column in ``columns_to_keep`` is present in neither
            the transform row nor the predictor row for a given pair.
    """
    tx_rows = list(native_transform_input or [])
    pred_rows = list(predictor_input or [])

    if not tx_rows or not pred_rows:
        return [*tx_rows, *pred_rows]

    columns_to_keep = (
        columns_to_keep
        if columns_to_keep is not None
        else list(set(tx_rows[0].keys()) | set(pred_rows[0].keys()))
    )

    out: list[dict[str, Any]] = []

    for tx_row, pred_row in zip(tx_rows, pred_rows):
        row: dict[str, Any] = {}
        for col in columns_to_keep:
            if col in tx_row:
                arr = tx_row[col]
            elif col in pred_row:
                arr = pred_row[col]
            else:
                raise ValueError(
                    f"Fused sample data missing input {col!r}: not in native "
                    "transform or predictor sample_data row."
                )
            row[col] = arr
        out.append(row)

    return out
