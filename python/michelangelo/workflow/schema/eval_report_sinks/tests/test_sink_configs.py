"""Tests for EvalReportSink config dataclasses."""

from __future__ import annotations

from unittest import TestCase

from michelangelo.workflow.schema.eval_report_sinks.local_file import (
    LocalFileEvalReportSinkConfig,
)
from michelangelo.workflow.schema.eval_report_sinks.result import EvalReportSinkResult


class TestEvalReportSinkResult(TestCase):
    """Tests for EvalReportSinkResult."""

    def test_requires_name_and_namespace(self):
        """It stores name and namespace as required fields."""
        r = EvalReportSinkResult(name="r1", namespace="ns")
        self.assertEqual(r.name, "r1")
        self.assertEqual(r.namespace, "ns")

    def test_output_path_defaults_to_empty(self):
        """It defaults output_path to empty string."""
        r = EvalReportSinkResult(name="r1", namespace="")
        self.assertEqual(r.output_path, "")

    def test_extra_defaults_to_empty_dict(self):
        """It defaults extra to an empty dict."""
        r = EvalReportSinkResult(name="r1", namespace="")
        self.assertEqual(r.extra, {})

    def test_is_frozen(self):
        """It raises FrozenInstanceError when a field is assigned after construction."""
        from dataclasses import FrozenInstanceError

        r = EvalReportSinkResult(name="r1", namespace="")
        with self.assertRaises(FrozenInstanceError):
            r.name = "changed"  # type: ignore[misc]


class TestLocalFileEvalReportSinkConfig(TestCase):
    """Tests for LocalFileEvalReportSinkConfig."""

    def test_output_dir_defaults_to_none(self):
        """It defaults output_dir to None (tempdir created at write time)."""
        cfg = LocalFileEvalReportSinkConfig()
        self.assertIsNone(cfg.output_dir)

    def test_explicit_output_dir(self):
        """It stores an explicit output_dir."""
        cfg = LocalFileEvalReportSinkConfig(output_dir="/tmp/reports")
        self.assertEqual(cfg.output_dir, "/tmp/reports")
