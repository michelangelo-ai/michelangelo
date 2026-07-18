"""Tests for ``michelangelo.lib.constants.sentinel``."""

from __future__ import annotations

import numpy as np

from michelangelo.lib.constants.sentinel import FLOAT_SENTINEL, INT32_SENTINEL


class TestSentinelValues:
    """Tests for the raw sentinel constant values."""

    def test_float_sentinel_is_nan(self):
        """The float sentinel is NaN."""
        assert np.isnan(FLOAT_SENTINEL)

    def test_int32_sentinel_value(self):
        """The int32 sentinel is the minimum int32 value."""
        assert INT32_SENTINEL == -(2**31)
