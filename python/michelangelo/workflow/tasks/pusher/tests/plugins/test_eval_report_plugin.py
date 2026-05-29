"""Tests for EvalReportPusherPlugin with sink dispatch."""

from __future__ import annotations

import json
import os
import shutil
from typing import Any
from unittest import TestCase
from unittest.mock import MagicMock

from michelangelo.gen.api.v2.evaluation_report_pb2 import (
    EvaluationReport,
    EvaluationReportSpec,
)
from michelangelo.workflow.schema.eval_report_sinks.result import EvalReportSinkResult
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.pusher import EvalReportPluginConfig
from michelangelo.workflow.tasks.functions.eval_report_sinks import (
    LocalFileEvalReportSink,
)
from michelangelo.workflow.tasks.pusher.plugins.eval_report_plugin import (
    EvalReportPusherPlugin,
)


def _report(title: str = "Test Report") -> EvaluationReport:
    """Build a minimal EvaluationReport for test use."""
    return EvaluationReport(spec=EvaluationReportSpec(title=title))


def _mock_sink(
    name: str = "mock-report",
    namespace: str = "",
    output_path: str = "",
) -> MagicMock:
    """Build a mock EvalReportSink that returns a fixed result."""
    sink = MagicMock()
    sink.write.return_value = EvalReportSinkResult(
        name=name, namespace=namespace, output_path=output_path
    )
    return sink


def _plugin(
    artifact: EvaluationReport | None = None,
    report_name: str | None = None,
    sinks: list | None = None,
    extra_fields: dict | None = None,
) -> EvalReportPusherPlugin:
    """Build an EvalReportPusherPlugin with defaults for test convenience."""
    return EvalReportPusherPlugin(
        config=EvalReportPluginConfig(
            sinks=sinks,
            report_name=report_name,
            extra_fields=extra_fields or {},
        ),
        artifact=artifact if artifact is not None else _report(),
    )


class TestEvalReportPusherPluginInit(TestCase):
    """Tests for EvalReportPusherPlugin.__init__() validation."""

    def test_raises_when_artifact_is_none(self):
        """It raises ConfigurationError when artifact=None is passed."""
        with self.assertRaises(ConfigurationError):
            EvalReportPusherPlugin(
                config=EvalReportPluginConfig(),
                artifact=None,
            )

    def test_raises_when_artifact_is_not_evaluation_report(self):
        """It raises ConfigurationError when artifact is not an EvaluationReport."""
        with self.assertRaises(ConfigurationError):
            EvalReportPusherPlugin(
                config=EvalReportPluginConfig(),
                artifact={"accuracy": 0.9},  # type: ignore[arg-type]
            )

    def test_accepts_evaluation_report_proto(self):
        """It accepts an EvaluationReport without raising."""
        self.assertIsNotNone(_plugin())


class TestEvalReportPusherPluginExecute(TestCase):
    """Tests for EvalReportPusherPlugin.execute()."""

    def setUp(self) -> None:
        """Collect output dirs for cleanup."""
        self._output_dirs: list[str] = []

    def tearDown(self) -> None:
        """Remove temp directories created during tests."""
        for d in self._output_dirs:
            if os.path.exists(d):
                shutil.rmtree(d)

    def _run_with_mock_sink(self, **kwargs: Any) -> tuple[dict, MagicMock]:
        sink = _mock_sink()
        result = _plugin(sinks=[sink], **kwargs).execute()
        return result, sink

    # ── Name resolution ────────────────────────────────────────────────────

    def test_config_report_name_takes_precedence(self):
        """It uses config.report_name over proto.metadata.name."""
        report = _report()
        report.metadata.name = "from-proto"
        result, _ = self._run_with_mock_sink(artifact=report, report_name="from-config")
        self.assertEqual(result["name"], "from-config")

    def test_proto_metadata_name_used_when_config_name_absent(self):
        """It falls back to proto.metadata.name when config.report_name is not set."""
        report = _report()
        report.metadata.name = "proto-name"
        result, _ = self._run_with_mock_sink(artifact=report, report_name=None)
        self.assertEqual(result["name"], "proto-name")

    def test_auto_generates_name_when_neither_set(self):
        """It generates an eval-report-<uuid> name when no name is configured."""
        result, _ = self._run_with_mock_sink(report_name=None)
        self.assertTrue(result["name"].startswith("eval-report-"))

    def test_name_set_on_proto_before_sink_called(self):
        """It enriches report.metadata.name before passing the proto to the sink."""
        report = _report()
        _result, sink = self._run_with_mock_sink(
            artifact=report, report_name="enriched"
        )
        called_report = sink.write.call_args[0][0]
        self.assertEqual(called_report.metadata.name, "enriched")

    # ── Sink dispatch ──────────────────────────────────────────────────────

    def test_calls_each_sink_once(self):
        """It calls write() on each configured sink exactly once."""
        sink_a, sink_b = _mock_sink(), _mock_sink()
        _plugin(sinks=[sink_a, sink_b]).execute()
        sink_a.write.assert_called_once()
        sink_b.write.assert_called_once()

    def test_passes_extra_fields_to_each_sink(self):
        """It passes extra_fields to each sink's write() call."""
        sink = _mock_sink()
        _plugin(sinks=[sink], extra_fields={"ci_run": "42"}).execute()
        passed = sink.write.call_args[0][1]
        self.assertEqual(passed.get("ci_run"), "42")

    def test_sinks_list_in_result_contains_per_sink_dicts(self):
        """It returns a sinks list with one entry per sink."""
        sink_a = _mock_sink(name="r", namespace="ns", output_path="/tmp/r.json")
        result = _plugin(sinks=[sink_a]).execute()
        self.assertEqual(len(result["sinks"]), 1)
        self.assertEqual(result["sinks"][0]["output_path"], "/tmp/r.json")

    def test_output_path_from_first_sink_result(self):
        """It exposes the first sink's output_path as the top-level output_path."""
        sink = _mock_sink(output_path="/tmp/report.json")
        result = _plugin(sinks=[sink]).execute()
        self.assertEqual(result["output_path"], "/tmp/report.json")

    def test_namespace_from_first_sink_result(self):
        """It exposes the first sink's namespace as the top-level namespace."""
        sink = _mock_sink(namespace="ml-prod")
        result = _plugin(sinks=[sink]).execute()
        self.assertEqual(result["namespace"], "ml-prod")

    def test_empty_sinks_list_returns_proto_namespace(self):
        """It returns proto.metadata.namespace when sinks=[] (dry-run)."""
        report = _report()
        report.metadata.namespace = "dry-ns"
        result = _plugin(artifact=report, sinks=[]).execute()
        self.assertEqual(result["namespace"], "dry-ns")
        self.assertEqual(result["sinks"], [])

    # ── Default sink (LocalFileEvalReportSink) ─────────────────────────────

    def test_default_config_writes_local_file(self):
        """It writes a JSON file when no sinks are configured (default LocalFile)."""
        plugin = EvalReportPusherPlugin(
            config=EvalReportPluginConfig(report_name="default-sink-test"),
            artifact=_report(),
        )
        result = plugin.execute()
        self._output_dirs.append(os.path.dirname(result["output_path"]))
        self.assertTrue(os.path.exists(result["output_path"]))
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertIn("spec", doc)

    # ── Multi-sink ─────────────────────────────────────────────────────────

    def test_multi_sink_both_called(self):
        """It dispatches to all sinks; result has one entry per sink."""
        local_sink = LocalFileEvalReportSink()
        mock_api_sink = _mock_sink(name="r", namespace="ns")
        result = EvalReportPusherPlugin(
            config=EvalReportPluginConfig(
                sinks=[local_sink, mock_api_sink],
                report_name="multi-sink",
            ),
            artifact=_report(),
        ).execute()
        if result["sinks"][0]["output_path"]:
            self._output_dirs.append(os.path.dirname(result["sinks"][0]["output_path"]))
        self.assertEqual(len(result["sinks"]), 2)
        mock_api_sink.write.assert_called_once()
