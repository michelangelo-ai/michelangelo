"""Tests for michelangelo.workflow.tasks.evaluator.preprocessor."""

from __future__ import annotations

from unittest import TestCase

import pytest

from michelangelo.workflow.tasks.evaluator.preprocessor import declare_inputs


class DeclareInputsTest(TestCase):
    """Tests for the declare_inputs decorator."""

    def test_attaches_input_columns_to_callable(self):
        """The decorated function carries the declared columns."""

        @declare_inputs(["a", "b"])
        def fn(df):
            return df

        assert fn.input_columns == ["a", "b"]

    def test_returns_the_same_function_object(self):
        """Decoration does not wrap or replace the function."""

        def fn(df):
            return df

        assert declare_inputs(["a"])(fn) is fn

    def test_copies_the_columns_list(self):
        """Mutating the caller's list afterwards does not change the declaration."""
        columns = ["a", "b"]

        @declare_inputs(columns)
        def fn(df):
            return df

        columns.append("c")
        assert fn.input_columns == ["a", "b"]

    def test_empty_columns_raises(self):
        """An empty declaration would project every row to zero columns."""
        with pytest.raises(ValueError, match="non-empty"):
            declare_inputs([])

    def test_decorated_function_still_runs(self):
        """The decorator leaves call behaviour untouched."""

        @declare_inputs(["a"])
        def double(value):
            return value * 2

        assert double(3) == 6
