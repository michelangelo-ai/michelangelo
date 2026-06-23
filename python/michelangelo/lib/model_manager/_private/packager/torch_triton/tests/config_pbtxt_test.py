"""Tests for torch_triton config.pbtxt template rendering."""

from unittest import TestCase

from michelangelo.lib.model_manager._private.constants.triton_backend_type import (
    TritonBackendType,
)
from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TritonTemplateRenderer,
)
from michelangelo.lib.model_manager._private.packager.torch_triton import (
    generate_config_pbtxt_content,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem


class ConfigPbtxtTest(TestCase):
    """Tests config.pbtxt rendering for torch-based Triton backends."""

    def setUp(self):
        """Set up a renderer and a minimal float input/output schema."""
        self.gen = TritonTemplateRenderer()
        self.schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="input", data_type=DataType.FLOAT, shape=[4]),
            ],
            output_schema=[
                ModelSchemaItem(name="output", data_type=DataType.FLOAT, shape=[2]),
            ],
        )

    def test_generate_config_pbtxt_content_pytorch_backend(self):
        """It renders a pytorch backend config with dynamic batching enabled."""
        config = generate_config_pbtxt_content(
            self.gen,
            model_name="test_model",
            model_revision=None,
            model_schema=self.schema,
            backend=TritonBackendType.TORCH,
        )

        self.assertIn('name: "test_model"', config)
        self.assertIn('backend: "pytorch"', config)
        self.assertIn("max_batch_size: 256", config)
        self.assertIn("dynamic_batching: {", config)
        self.assertIn("max_queue_delay_microseconds: 300", config)
        self.assertIn('name: "input"', config)
        self.assertIn('name: "output"', config)
        self.assertIn("data_type: TYPE_FP32", config)
        self.assertIn("kind: KIND_CPU", config)

    def test_generate_config_pbtxt_content_python_backend(self):
        """It renders a python backend config."""
        config = generate_config_pbtxt_content(
            self.gen,
            model_name="test_model",
            model_revision=None,
            model_schema=self.schema,
            backend=TritonBackendType.PYTHON,
        )

        self.assertIn('backend: "python"', config)
        self.assertIn("max_batch_size: 256", config)

    def test_generate_config_pbtxt_content_onnxruntime_backend(self):
        """It renders an onnxruntime backend config."""
        config = generate_config_pbtxt_content(
            self.gen,
            model_name="test_model",
            model_revision=None,
            model_schema=self.schema,
            backend=TritonBackendType.ONNX,
        )

        self.assertIn('backend: "onnxruntime"', config)
        self.assertIn("max_batch_size: 256", config)

    def test_generate_config_pbtxt_content_with_dynamic_batching(self):
        """It uses max_batch_size 256 and a dynamic_batching block when enabled."""
        config = generate_config_pbtxt_content(
            self.gen,
            model_name="test_model",
            model_revision=None,
            model_schema=self.schema,
            enable_dynamic_batching=True,
        )

        self.assertIn("max_batch_size: 256", config)
        self.assertIn("dynamic_batching: {", config)
        self.assertIn("preserve_ordering: true", config)

    def test_generate_config_pbtxt_content_without_dynamic_batching(self):
        """It uses max_batch_size 0 and omits dynamic_batching when disabled."""
        config = generate_config_pbtxt_content(
            self.gen,
            model_name="test_model",
            model_revision=None,
            model_schema=self.schema,
            enable_dynamic_batching=False,
        )

        self.assertIn("max_batch_size: 0", config)
        self.assertNotIn("dynamic_batching: {", config)

    def test_generate_config_pbtxt_content_appends_revision(self):
        """It appends the revision to the model name as ``name-revision``."""
        config = generate_config_pbtxt_content(
            self.gen,
            model_name="test_model",
            model_revision="7",
            model_schema=self.schema,
        )

        self.assertIn('name: "test_model-7"', config)

    def test_generate_config_pbtxt_content_without_revision(self):
        """It uses the bare model name when no revision is given."""
        config = generate_config_pbtxt_content(
            self.gen,
            model_name="test_model",
            model_revision=None,
            model_schema=self.schema,
        )

        self.assertIn('name: "test_model"', config)
        self.assertNotIn("test_model-", config)

    def test_generate_config_pbtxt_content_all_backends_render(self):
        """Every supported backend renders a non-empty config with its name."""
        for backend in (
            TritonBackendType.TORCH,
            TritonBackendType.PYTHON,
            TritonBackendType.ONNX,
        ):
            config = generate_config_pbtxt_content(
                self.gen,
                model_name="m",
                model_revision=None,
                model_schema=self.schema,
                backend=backend,
            )
            self.assertIn(f'backend: "{backend}"', config)
            self.assertTrue(config.strip())
