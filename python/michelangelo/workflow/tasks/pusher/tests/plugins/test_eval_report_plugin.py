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

_ARTIFACT = {"accuracy": 0.93, "f1": 0.91, "loss": 0.12}


def _plugin(
    artifact: dict | EvaluationReport | None = None,
    report_name: str | None = None,
    extra_fields: dict | None = None,
) -> EvalReportPusherPlugin:
    """Build an EvalReportPusherPlugin with defaults for test convenience."""
    return EvalReportPusherPlugin(
        config=EvalReportPluginConfig(
            report_name=report_name,
            extra_fields=extra_fields or {},
        ),
        artifact=artifact if artifact is not None else dict(_ARTIFACT),
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

    def test_raises_when_artifact_contains_reserved_key(self):
        """It raises ConfigurationError when artifact contains '_report_name'."""
        with self.assertRaises(ConfigurationError):
            _plugin(artifact={"_report_name": "clash", "accuracy": 0.9})

    def test_raises_when_artifact_type_invalid(self):
        """It raises ConfigurationError when artifact is not a dict or proto."""
        with self.assertRaises(ConfigurationError):
            EvalReportPusherPlugin(
                config=EvalReportPluginConfig(),
                artifact="not-a-dict",  # type: ignore[arg-type]
            )

    def test_accepts_evaluation_report_proto(self):
        """It accepts an EvaluationReport protobuf message without raising."""
        spec = EvaluationReportSpec(title="Q1 Evaluation")
        report = EvaluationReport(spec=spec)
        plugin = EvalReportPusherPlugin(
            config=EvalReportPluginConfig(report_name="q1-proto"),
            artifact=report,
        )
        self.assertIsNotNone(plugin)


class TestEvalReportPusherPluginExecute(TestCase):
    """Tests for EvalReportPusherPlugin.execute()."""

    def setUp(self) -> None:
        """Collect output paths for cleanup."""
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
        """It writes a valid JSON file containing artifact keys."""
        result = self._run()
        self.assertTrue(os.path.exists(result["output_path"]))
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertIn("accuracy", doc)
        self.assertIn("f1", doc)

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

    def test_extra_fields_override_artifact_on_collision(self):
        """It gives extra_fields precedence over artifact keys on collision."""
        result = self._run(
            artifact={"accuracy": 0.80},
            extra_fields={"accuracy": 0.99},
        )
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertEqual(doc["accuracy"], 0.99)

    def test_returns_three_key_dict(self):
        """It returns a dict with report_name, output_path, num_keys."""
        result = self._run()
        self.assertIn("report_name", result)
        self.assertIn("output_path", result)
        self.assertIn("num_keys", result)

    def test_num_keys_counts_only_artifact_keys(self):
        """It counts only artifact keys in num_keys, not extra or reserved."""
        result = self._run(
            artifact={"a": 1, "b": 2},
            extra_fields={"extra": "x"},
        )
        self.assertEqual(result["num_keys"], 2)

    def test_report_name_present_in_document(self):
        """It writes the assigned report name to the document's '_report_name' key."""
        result = self._run(report_name="my-report")
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertEqual(doc["_report_name"], "my-report")

    def test_output_written_inside_michelangelo_reports_tmpdir(self):
        """It writes the output file inside a michelangelo_reports_ temp directory."""
        result = self._run()
        parent = os.path.basename(os.path.dirname(result["output_path"]))
        self.assertTrue(parent.startswith("michelangelo_reports_"))

    def test_proto_artifact_written_as_json(self):
        """It serializes an EvaluationReport proto to JSON with snake_case keys."""
        spec = EvaluationReportSpec(title="Q1 Eval")
        report = EvaluationReport(spec=spec)
        plugin = EvalReportPusherPlugin(
            config=EvalReportPluginConfig(report_name="proto-eval"),
            artifact=report,
        )
        result = plugin.execute()
        self._output_dirs.append(os.path.dirname(result["output_path"]))
        self.assertTrue(os.path.exists(result["output_path"]))
        with open(result["output_path"]) as f:
            doc = json.load(f)
        self.assertIn("spec", doc)
        self.assertEqual(doc["spec"]["title"], "Q1 Eval")

    def test_proto_num_keys_reflects_proto_fields(self):
        """It counts top-level fields in the serialized proto for num_keys."""
        spec = EvaluationReportSpec(title="Eval")
        report = EvaluationReport(spec=spec)
        plugin = EvalReportPusherPlugin(
            config=EvalReportPluginConfig(),
            artifact=report,
        )
        result = plugin.execute()
        self._output_dirs.append(os.path.dirname(result["output_path"]))
        self.assertGreaterEqual(result["num_keys"], 1)
