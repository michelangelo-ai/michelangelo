"""Tests for the shared scaffold helpers."""

import pytest

from michelangelo.cli.importer import scaffold


@pytest.mark.parametrize(
    ("quantity", "expected"),
    [(None, None), ("500m", 1), ("1500m", 2), ("2", 2), (2, 2), ("2.5", 3), (0.1, 1)],
)
def test_cpu_quantities_round_up(quantity, expected):
    """Cpu quantities round up."""
    assert scaffold.cpu_count(quantity) == expected


def test_gpu_quantities():
    """Gpu quantities."""
    assert scaffold.gpu_count(None) is None
    assert scaffold.gpu_count("2") == 2
    assert scaffold.gpu_count(1) == 1
