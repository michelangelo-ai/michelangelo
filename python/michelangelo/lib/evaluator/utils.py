"""Shared helpers for the evaluator library."""

from __future__ import annotations

__all__ = ["sample_curve_indices"]


def sample_curve_indices(n: int, max_count: int = 100) -> list[int]:
    """Return indices into a curve's threshold array, capped at ~``max_count``.

    Half the budget is spread evenly, half geometrically over the low-index head.
    Thresholds run high-to-low, so on an imbalanced problem both the ROC and PR
    curves fall most of their height in the first few indices; even spacing alone
    samples that descent once and draws it as a vertical segment. The even half
    preserves tail coverage and the threshold slider.

    Args:
        n: Total number of available threshold points.
        max_count: Approximate budget. The deduplicated union is usually under it
            and can exceed it by at most 2.

    Returns:
        Sorted list of integer indices into the threshold array. The first and
        last index are always included so the slider spans the full
        prediction-value range.

    Example:
        >>> sample_curve_indices(5)
        [0, 1, 2, 3, 4]
        >>> idx = sample_curve_indices(10_000, max_count=10)
        >>> idx[0], idx[-1]
        (0, 9999)
    """
    if n <= max_count:
        return list(range(n))
    half = max(2, max_count // 2)
    even = {round(i * (n - 1) / (half - 1)) for i in range(half)}
    # Geometric from index 1 to n-1; dense where the curve is steep.
    geometric = {round((n - 1) ** (i / (half - 1))) for i in range(half)}
    return sorted(even | geometric | {0, n - 1})
