import os
from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton import generate_user_model_content


class UserModelPyTest(TestCase):
    def test_generate_user_model_content(self):
        gen = TritonTemplateRenderer()
        content = generate_user_model_content(gen)
        user_model_path = os.path.join(os.path.dirname(__file__), "fixtures", "user_model.py.txt")

        with open(user_model_path) as f:
            expected_content = f.read()

        self.assertEqual(content, expected_content)

    def test_generate_user_model_content_with_process_batch(self):
        gen = TritonTemplateRenderer()
        content = generate_user_model_content(gen, process_batch=True)
        user_model_path = os.path.join(os.path.dirname(__file__), "fixtures", "user_model_process_batch.py.txt")

        with open(user_model_path) as f:
            expected_content = f.read()

        self.assertEqual(content, expected_content)
