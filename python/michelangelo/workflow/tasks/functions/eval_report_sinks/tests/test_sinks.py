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

    _STUB_PATH = (
        "michelangelo.gen.api.v2"
        ".evaluation_report_svc_pb2_grpc.EvaluationReportServiceStub"
    )

    def _make_stub(
        self, report_name: str = "api-report", namespace: str = "ns"
    ) -> MagicMock:
        """Build a mock gRPC stub with a canned CreateEvaluationReport response."""
        stub = MagicMock()
        created = EvaluationReport()
        created.metadata.name = report_name
        created.metadata.namespace = namespace
        resp = MagicMock()
        resp.evaluation_report = created
        stub.CreateEvaluationReport.return_value = resp
        return stub

    def test_raises_import_error_when_grpcio_missing(self):
        """It raises ImportError when grpcio is not installed."""
        with patch.dict(sys.modules, {"grpc": None}):
            from michelangelo.workflow.tasks.functions.eval_report_sinks.api import (
                GRPCEvalReportSink,
            )
            with self.assertRaises(ImportError):
                GRPCEvalReportSink(GRPCEvalReportSinkConfig(endpoint="localhost:50051"))

    def test_creates_report_via_grpc(self):
        """It calls CreateEvaluationReport on the stub and returns the result."""
        from michelangelo.workflow.tasks.functions.eval_report_sinks.api import (
            GRPCEvalReportSink,
        )

        stub = self._make_stub("r1", "ns1")
        with patch(self._STUB_PATH, return_value=stub):
            cfg = GRPCEvalReportSinkConfig(endpoint="localhost:50051")
            sink = GRPCEvalReportSink(cfg)
            result = sink.write(_report(name="r1"))

        stub.CreateEvaluationReport.assert_called_once()
        self.assertEqual(result.name, "r1")
        self.assertEqual(result.namespace, "ns1")
        self.assertEqual(result.output_path, "")

    def test_namespace_injected_from_config(self):
        """It sets report.metadata.namespace from config.namespace before create."""
        from michelangelo.workflow.tasks.functions.eval_report_sinks.api import (
            GRPCEvalReportSink,
        )

        stub = self._make_stub("r1", "injected-ns")
        with patch(self._STUB_PATH, return_value=stub):
            cfg = GRPCEvalReportSinkConfig(
                endpoint="localhost:50051", namespace="injected-ns"
            )
            sink = GRPCEvalReportSink(cfg)
            report = _report(name="r1", namespace="")
            sink.write(report)

        self.assertEqual(report.metadata.namespace, "injected-ns")
