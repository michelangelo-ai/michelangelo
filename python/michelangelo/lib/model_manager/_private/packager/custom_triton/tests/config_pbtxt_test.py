"""Tests for config.pbtxt template rendering."""

from textwrap import dedent
from unittest import TestCase

from michelangelo.lib.model_manager._private.packager.custom_triton import (
    generate_config_pbtxt_content,
)
from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TritonTemplateRenderer,
)


def _build_expected_config_pbtxt(name_line="", parameters_block=""):
    """Build the expected config.pbtxt body shared by all rendering tests."""
    body = dedent(
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
        """
    )
    tail = dedent(
        """\
        instance_group: [
          {
            kind: KIND_CPU,
            count: 1
          }
        ]
        """
    )
    return name_line + body + parameters_block + tail


class ConfigPbtxtTest(TestCase):
    """Tests config.pbtxt template rendering."""

    def test_generate_config_pbtxt_content(self):
        """It renders the expected config.pbtxt contents for name/revision variants."""
        cases = [
            ("test_model", "test_revision", 'name: "test_model-test_revision"\n'),
            ("test_model", None, 'name: "test_model"\n'),
            (None, "test_revision", ""),
            (1, None, 'name: "1"\n'),
            (1, 2.0, 'name: "1-2.0"\n'),
            (0, None, 'name: "0"\n'),
            (0, 0, 'name: "0-0"\n'),
        ]
        for model_name, model_revision, name_line in cases:
            with self.subTest(model_name=model_name, model_revision=model_revision):
                gen = TritonTemplateRenderer()
                config_pbtxt = generate_config_pbtxt_content(
                    gen,
                    model_name=model_name,
                    model_revision=model_revision,
                    input_schema={"input": {"data_type": "FP32", "shape": [1, 100]}},
                    output_schema={"output": {"data_type": "FP32", "shape": [1, 100]}},
                )

                self.assertEqual(
                    config_pbtxt, _build_expected_config_pbtxt(name_line=name_line)
                )

    def test_generate_config_pbtxt_content_with_triton_parameters(self):
        """It renders a parameters block for each triton_parameters entry."""
        gen = TritonTemplateRenderer()
        config_pbtxt = generate_config_pbtxt_content(
            gen,
            model_name="test_model",
            model_revision=None,
            input_schema={"input": {"data_type": "FP32", "shape": [1, 100]}},
            output_schema={"output": {"data_type": "FP32", "shape": [1, 100]}},
            triton_parameters={"MY_CUSTOM_PARAM": "16"},
        )

        parameters_block = dedent(
            """\
            parameters: {
              key: "MY_CUSTOM_PARAM"
              value: {
                string_value: "16"
              }
            }
            """
        )
        self.assertEqual(
            config_pbtxt,
            _build_expected_config_pbtxt(
                name_line='name: "test_model"\n', parameters_block=parameters_block
            ),
        )

    def test_generate_config_pbtxt_content_without_triton_parameters(self):
        """It omits the parameters block when triton_parameters is None."""
        gen = TritonTemplateRenderer()
        config_pbtxt = generate_config_pbtxt_content(
            gen,
            model_name="test_model",
            model_revision=None,
            input_schema={"input": {"data_type": "FP32", "shape": [1, 100]}},
            output_schema={"output": {"data_type": "FP32", "shape": [1, 100]}},
        )

        self.assertNotIn("parameters:", config_pbtxt)
