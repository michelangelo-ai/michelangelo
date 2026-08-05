"""Combine feature-package and model sample data into e2e sample data for serving."""

from typing import Optional


def build_e2e_sample_data(
    feature_sample_data: Optional[list[dict]],
    model_sample_data: Optional[list[dict]],
    e2e_input_cols: set[str],
) -> list[dict]:
    """Merge feature-package (raw) + model sample data, filtered to e2e input schema columns.

    When a key appears in both, feature_sample_data wins - it holds the raw, pre-derivation
    values that serving callers actually supply.
    """
    merged: dict = {}
    for sample in model_sample_data or []:
        merged.update(sample)
    for sample in feature_sample_data or []:
        merged.update(sample)
    return [{k: v for k, v in merged.items() if k in e2e_input_cols}]
