"""Tests for ``michelangelo.lib._internal.errors``."""

from __future__ import annotations

import pytest

from michelangelo.lib._internal.errors import UserInputError


class TestUserInputError:
    """The ``UserInputError`` exception class."""

    def test_is_exception_subclass(self):
        """``UserInputError`` derives from ``Exception``."""
        assert issubclass(UserInputError, Exception)

    def test_can_be_raised_and_carries_message(self):
        """It can be raised and preserves its message."""
        with pytest.raises(UserInputError, match="bad path"):
            raise UserInputError("bad path")
