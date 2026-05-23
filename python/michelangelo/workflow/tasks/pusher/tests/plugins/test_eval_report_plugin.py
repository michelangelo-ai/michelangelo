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
        plugin = _plugin()
        self.assertIsNotNone(plugin)


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

    def test_output_file_exists_and_is_valid_json(self):
        """It writes a valid JSON file containing the report spec."""
        result = self._run()
        self.assertTrue(os.path.exists(result["output_path"]))
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertIn("spec", doc)

    def test_proto_title_preserved_in_output(self):
        """It serializes the EvaluationReportSpec title into the JSON document."""
        result = self._run(artifact=_report(title="Q1 Evaluation"))
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertEqual(doc["spec"]["title"], "Q1 Evaluation")

    def test_config_report_name_used_as_filename(self):
        """It uses report_name as the JSON filename stem."""
        result = self._run(report_name="q1-eval")
        self.assertTrue(result["output_path"].endswith("q1-eval.json"))
        self.assertEqual(result["report_name"], "q1-eval")

    def test_generated_report_name_starts_with_eval_report(self):
        """It generates a name starting with 'eval-report-' when none given."""
        result = self._run(report_name=None)
        self.assertTrue(result["report_name"].startswith("eval-report-"))

    def test_extra_fields_merged_into_document(self):
        """It merges extra_fields into the written document."""
        result = self._run(extra_fields={"team": "pricing"})
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertEqual(doc["team"], "pricing")

    def test_extra_fields_override_proto_keys_on_collision(self):
        """It gives extra_fields precedence over proto fields on collision."""
        result = self._run(extra_fields={"spec": "overridden"})
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertEqual(doc["spec"], "overridden")

    def test_returns_three_key_dict(self):
        """It returns a dict with report_name, output_path, num_keys."""
        result = self._run()
        self.assertIn("report_name", result)
        self.assertIn("output_path", result)
        self.assertIn("num_keys", result)

    def test_num_keys_counts_only_proto_fields(self):
        """It counts only serialized proto fields in num_keys, not extra or reserved."""
        result = self._run(extra_fields={"extra": "x"})
        with open(result["output_path"]) as f:
            doc = json.load(f)
        proto_keys = set(doc.keys()) - {"extra", "_report_name"}
        self.assertEqual(result["num_keys"], len(proto_keys))

    def test_report_name_injected_into_document(self):
        """It writes the assigned report name to the document's '_report_name' key."""
        result = self._run(report_name="my-report")
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertEqual(doc["_report_name"], "my-report")

    def test_output_written_inside_michelangelo_reports_tmpdir(self):
        """It writes output inside a michelangelo_reports_ temp directory."""
        result = self._run()
        parent = os.path.basename(os.path.dirname(result["output_path"]))
        self.assertTrue(parent.startswith("michelangelo_reports_"))

    def test_snake_case_field_names_in_json(self):
        """It uses snake_case field names from the proto (not camelCase)."""
        report = EvaluationReport(
            spec=EvaluationReportSpec(
                title="Test",
                sealed=True,
            )
        )
        result = EvalReportPusherPlugin(
            config=EvalReportPluginConfig(),
            artifact=report,
        ).execute()
        self._output_dirs.append(os.path.dirname(result["output_path"]))
        with open(result["output_path"]) as f:
            doc = json.load(f)
        # preserving_proto_field_name=True → snake_case keys
        self.assertIn("spec", doc)
        self.assertNotIn("typeMeta", doc)
