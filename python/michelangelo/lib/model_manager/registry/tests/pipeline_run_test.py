"""Tests for the pipeline-run provenance accessor."""

from __future__ import annotations

import os
from unittest import TestCase
from unittest.mock import patch

from michelangelo.lib.model_manager.registry import pipeline_run
from michelangelo.lib.model_manager.registry.pipeline_run import (
    SourcePipelineRun,
    _reset_source_pipeline_run_cache,
    get_source_pipeline_run,
)


class TestGetSourcePipelineRun(TestCase):
    """Tests for get_source_pipeline_run()."""

    def setUp(self) -> None:
        """Ensure each test starts with a fresh, unpopulated cache."""
        _reset_source_pipeline_run_cache()
        self.addCleanup(_reset_source_pipeline_run_cache)

    def test_both_env_vars_set(self):
        """Both env vars set returns a fully populated SourcePipelineRun."""
        with patch.dict(
            os.environ,
            {"MA_PIPELINE_RUN_NAME": "run-1", "MA_NAMESPACE": "ns-1"},
            clear=False,
        ):
            result = get_source_pipeline_run()
        self.assertEqual(result, SourcePipelineRun(name="run-1", namespace="ns-1"))

    def test_only_run_name_set(self):
        """Only MA_PIPELINE_RUN_NAME set returns a run with namespace=None."""
        with patch.dict(os.environ, {"MA_PIPELINE_RUN_NAME": "run-1"}, clear=False):
            os.environ.pop("MA_NAMESPACE", None)
            result = get_source_pipeline_run()
        self.assertEqual(result, SourcePipelineRun(name="run-1", namespace=None))

    def test_only_namespace_set_returns_none(self):
        """Only MA_NAMESPACE set (no run name) returns None.

        A namespace alone is not a pipeline-run reference.
        """
        with patch.dict(os.environ, {"MA_NAMESPACE": "ns-1"}, clear=False):
            os.environ.pop("MA_PIPELINE_RUN_NAME", None)
            result = get_source_pipeline_run()
        self.assertIsNone(result)

    def test_neither_set_returns_none(self):
        """Neither env var set (local dev / outside pipeline execution) returns None."""
        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("MA_PIPELINE_RUN_NAME", None)
            os.environ.pop("MA_NAMESPACE", None)
            result = get_source_pipeline_run()
        self.assertIsNone(result)

    def test_empty_string_run_name_treated_as_unset(self):
        """An empty-string MA_PIPELINE_RUN_NAME is treated the same as unset."""
        with patch.dict(
            os.environ,
            {"MA_PIPELINE_RUN_NAME": "", "MA_NAMESPACE": "ns-1"},
            clear=False,
        ):
            result = get_source_pipeline_run()
        self.assertIsNone(result)

    def test_result_is_cached_across_calls(self):
        """The value is read once and cached — a later env change is not observed."""
        with patch.dict(os.environ, {"MA_PIPELINE_RUN_NAME": "run-1"}, clear=False):
            os.environ.pop("MA_NAMESPACE", None)
            first = get_source_pipeline_run()
            os.environ["MA_PIPELINE_RUN_NAME"] = "run-2"
            second = get_source_pipeline_run()
        self.assertEqual(first, SourcePipelineRun(name="run-1", namespace=None))
        self.assertEqual(second, first)

    def test_reset_cache_forces_re_read(self):
        """The reset hook forces the next call to re-read the environment."""
        with patch.dict(os.environ, {"MA_PIPELINE_RUN_NAME": "run-1"}, clear=False):
            os.environ.pop("MA_NAMESPACE", None)
            first = get_source_pipeline_run()
            os.environ["MA_PIPELINE_RUN_NAME"] = "run-2"
            _reset_source_pipeline_run_cache()
            second = get_source_pipeline_run()
        self.assertEqual(first, SourcePipelineRun(name="run-1", namespace=None))
        self.assertEqual(second, SourcePipelineRun(name="run-2", namespace=None))

    def test_none_result_is_also_cached(self):
        """A None result (no run name) is cached too, not re-derived every call."""
        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("MA_PIPELINE_RUN_NAME", None)
            first = get_source_pipeline_run()
            os.environ["MA_PIPELINE_RUN_NAME"] = "run-1"
            second = get_source_pipeline_run()
        self.assertIsNone(first)
        self.assertIsNone(second)

    def test_source_pipeline_run_is_frozen(self):
        """SourcePipelineRun instances are immutable."""
        from dataclasses import FrozenInstanceError

        run = SourcePipelineRun(name="run-1")
        with self.assertRaises(FrozenInstanceError):
            run.name = "run-2"  # type: ignore[misc]

    def test_source_pipeline_run_namespace_defaults_to_none(self):
        """SourcePipelineRun.namespace defaults to None when not provided."""
        run = SourcePipelineRun(name="run-1")
        self.assertIsNone(run.namespace)

    def test_cache_starts_unset(self):
        """After a reset, the module-level cache is the _Unset sentinel."""
        self.assertIsInstance(pipeline_run._cache, pipeline_run._Unset)
