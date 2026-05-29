"""Tests for EvalReportSink implementations."""

from __future__ import annotations

import json
import os
import shutil
import sys
from typing import Any
from unittest import TestCase
from unittest.mock import MagicMock, patch

from michelangelo.gen.api.v2.evaluation_report_pb2 import (
    EvaluationReport,
    EvaluationReportSpec,
)
from michelangelo.workflow.schema.eval_report_sinks.api import GRPCEvalReportSinkConfig
from michelangelo.workflow.schema.eval_report_sinks.local_file import (
    LocalFileEvalReportSinkConfig,
)
from michelangelo.workflow.tasks.functions.eval_report_sinks.base import EvalReportSink
from michelangelo.workflow.tasks.functions.eval_report_sinks.local_file import (
    LocalFileEvalReportSink,
)


def _report(name: str = "test-report", namespace: str = "") -> EvaluationReport:
    """Build a minimal EvaluationReport for test use."""
    r = EvaluationReport(spec=EvaluationReportSpec(title="Test"))
    r.metadata.name = name
    r.metadata.namespace = namespace
    return r


class TestEvalReportSinkABC(TestCase):
    """Tests for the EvalReportSink abstract base class."""

    def test_cannot_instantiate_abstract_base(self):
        """It raises TypeError when EvalReportSink is instantiated directly."""
        with self.assertRaises(TypeError):
            EvalReportSink()  # type: ignore[abstract]


class TestLocalFileEvalReportSink(TestCase):
    """Tests for LocalFileEvalReportSink."""

    def setUp(self) -> None:
        """Track output dirs for cleanup."""
        self._output_dirs: list[str] = []

    def tearDown(self) -> None:
        """Remove temp dirs created during tests."""
        for d in self._output_dirs:
            if os.path.exists(d):
                shutil.rmtree(d)

    def _write(self, report: EvaluationReport, **kwargs: Any):
        sink = LocalFileEvalReportSink(**kwargs)
        result = sink.write(report)
        self._output_dirs.append(os.path.dirname(result.output_path))
        return result

    def test_writes_valid_json_file(self):
        """It writes a valid JSON file and returns the path."""
        result = self._write(_report())
        self.assertTrue(os.path.exists(result.output_path))
        with open(result.output_path) as f:
            doc = json.load(f)
        self.assertIn("spec", doc)

    def test_filename_matches_report_name(self):
        """It names the file after report.metadata.name."""
        result = self._write(_report(name="my-eval"))
        self.assertTrue(result.output_path.endswith("my-eval.json"))

    def test_output_dir_auto_created_when_config_none(self):
        """It creates a michelangelo_reports_ temp dir when no config given."""
        result = self._write(_report())
        parent = os.path.basename(os.path.dirname(result.output_path))
        self.assertTrue(parent.startswith("michelangelo_reports_"))

    def test_explicit_output_dir_used(self):
        """It writes to the configured output_dir."""
        import tempfile

        d = tempfile.mkdtemp()
        self._output_dirs.append(d)
        cfg = LocalFileEvalReportSinkConfig(output_dir=d)
        sink = LocalFileEvalReportSink(cfg)
        result = sink.write(_report())
        self.assertTrue(result.output_path.startswith(d))

    def test_extra_fields_merged_into_json(self):
        """It merges extra_fields into the written JSON document."""
        sink = LocalFileEvalReportSink()
        result = sink.write(_report(), extra_fields={"ci_run": "build-42"})
        self._output_dirs.append(os.path.dirname(result.output_path))
        with open(result.output_path) as f:
            doc = json.load(f)
        self.assertEqual(doc["ci_run"], "build-42")

    def test_result_name_and_namespace(self):
        """It returns the report name and namespace in the result."""
        result = self._write(_report(name="r1", namespace="ns-prod"))
        self.assertEqual(result.name, "r1")
        self.assertEqual(result.namespace, "ns-prod")

    def test_raises_when_name_not_set(self):
        """It raises ValueError when report.metadata.name is empty."""
        report = EvaluationReport(spec=EvaluationReportSpec(title="T"))
        with self.assertRaises(ValueError):
            LocalFileEvalReportSink().write(report)

    def test_snake_case_field_names_in_output(self):
        """It serializes proto fields with snake_case keys."""
        result = self._write(_report())
        with open(result.output_path) as f:
            doc = json.load(f)
        self.assertNotIn("typeMeta", doc)


class TestGRPCEvalReportSink(TestCase):
    """Tests for GRPCEvalReportSink."""

    def _make_created(
        self, report_name: str = "api-report", namespace: str = "ns"
    ) -> EvaluationReport:
        """Build a canned EvaluationReport response proto."""
        created = EvaluationReport()
        created.metadata.name = report_name
        created.metadata.namespace = namespace
        return created

    def _make_sink(self, endpoint: str = "localhost:50051", **kwargs):
        """Build a GRPCEvalReportSink with a mocked _svc.create."""
        from michelangelo.workflow.tasks.functions.eval_report_sinks.api import (
            GRPCEvalReportSink,
        )

        cfg = GRPCEvalReportSinkConfig(endpoint=endpoint, **kwargs)
        with patch("grpc.insecure_channel"), patch("grpc.secure_channel"):
            sink = GRPCEvalReportSink(cfg)
        return sink

    def test_raises_import_error_when_grpcio_missing(self):
        """It raises ImportError when grpcio is not installed."""
        with patch.dict(sys.modules, {"grpc": None}):
            from michelangelo.workflow.tasks.functions.eval_report_sinks.api import (
                GRPCEvalReportSink,
            )

            with self.assertRaises(ImportError):
                GRPCEvalReportSink(GRPCEvalReportSinkConfig(endpoint="localhost:50051"))

    def test_creates_report_via_grpc(self):
        """It delegates to _svc.create and returns an EvalReportSinkResult."""
        sink = self._make_sink()
        created = self._make_created("r1", "ns1")
        sink._svc = MagicMock()
        sink._svc.create.return_value = created

        result = sink.write(_report(name="r1"))

        sink._svc.create.assert_called_once()
        self.assertEqual(result.name, "r1")
        self.assertEqual(result.namespace, "ns1")
        self.assertEqual(result.output_path, "")

    def test_namespace_injected_from_config(self):
        """It sets report.metadata.namespace from config.namespace before create."""
        sink = self._make_sink(namespace="injected-ns")
        created = self._make_created("r1", "injected-ns")
        sink._svc = MagicMock()
        sink._svc.create.return_value = created

        report = _report(name="r1", namespace="")
        sink.write(report)

        self.assertEqual(report.metadata.namespace, "injected-ns")

    def test_grpc_rpc_error_raised_as_oserror(self):
        """It wraps grpc.RpcError as OSError with the endpoint in the message."""
        import grpc

        class _FakeRpcError(grpc.RpcError):
            def code(self):
                return grpc.StatusCode.UNAVAILABLE

            def details(self):
                return "server unreachable"

        sink = self._make_sink()
        sink._svc = MagicMock()
        sink._svc.create.side_effect = _FakeRpcError()

        with self.assertRaises(OSError) as ctx:
            sink.write(_report(name="r1"))
        self.assertIn("localhost:50051", str(ctx.exception))

    def test_default_caller_set_on_context(self):
        """It sets a default rpc-caller; no APIClient.set_caller() needed."""
        from michelangelo.workflow.tasks.functions.eval_report_sinks.api import (
            GRPCEvalReportSink,
        )

        with patch("grpc.insecure_channel"), patch("grpc.secure_channel"):
            sink = GRPCEvalReportSink(
                GRPCEvalReportSinkConfig(endpoint="localhost:50051")
            )
        self.assertIsNotNone(sink._svc._context.header_provider._caller)


class TestFlattenReportToMetrics(TestCase):
    """Tests for flatten_report_to_metrics().

    flatten_report_to_metrics() uses MessageToDict to convert the proto, then
    traverses doc["spec"]["charts"][i]["series"][0]["data_points"][0]["value"].
    Tests mock MessageToDict to exercise the traversal logic without requiring
    exact proto construction for each chart sub-type.
    """

    _MSGTODICT = "google.protobuf.json_format.MessageToDict"

    def _run(self, charts: list) -> dict:
        from michelangelo.workflow.tasks.functions.eval_report_sinks.base import (
            flatten_report_to_metrics,
        )

        doc = {"spec": {"charts": charts}}
        with patch(self._MSGTODICT, return_value=doc):
            return flatten_report_to_metrics(_report())

    def test_empty_report_returns_empty_dict(self):
        """It returns {} when there are no charts."""
        from michelangelo.workflow.tasks.functions.eval_report_sinks.base import (
            flatten_report_to_metrics,
        )

        with patch(self._MSGTODICT, return_value={}):
            self.assertEqual(flatten_report_to_metrics(_report()), {})

    def test_single_scalar_chart_extracted(self):
        """It extracts title→float for a chart with one single-point series."""
        result = self._run(
            [
                {"title": "accuracy", "series": [{"data_points": [{"value": "0.95"}]}]},
            ]
        )
        self.assertAlmostEqual(result["accuracy"], 0.95)

    def test_missing_title_falls_back_to_index(self):
        """It uses metric_<i> when title is absent."""
        result = self._run(
            [
                {"series": [{"data_points": [{"value": "0.8"}]}]},
            ]
        )
        self.assertAlmostEqual(result["metric_0"], 0.8)

    def test_non_numeric_value_silently_skipped(self):
        """It silently drops data points whose value cannot be cast to float."""
        result = self._run(
            [
                {"title": "bad", "series": [{"data_points": [{"value": "n/a"}]}]},
            ]
        )
        self.assertNotIn("bad", result)

    def test_multi_point_series_skipped(self):
        """It skips charts whose series has more than one data point."""
        result = self._run(
            [
                {
                    "title": "loss_curve",
                    "series": [{"data_points": [{"value": "0.9"}, {"value": "0.8"}]}],
                }
            ]
        )
        self.assertNotIn("loss_curve", result)
