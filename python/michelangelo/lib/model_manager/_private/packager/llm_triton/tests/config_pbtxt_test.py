from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton import generate_config_pbtxt_content
from uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.tests.fixtures.sample_config_pbtxt import SAMPLE_CONFIG_PBTXT


class ConfigPbtxtContentTest(TestCase):
    def test_generate_config_pbtxt_content(self):
        gen = TritonTemplateRenderer()
        content = generate_config_pbtxt_content(gen, "model", "0")
        self.assertEqual(content, SAMPLE_CONFIG_PBTXT)
