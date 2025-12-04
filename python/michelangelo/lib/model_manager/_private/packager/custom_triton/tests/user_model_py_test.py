import os
from unittest import TestCase

from michelangelo.lib.model_manager._private.packager.custom_triton import (
    generate_user_model_content,
)
from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TritonTemplateRenderer,
)


class UserModelPyTest(TestCase):
    """Tests user_model.py file generation."""

    def test_generate_user_model_content(self):
        """It generates the user_model.py file content."""
        gen = TritonTemplateRenderer()
        content = generate_user_model_content(gen)
        user_model_path = os.path.join(
            os.path.dirname(__file__), "fixtures", "user_model.py.txt"
        )

        with open(user_model_path) as f:
            expected_content = f.read()

        self.assertEqual(content, expected_content)

    def test_generate_user_model_content_with_process_batch(self):
        """It generates the user_model.py file content with process batch."""
        gen = TritonTemplateRenderer()
        content = generate_user_model_content(gen, process_batch=True)
        user_model_path = os.path.join(
            os.path.dirname(__file__), "fixtures", "user_model_process_batch.py.txt"
        )

        with open(user_model_path) as f:
            expected_content = f.read()

        self.assertEqual(content, expected_content)
