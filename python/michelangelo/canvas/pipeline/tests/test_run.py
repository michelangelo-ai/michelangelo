"""Tests for run_pipeline (michelangelo.canvas.pipeline.run)."""

import tempfile
import unittest
from pathlib import Path

from michelangelo.canvas.pipeline.run import run_pipeline


def _write_yaml(content: str) -> Path:
    """Write ``content`` to a temp file and return its path."""
    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
        f.write(content)
    return Path(f.name)


class RunPipelineTest(unittest.TestCase):
    """Tests for run_pipeline."""

    def test_runs_workflow_end_to_end(self):
        """A pipeline with a workflow-level config runs to completion."""
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

        result = run_pipeline(path)

        self.assertEqual(result["dataset"], "cats-vs-dogs")
        self.assertEqual(result["train"], {"trained_lr": 0.05})
        self.assertEqual(result["eval"], {"passed": True})

    def test_runs_workflow_without_workflow_level_config(self):
        """A pipeline with no workflow-level config runs to completion."""
        path = _write_yaml(
            "workflow_function: "
            "michelangelo.canvas.pipeline.tests.demo_workflow.demo_workflow_no_config\n"
            "task_configs:\n"
            "  train:\n"
            "    config:\n"
            "      learning_rate: 0.1\n"
        )

        result = run_pipeline(path)

        self.assertEqual(result, {"trained_lr": 0.1})


if __name__ == "__main__":
    unittest.main()
