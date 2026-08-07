"""Tests for the pipeline_conf.yaml branch of the registration subprocess."""

import argparse
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from michelangelo.uniflow.registration.subprocess import (
    find_pipeline_conf,
    register_yaml_pipeline,
)

_DEMO_MODULE = "michelangelo.canvas.pipeline.tests.demo_workflow"

_PIPELINE_CONF_NO_WORKFLOW_CONFIG = f"""\
workflow_function: {_DEMO_MODULE}.demo_workflow_no_config
task_configs:
  train:
    config:
      learning_rate: 0.05
"""

_PIPELINE_CONF_WITH_WORKFLOW_CONFIG = f"""\
workflow_function: {_DEMO_MODULE}.demo_workflow
workflow_config:
  dataset: cats-vs-dogs
task_configs:
  train:
    config:
      learning_rate: 0.05
  evaluate:
    config:
      threshold: 0.8
"""


def _write(path: Path, content: str) -> Path:
    path.write_text(content)
    return path


def _registration_args(output_dir: str) -> argparse.Namespace:
    return argparse.Namespace(
        project="demo-project",
        pipeline="demo-pipeline",
        output_dir=output_dir,
        storage_url=None,
        output_filename=None,
        environ=None,
    )


class FindPipelineConfTest(unittest.TestCase):
    """Tests for find_pipeline_conf."""

    def setUp(self):
        """Create a scratch directory for YAML fixtures."""
        self.tmp_dir = Path(tempfile.mkdtemp())

    def test_direct_pipeline_conf_returns_itself(self):
        """A pipeline_conf.yaml passed directly is detected."""
        conf = _write(
            self.tmp_dir / "pipeline_conf.yaml", _PIPELINE_CONF_NO_WORKFLOW_CONFIG
        )
        self.assertEqual(find_pipeline_conf(str(conf)), conf)

    def test_crd_manifest_resolves_relative_to_crd_file(self):
        """A Pipeline CRD's manifest filePath resolves against the CRD's dir."""
        _write(self.tmp_dir / "pipeline_conf.yaml", _PIPELINE_CONF_NO_WORKFLOW_CONFIG)
        crd = _write(
            self.tmp_dir / "pipeline.yaml",
            "apiVersion: michelangelo.api/v2\n"
            "kind: Pipeline\n"
            "metadata: {namespace: p, name: n}\n"
            "spec:\n"
            "  manifest:\n"
            "    filePath: pipeline_conf.yaml\n",
        )
        self.assertEqual(
            find_pipeline_conf(str(crd)), self.tmp_dir / "pipeline_conf.yaml"
        )

    def test_crd_manifest_path_key_is_accepted(self):
        """The docs' manifest 'path' spelling works like 'filePath'."""
        _write(self.tmp_dir / "pipeline_conf.yaml", _PIPELINE_CONF_NO_WORKFLOW_CONFIG)
        crd = _write(
            self.tmp_dir / "pipeline.yaml",
            "spec:\n  manifest:\n    path: pipeline_conf.yaml\n",
        )
        self.assertEqual(
            find_pipeline_conf(str(crd)), self.tmp_dir / "pipeline_conf.yaml"
        )

    def test_module_path_manifest_is_not_detected(self):
        """Custom workflows (module-path manifest) keep the legacy flow."""
        crd = _write(
            self.tmp_dir / "pipeline.yaml",
            "spec:\n  manifest:\n    filePath: examples.bert_cola.bert_cola\n",
        )
        self.assertIsNone(find_pipeline_conf(str(crd)))

    def test_yaml_manifest_without_workflow_function_is_not_detected(self):
        """A .yaml manifest that isn't a pipeline_conf.yaml is left alone."""
        _write(self.tmp_dir / "other.yaml", "some_key: some_value\n")
        crd = _write(
            self.tmp_dir / "pipeline.yaml",
            "spec:\n  manifest:\n    filePath: other.yaml\n",
        )
        self.assertIsNone(find_pipeline_conf(str(crd)))

    def test_missing_manifest_file_is_not_detected(self):
        """A dangling .yaml manifest path falls through to the legacy flow."""
        crd = _write(
            self.tmp_dir / "pipeline.yaml",
            "spec:\n  manifest:\n    filePath: does_not_exist.yaml\n",
        )
        self.assertIsNone(find_pipeline_conf(str(crd)))

    def test_non_mapping_yaml_is_not_detected(self):
        """Non-mapping YAML content is left to the legacy flow."""
        crd = _write(self.tmp_dir / "pipeline.yaml", "- just\n- a\n- list\n")
        self.assertIsNone(find_pipeline_conf(str(crd)))


class RegisterYamlPipelineTest(unittest.TestCase):
    """Tests for register_yaml_pipeline's artifacts."""

    def setUp(self):
        """Create scratch directories for the conf file and outputs."""
        self.tmp_dir = Path(tempfile.mkdtemp())
        self.output_dir = self.tmp_dir / "out"
        self.output_dir.mkdir()

    def _register(self, conf_content: str) -> dict:
        conf = _write(self.tmp_dir / "pipeline_conf.yaml", conf_content)
        with patch(
            "michelangelo.uniflow.registration.uniflow_tar.prepare_uniflow_tar",
            return_value="s3://fake/uniflow/demo.tar.gz",
        ) as mock_tar:
            remote_path, fn_name = register_yaml_pipeline(
                conf, _registration_args(str(self.output_dir))
            )
        self.mock_tar = mock_tar
        self.remote_path = remote_path
        self.fn_name = fn_name
        return json.loads((self.output_dir / "uniflow_input.txt").read_text())

    def test_input_file_uses_yaml_pipeline_shape(self):
        """uniflow_input.txt carries top-level task_configs, not args/kwargs."""
        payload = self._register(_PIPELINE_CONF_NO_WORKFLOW_CONFIG)

        self.assertEqual(self.remote_path, "s3://fake/uniflow/demo.tar.gz")
        self.assertEqual(self.fn_name, "demo_workflow_no_config")
        self.assertIn("task_configs", payload)
        self.assertNotIn("args", payload)
        self.assertNotIn("kwargs", payload)
        # 1-parameter workflow: no workflow_config entry (the pipeline-run
        # controller would pass it as an extra positional arg).
        self.assertNotIn("workflow_config", payload)
        self.assertEqual(payload["environ"], {})

        train = payload["task_configs"]["train"]
        self.assertEqual(train["config"]["learning_rate"], 0.05)
        # The envelope must keep its codec markers for Starlark/run_task.
        self.assertIn("__class__", train)

    def test_workflow_config_included_for_two_param_workflow(self):
        """A workflow-level config lands as a top-level workflow_config."""
        payload = self._register(_PIPELINE_CONF_WITH_WORKFLOW_CONFIG)

        self.assertEqual(self.fn_name, "demo_workflow")
        self.assertEqual(payload["workflow_config"]["dataset"], "cats-vs-dogs")
        self.assertEqual(sorted(payload["task_configs"]), ["evaluate", "train"])

    def test_tarball_built_from_registration_identity(self):
        """The tar builder gets the project/pipeline names and workflow fqn."""
        self._register(_PIPELINE_CONF_NO_WORKFLOW_CONFIG)

        kwargs = self.mock_tar.call_args.kwargs
        self.assertEqual(kwargs["project_name"], "demo-project")
        self.assertEqual(kwargs["pipeline_name"], "demo-pipeline")
        self.assertEqual(
            kwargs["workflow_function"],
            f"{_DEMO_MODULE}.demo_workflow_no_config",
        )


if __name__ == "__main__":
    unittest.main()
