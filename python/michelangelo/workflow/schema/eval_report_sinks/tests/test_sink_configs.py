"""Tests for EvalReportSink config dataclasses."""

from __future__ import annotations

from unittest import TestCase

from michelangelo.workflow.schema.eval_report_sinks.api import GRPCEvalReportSinkConfig
from michelangelo.workflow.schema.eval_report_sinks.local_file import (
    LocalFileEvalReportSinkConfig,
)
from michelangelo.workflow.schema.eval_report_sinks.result import EvalReportSinkResult
from michelangelo.workflow.schema.exceptions import ConfigurationError


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


class TestGRPCEvalReportSinkConfig(TestCase):
    """Tests for GRPCEvalReportSinkConfig."""

    def test_required_endpoint(self):
        """It stores the endpoint."""
        cfg = GRPCEvalReportSinkConfig(endpoint="localhost:50051")
        self.assertEqual(cfg.endpoint, "localhost:50051")

    def test_raises_on_empty_endpoint(self):
        """It raises ConfigurationError when endpoint is empty."""
        with self.assertRaises(ConfigurationError):
            GRPCEvalReportSinkConfig(endpoint="")

    def test_defaults(self):
        """It defaults to insecure=True, no namespace, timeout 30s."""
        cfg = GRPCEvalReportSinkConfig(endpoint="localhost:50051")
        self.assertTrue(cfg.insecure)
        self.assertEqual(cfg.namespace, "")
        self.assertEqual(cfg.timeout_seconds, 30)

    def test_tls_config(self):
        """It accepts insecure=False for TLS connections."""
        cfg = GRPCEvalReportSinkConfig(
            endpoint="eval-reports.example.com:443",
            namespace="ml-prod",
            insecure=False,
        )
        self.assertFalse(cfg.insecure)
        self.assertEqual(cfg.namespace, "ml-prod")
