"""Tests for config.pbtxt template rendering."""

from textwrap import dedent
from unittest import TestCase

from michelangelo.lib.model_manager._private.packager.custom_triton import (
    generate_config_pbtxt_content,
)
from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TritonTemplateRenderer,
)


class ConfigPbtxtTest(TestCase):
    """Tests config.pbtxt template rendering."""
    def setUp(self):
        self.maxDiff = None

    def test_generate_config_pbtxt_content(self):
        """It renders the expected config.pbtxt contents."""
        gen = TritonTemplateRenderer()
        config_pbtxt = generate_config_pbtxt_content(
            gen,
            model_name="test_model",
            model_revision="test_revision",
            input_schema={"input": {"data_type": "FP32", "shape": [1, 100]}},
            output_schema={"output": {"data_type": "FP32", "shape": [1, 100]}},
        )

        expected_config_pbtxt = dedent(
            """\
            name: "test_model-test_revision"
            backend: "python"
            max_batch_size: 256
            dynamic_batching: {
              preferred_batch_size: 10,
              max_queue_delay_microseconds: 300,
              preserve_ordering: true
            }
            input : [
              {
                name: "input",
                data_type: TYPE_FP32,
                dims: [1, 100]
              }
            ]
            output: [
              {
                name: "output",
                data_type: TYPE_FP32,
                dims: [1, 100]
              }
            ]
            instance_group: [
              {
                kind: KIND_CPU,
                count: 1
              }
            ]
            """
        )
        self.assertEqual(config_pbtxt, expected_config_pbtxt)

    def test_generate_config_pbtxt_content_without_revision(self):
        """It renders the expected config.pbtxt contents."""
        gen = TritonTemplateRenderer()
        config_pbtxt = generate_config_pbtxt_content(
            gen,
            model_name="test_model",
            model_revision=None,
            input_schema={"input": {"data_type": "FP32", "shape": [1, 100]}},
            output_schema={"output": {"data_type": "FP32", "shape": [1, 100]}},
        )

        expected_config_pbtxt = dedent(
            """\
            name: "test_model"
            backend: "python"
            max_batch_size: 256
            dynamic_batching: {
              preferred_batch_size: 10,
              max_queue_delay_microseconds: 300,
              preserve_ordering: true
            }
            input : [
              {
                name: "input",
                data_type: TYPE_FP32,
                dims: [1, 100]
              }
            ]
            output: [
              {
                name: "output",
                data_type: TYPE_FP32,
                dims: [1, 100]
              }
            ]
            instance_group: [
              {
                kind: KIND_CPU,
                count: 1
              }
            ]
            """
        )
        self.assertEqual(config_pbtxt, expected_config_pbtxt)

    def test_generate_config_pbtxt_content_without_name(self):
        """It renders the expected config.pbtxt contents without name."""
        gen = TritonTemplateRenderer()
        config_pbtxt = generate_config_pbtxt_content(
            gen,
            model_name=None,
            model_revision="test_revision",
            input_schema={"input": {"data_type": "FP32", "shape": [1, 100]}},
            output_schema={"output": {"data_type": "FP32", "shape": [1, 100]}},
        )
        
        expected_config_pbtxt = dedent(
            """\
            backend: "python"
            max_batch_size: 256
            dynamic_batching: {
              preferred_batch_size: 10,
              max_queue_delay_microseconds: 300,
              preserve_ordering: true
            }
            input : [
              {
                name: "input",
                data_type: TYPE_FP32,
                dims: [1, 100]
              }
            ]
            output: [
              {
                name: "output",
                data_type: TYPE_FP32,
                dims: [1, 100]
              }
            ]
            instance_group: [
              {
                kind: KIND_CPU,
                count: 1
              }
            ]
            """
        )
        self.assertEqual(config_pbtxt, expected_config_pbtxt)