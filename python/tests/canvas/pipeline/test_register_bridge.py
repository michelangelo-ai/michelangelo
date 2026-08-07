"""Tests for the registration bridge (michelangelo.canvas.pipeline.register).

Covers the YAML -> resolved workflow call -> uniflow registration input path
without needing a cluster: the bridge must hand ConfigBuilder/prepare_uniflow_input
typed TaskConfig envelopes so the encoders keep the __class__/__codec__ markers
(and the YAML job_specs values) in the uniflow_input-style JSON.
"""

import json
import tempfile
import unittest
from pathlib import Path

from michelangelo.canvas.pipeline.config_loader import load_pipeline_config
from michelangelo.canvas.pipeline.register import resolve_workflow_call
from michelangelo.canvas.pipeline.tests.demo_workflow import DemoWorkflowConfig
from michelangelo.canvas.schema.v2alpha1.config import TaskConfig as EnvelopeTaskConfig
from michelangelo.uniflow.registration.config_builder import ConfigEncoder
from michelangelo.uniflow.registration.register import prepare_uniflow_input

_DEMO_MODULE = "michelangelo.canvas.pipeline.tests.demo_workflow"

_PIPELINE_YAML = f"""\
workflow_function: {_DEMO_MODULE}.demo_workflow
workflow_config:
  dataset: cats-vs-dogs
task_configs:
  train:
    config:
      learning_rate: 0.05
    job_specs:
      ray:
        head:
          pod:
            resource:
              cpu: 3
              memory: 6Gi
              disk_size: 30Gi
              gpu: 0
              gpu_sku: ""
        worker:
          pod:
            resource:
              cpu: 2
              memory: 4Gi
              disk_size: 20Gi
              gpu: 0
              gpu_sku: ""
          min_instances: 1
          max_instances: 5
  evaluate:
    config:
      threshold: 0.8
    job_specs:
      spark:
        driver:
          pod:
            resource:
              cpu: 4
              memory: 8G
              disk_size: 40G
              gpu: 0
              gpu_sku: ""
        executor:
          pod:
            resource:
              cpu: 2
              memory: 4G
              disk_size: 20G
              gpu: 0
              gpu_sku: ""
          instances: 2
"""


def _write_yaml(content: str) -> Path:
    """Write ``content`` to a temp file and return its path."""
    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
        f.write(content)
    return Path(f.name)


class ResolveWorkflowCallTest(unittest.TestCase):
    """Tests for resolve_workflow_call."""

    def test_two_param_workflow_kwargs(self):
        """Kwargs are keyed by the workflow's own parameter names."""
        fn, kwargs = resolve_workflow_call(
            load_pipeline_config(_write_yaml(_PIPELINE_YAML))
        )

        self.assertEqual(fn.__name__, "demo_workflow")
        self.assertEqual(set(kwargs), {"config", "task_configs"})
        self.assertIsInstance(kwargs["config"], DemoWorkflowConfig)
        self.assertEqual(set(kwargs["task_configs"]), {"train", "evaluate"})

    def test_task_configs_are_typed_envelopes(self):
        """task_configs values must be typed TaskConfig objects, not dicts."""
        _, kwargs = resolve_workflow_call(
            load_pipeline_config(_write_yaml(_PIPELINE_YAML))
        )

        for envelope in kwargs["task_configs"].values():
            self.assertIsInstance(envelope, EnvelopeTaskConfig)
        self.assertEqual(
            kwargs["task_configs"]["train"].job_specs.ray.worker.max_instances, 5
        )

    def test_single_param_workflow_kwargs(self):
        """A task_configs-only workflow gets a single kwarg."""
        yaml_text = (
            f"workflow_function: {_DEMO_MODULE}.demo_workflow_no_config\n"
            "task_configs:\n"
            "  train:\n"
            "    config:\n"
            "      learning_rate: 0.1\n"
        )
        fn, kwargs = resolve_workflow_call(load_pipeline_config(_write_yaml(yaml_text)))

        self.assertEqual(fn.__name__, "demo_workflow_no_config")
        self.assertEqual(set(kwargs), {"task_configs"})


class UniflowInputTest(unittest.TestCase):
    """The bridge's kwargs survive serialization into uniflow_input.txt."""

    def _task_configs_json(self) -> dict:
        """Run prepare_uniflow_input on the bridge kwargs, return task_configs JSON."""
        _, kwargs = resolve_workflow_call(
            load_pipeline_config(_write_yaml(_PIPELINE_YAML))
        )

        with tempfile.TemporaryDirectory() as output_dir:
            inputs_path = prepare_uniflow_input((), kwargs, {}, output_dir)
            with open(inputs_path) as f:
                inputs = json.load(f)

        self.assertEqual(inputs["args"], [])
        kwargs_json = dict(inputs["kwargs"])
        self.assertIn("task_configs", kwargs_json)
        return kwargs_json["task_configs"]

    def test_envelopes_keep_class_markers(self):
        """Each task_configs entry carries the TaskConfig __class__ marker."""
        task_configs = self._task_configs_json()

        self.assertEqual(set(task_configs), {"train", "evaluate"})
        for entry in task_configs.values():
            self.assertTrue(entry["__class__"].endswith(".TaskConfig"), entry)
            self.assertTrue(entry["__class__"].startswith("michelangelo."), entry)
            self.assertIn("__codec__", entry)

    def test_job_specs_values_survive_from_yaml(self):
        """The YAML job_specs resource values reach the serialized input."""
        task_configs = self._task_configs_json()

        ray_specs = task_configs["train"]["job_specs"]["ray"]
        self.assertEqual(ray_specs["head"]["pod"]["resource"]["cpu"], 3)
        self.assertEqual(ray_specs["head"]["pod"]["resource"]["memory"], "6Gi")
        self.assertEqual(ray_specs["worker"]["max_instances"], 5)

        spark_specs = task_configs["evaluate"]["job_specs"]["spark"]
        self.assertEqual(spark_specs["driver"]["pod"]["resource"]["cpu"], 4)
        self.assertEqual(spark_specs["executor"]["instances"], 2)

    def test_config_encoder_adds_uniflow_codec_markers(self):
        """ConfigBuilder's ConfigEncoder serializes envelopes with markers."""
        _, kwargs = resolve_workflow_call(
            load_pipeline_config(_write_yaml(_PIPELINE_YAML))
        )

        dumped = json.loads(json.dumps(kwargs["task_configs"], cls=ConfigEncoder))

        train = dumped["train"]
        self.assertTrue(train["__class__"].endswith(".TaskConfig"))
        self.assertEqual(train["__codec__"], "dataclass")
        # Nested JSONData payloads keep their own markers too.
        self.assertTrue(train["config"]["__class__"].endswith(".TrainConfig"))
        self.assertEqual(train["job_specs"]["ray"]["head"]["pod"]["resource"]["cpu"], 3)


if __name__ == "__main__":
    unittest.main()
