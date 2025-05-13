from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton import generate_model_py_content


class ModelPyTest(TestCase):
    def test_generate_model_py_content(self):
        gen = TritonTemplateRenderer()
        content = generate_model_py_content(gen)
        self.assertIsNotNone(content)
