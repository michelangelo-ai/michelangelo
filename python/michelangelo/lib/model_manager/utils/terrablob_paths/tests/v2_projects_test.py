from unittest import TestCase
from michelangelo.lib.model_manager.utils.terrablob_paths import (
    get_v2_projects_model_base_path,
    get_v2_projects_model_path,
    get_v2_projects_model_jar_path,
)


class V2ProjectsTest(TestCase):
    def test_get_v2_projects_model_base_path(self):
        path = get_v2_projects_model_base_path(
            "test_project",
            "test_model",
        )
        self.assertEqual(
            path,
            "/prod/michelangelo/v2_projects/test_project/trained_models/test_model",
        )

    def test_get_deployable_model_tar_path(self):
        path = get_v2_projects_model_path(
            "test_project",
            "test_model",
            "0",
        )
        self.assertEqual(
            path,
            "/prod/michelangelo/v2_projects/test_project/trained_models/test_model/0",
        )

    def test_get_v2_projects_model_jar_path(self):
        path = get_v2_projects_model_jar_path(
            "test_project",
            "test_model",
            "0",
        )
        self.assertEqual(
            path,
            "/prod/michelangelo/v2_projects/test_project/trained_models/test_model/0/deploy_jar/model.jar.gz",
        )
