"""Round-trip test for the CanvasFlex TaskConfig envelope.

A distributed task's config envelope travels three legs:

1. Registration: the envelope is serialized with the registration
   ConfigEncoder (michelangelo.uniflow.registration.config_builder), which
   dumps JSONData models with the ``UniflowCodec`` context so every nested
   model carries ``__codec__``/``__class__`` markers.
2. Star: the Go worker decodes the JSON into plain Starlark dicts and the
   star re-dumps it into container CLI args. Structurally this is a plain
   ``json.loads`` -> ``json.dumps`` of the leg-1 output.
3. Container: run_task decodes the CLI args with the Uniflow codec Decoder
   (michelangelo.uniflow.core.run_task._decode_arg), which must yield a
   typed TaskConfig because pipeline_task unwraps ``envelope.config`` via
   attribute access.

These tests drive the envelope through all three legs and assert the result
is a fully typed TaskConfig identical to the original.
"""

import json
import unittest

from michelangelo.canvas.lib.shared.json_data.json_data import JSONData
from michelangelo.canvas.schema.v2alpha1.config import TaskConfig
from michelangelo.canvas.schema.v2alpha1.job_specs import (
    DriverSpec,
    ExecutorSpec,
    HeadSpec,
    JobSpecs,
    PodSpec,
    RayJobSpec,
    ResourceSpec,
    SparkJobSpec,
    WorkerSpec,
)
from michelangelo.uniflow.core.run_task import _decode_arg
from michelangelo.uniflow.registration.config_builder import ConfigEncoder


class TrainerConfig(JSONData):
    """A typed task-level config payload, as a user would define one."""

    learning_rate: float
    epochs: int
    optimizer: str


def _resource() -> ResourceSpec:
    return ResourceSpec(cpu=4, memory="32G", disk_size="100G", gpu=1, gpu_sku="a100")


def _envelope(**kwargs) -> TaskConfig:
    """Build a TaskConfig, filling any other required envelope fields with None.

    Keeps the test focused on config + job_specs while remaining stable as
    new optional envelope fields (e.g. retry_attempts) are added.
    """
    for name, field_info in TaskConfig.model_fields.items():
        if name not in kwargs and field_info.is_required():
            kwargs[name] = None
    return TaskConfig(**kwargs)


def _roundtrip(envelope: TaskConfig) -> TaskConfig:
    """Send the envelope through registration encode, star re-dump, and decode."""
    # Leg 1: registration serializes the envelope with the UniflowCodec context.
    leg1 = json.dumps(envelope, cls=ConfigEncoder)

    # Leg 2: the star leg is structure-preserving (plain dicts in, JSON out).
    leg2 = json.dumps(json.loads(leg1))

    # Leg 3: the container decodes CLI args via the run_task decoder path.
    return _decode_arg(leg2)


class EnvelopeRoundtripTest(unittest.TestCase):
    """Tests for the TaskConfig envelope serialization round trip."""

    def test_ray_envelope_roundtrip(self):
        """A ray envelope survives the full round trip fully typed."""
        envelope = _envelope(
            task_function="my_pipeline.train",
            config=TrainerConfig(learning_rate=0.01, epochs=5, optimizer="adam"),
            job_specs=JobSpecs(
                ray=RayJobSpec(
                    head=HeadSpec(pod=PodSpec(resource=_resource())),
                    worker=WorkerSpec(
                        pod=PodSpec(resource=_resource()),
                        min_instances=1,
                        max_instances=8,
                    ),
                ),
                spark=None,
            ),
        )

        decoded = _roundtrip(envelope)

        self.assertIsInstance(decoded, TaskConfig)
        self.assertEqual(decoded, envelope)

        # pipeline_task unwraps envelope.config by attribute access; the inner
        # payload must come back as the typed user config class.
        self.assertIsInstance(decoded.config, TrainerConfig)
        self.assertEqual(decoded.config.learning_rate, 0.01)
        self.assertEqual(decoded.config.epochs, 5)
        self.assertEqual(decoded.config.optimizer, "adam")

        self.assertIsInstance(decoded.job_specs, JobSpecs)
        self.assertIsNone(decoded.job_specs.spark)
        self.assertIsInstance(decoded.job_specs.ray, RayJobSpec)
        self.assertIsInstance(decoded.job_specs.ray.head.pod.resource, ResourceSpec)
        self.assertEqual(decoded.job_specs.ray.head.pod.resource.cpu, 4)
        self.assertEqual(decoded.job_specs.ray.head.pod.resource.memory, "32G")
        self.assertEqual(decoded.job_specs.ray.head.pod.resource.gpu_sku, "a100")
        self.assertEqual(decoded.job_specs.ray.worker.min_instances, 1)
        self.assertEqual(decoded.job_specs.ray.worker.max_instances, 8)

    def test_spark_envelope_roundtrip(self):
        """A spark envelope survives the full round trip fully typed."""
        envelope = _envelope(
            task_function="my_pipeline.etl",
            config=TrainerConfig(learning_rate=0.1, epochs=1, optimizer="sgd"),
            job_specs=JobSpecs(
                spark=SparkJobSpec(
                    driver=DriverSpec(pod=PodSpec(resource=_resource())),
                    executor=ExecutorSpec(
                        pod=PodSpec(resource=_resource()), instances=16
                    ),
                    spark_conf={"spark.executor.memoryOverhead": "4g"},
                    deps={"jars": ["a.jar", "b.jar"]},
                ),
                ray=None,
            ),
        )

        decoded = _roundtrip(envelope)

        self.assertIsInstance(decoded, TaskConfig)
        self.assertEqual(decoded, envelope)

        self.assertIsInstance(decoded.config, TrainerConfig)
        self.assertIsNone(decoded.job_specs.ray)
        self.assertIsInstance(decoded.job_specs.spark, SparkJobSpec)
        self.assertIsInstance(decoded.job_specs.spark.driver.pod.resource, ResourceSpec)
        self.assertEqual(decoded.job_specs.spark.driver.pod.resource.cpu, 4)
        self.assertEqual(decoded.job_specs.spark.executor.instances, 16)
        self.assertEqual(
            decoded.job_specs.spark.spark_conf,
            {"spark.executor.memoryOverhead": "4g"},
        )
        self.assertEqual(decoded.job_specs.spark.deps, {"jars": ["a.jar", "b.jar"]})

    def test_codec_markers_do_not_leak_into_decoded_models(self):
        """__codec__/__class__ markers are consumed by the decoder, not kept."""
        envelope = _envelope(
            task_function="my_pipeline.train",
            config=TrainerConfig(learning_rate=0.01, epochs=5, optimizer="adam"),
            job_specs=None,
        )

        leg1 = json.loads(json.dumps(envelope, cls=ConfigEncoder))
        # Sanity-check the wire format actually carries the markers.
        self.assertEqual(leg1["__codec__"], "dataclass")
        self.assertEqual(leg1["config"]["__codec__"], "dataclass")

        decoded = _roundtrip(envelope)
        self.assertNotIn("__codec__", decoded.model_dump())
        self.assertNotIn("__codec__", decoded.config.model_dump())


if __name__ == "__main__":
    unittest.main()
