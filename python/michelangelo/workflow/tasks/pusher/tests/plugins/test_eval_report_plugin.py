"""Tests for EvalReportPusherPlugin."""

from __future__ import annotations

import json
import os
import shutil
from unittest import TestCase

from michelangelo.gen.api.v2.evaluation_report_pb2 import (
    EvaluationReport,
    EvaluationReportSpec,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.pusher import EvalReportPluginConfig
from michelangelo.workflow.tasks.pusher.plugins.eval_report_plugin import (
    EvalReportPusherPlugin,
)


def _report(title: str = "Test Report") -> EvaluationReport:
    """Build a minimal EvaluationReport for test use."""
    return EvaluationReport(spec=EvaluationReportSpec(title=title))


def _plugin(
    artifact: EvaluationReport | None = None,
    report_name: str | None = None,
    extra_fields: dict | None = None,
) -> EvalReportPusherPlugin:
    """Build an EvalReportPusherPlugin with defaults for test convenience."""
    return EvalReportPusherPlugin(
        config=EvalReportPluginConfig(
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
        """Remove temp directories created by execute()."""
        for d in self._output_dirs:
            if os.path.exists(d):
                shutil.rmtree(d)

    def _run(self, **kwargs: object) -> dict:
        result = _plugin(**kwargs).execute()  # type: ignore[arg-type]
        self._output_dirs.append(os.path.dirname(result["output_path"]))
        return result

    def test_returns_name_namespace_output_path(self):
        """It returns a dict with name, namespace, and output_path keys."""
        result = self._run()
        self.assertIn("name", result)
        self.assertIn("namespace", result)
        self.assertIn("output_path", result)

    def test_config_report_name_takes_precedence(self):
        """It uses config.report_name over proto.metadata.name."""
        report = EvaluationReport(
            spec=EvaluationReportSpec(title="T"),
        )
        report.metadata.name = "from-proto"
        plugin = EvalReportPusherPlugin(
            config=EvalReportPluginConfig(report_name="from-config"),
            artifact=report,
        )
        result = plugin.execute()
        self._output_dirs.append(os.path.dirname(result["output_path"]))
        self.assertEqual(result["name"], "from-config")

    def test_proto_metadata_name_used_when_config_name_absent(self):
        """It uses proto.metadata.name when config.report_name is not set."""
        report = EvaluationReport(spec=EvaluationReportSpec(title="T"))
        report.metadata.name = "proto-name"
        plugin = EvalReportPusherPlugin(
            config=EvalReportPluginConfig(),
            artifact=report,
        )
        result = plugin.execute()
        self._output_dirs.append(os.path.dirname(result["output_path"]))
        self.assertEqual(result["name"], "proto-name")

    def test_auto_generates_name_when_none_set(self):
        """It generates a name starting with 'eval-report-' when none given."""
        result = self._run(report_name=None)
        self.assertTrue(result["name"].startswith("eval-report-"))

    def test_name_set_on_proto_after_execute(self):
        """It sets metadata.name on the artifact so the proto carries the name."""
        report = _report()
        plugin = EvalReportPusherPlugin(
            config=EvalReportPluginConfig(report_name="my-report"),
            artifact=report,
        )
        result = plugin.execute()
        self._output_dirs.append(os.path.dirname(result["output_path"]))
        self.assertEqual(report.metadata.name, "my-report")

    def test_namespace_from_proto_in_result(self):
        """It includes metadata.namespace from the proto in the result dict."""
        report = EvaluationReport(spec=EvaluationReportSpec(title="T"))
        report.metadata.namespace = "ml-project"
        plugin = EvalReportPusherPlugin(
            config=EvalReportPluginConfig(),
            artifact=report,
        )
        result = plugin.execute()
        self._output_dirs.append(os.path.dirname(result["output_path"]))
        self.assertEqual(result["namespace"], "ml-project")

    def test_output_file_is_valid_json_with_spec(self):
        """It writes a valid JSON file containing the serialized proto spec."""
        result = self._run(artifact=_report(title="Q1 Eval"))
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertEqual(doc["spec"]["title"], "Q1 Eval")

    def test_name_present_in_serialized_document(self):
        """It serializes the resolved name into metadata.name in the JSON."""
        result = self._run(report_name="named-report")
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertEqual(doc["metadata"]["name"], "named-report")

    def test_extra_fields_merged_into_document(self):
        """It merges extra_fields into the written document."""
        result = self._run(extra_fields={"ci_run_id": "build-42"})
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertEqual(doc["ci_run_id"], "build-42")

    def test_extra_fields_take_precedence_over_proto_keys(self):
        """It gives extra_fields precedence over proto fields on collision."""
        result = self._run(extra_fields={"spec": "overridden"})
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertEqual(doc["spec"], "overridden")

    def test_output_written_inside_michelangelo_reports_tmpdir(self):
        """It writes output inside a michelangelo_reports_ temp directory."""
        result = self._run()
        parent = os.path.basename(os.path.dirname(result["output_path"]))
        self.assertTrue(parent.startswith("michelangelo_reports_"))

    def test_snake_case_field_names_in_json(self):
        """It uses snake_case field names in the output JSON, not camelCase."""
        result = self._run()
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertNotIn("typeMeta", doc)
        self.assertNotIn("objectMeta", doc)
