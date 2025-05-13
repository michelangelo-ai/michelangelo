from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.packager.spark import generate_project_yaml


class ProjectYamlTest(TestCase):
    def test_generate_project_yaml(self):
        project_name = "project_name"

        content = generate_project_yaml(project_name)

        self.assertEqual(
            content,
            "project:\n  id: project_name\n",
        )
