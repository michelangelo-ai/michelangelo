"""Tests for load_pipeline_config (michelangelo.canvas.pipeline.config_loader)."""

import tempfile
import unittest
from pathlib import Path

from michelangelo.canvas.pipeline.config_loader import load_pipeline_config
from michelangelo.canvas.pipeline.tests import demo_workflow, demo_workflow_override


def _write_yaml(content: str) -> Path:
    """Write ``content`` to a temp file and return its path."""
    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
        f.write(content)
    return Path(f.name)


class LoadPipelineConfigTest(unittest.TestCase):
    """Tests for load_pipeline_config."""

    def test_parses_workflow_with_config_and_task_configs(self):
        """Workflow-level config and per-task configs parse into typed objects."""
        path = _write_yaml(
            "workflow_function: "
            "michelangelo.canvas.pipeline.tests.demo_workflow.demo_workflow\n"
            "workflow_config:\n"
            "  dataset: cats-vs-dogs\n"
            "task_configs:\n"
            "  train:\n"
            "    config:\n"
            "      learning_rate: 0.05\n"
            "  evaluate:\n"
            "    config:\n"
            "      threshold: 0.8\n"
        )

        pipeline_config = load_pipeline_config(path)

        self.assertEqual(
            pipeline_config.workflow_config.workflow_config.dataset, "cats-vs-dogs"
        )
        self.assertEqual(
            pipeline_config.workflow_config.task_configs["train"].config.learning_rate,
            0.05,
        )
        self.assertEqual(
            pipeline_config.workflow_config.task_configs["evaluate"].config.threshold,
            0.8,
        )
        self.assertIs(pipeline_config.task_functions["train"], demo_workflow.train)
        self.assertIs(
            pipeline_config.task_functions["evaluate"], demo_workflow.evaluate
        )

    def test_parses_workflow_without_workflow_level_config(self):
        """A workflow taking only task_configs has no workflow_config object."""
        path = _write_yaml(
            "workflow_function: "
            "michelangelo.canvas.pipeline.tests.demo_workflow.demo_workflow_no_config\n"
            "task_configs:\n"
            "  train:\n"
            "    config:\n"
            "      learning_rate: 0.1\n"
        )

        pipeline_config = load_pipeline_config(path)

        self.assertIsNone(pipeline_config.workflow_config.workflow_config)
        self.assertEqual(
            pipeline_config.workflow_config.task_configs["train"].config.learning_rate,
            0.1,
        )

    def test_missing_workflow_function_raises(self):
        """A pipeline_conf.yaml with no workflow_function key is rejected."""
        path = _write_yaml("task_configs: {}\n")
        with self.assertRaises(ValueError):
            load_pipeline_config(path)

    def test_explicit_task_function_override(self):
        """An explicit task_function in YAML overrides the workflow's own global."""
        path = _write_yaml(
            "workflow_function: "
            "michelangelo.canvas.pipeline.tests.demo_workflow_override.demo_workflow\n"
            "task_configs:\n"
            "  train:\n"
            "    task_function: "
            "michelangelo.canvas.pipeline.tests.demo_workflow_override.custom_train\n"
            "    config:\n"
            "      learning_rate: 0.2\n"
        )

        pipeline_config = load_pipeline_config(path)

        self.assertIs(
            pipeline_config.task_functions["train"], demo_workflow_override.custom_train
        )

    def test_unknown_task_without_task_function_raises(self):
        """A task with no task_function and no matching workflow-module global fails."""
        path = _write_yaml(
            "workflow_function: "
            "michelangelo.canvas.pipeline.tests.demo_workflow.demo_workflow_no_config\n"
            "task_configs:\n"
            "  does_not_exist:\n"
            "    config: {}\n"
        )
        with self.assertRaises(ValueError):
            load_pipeline_config(path)


if __name__ == "__main__":
    unittest.main()
