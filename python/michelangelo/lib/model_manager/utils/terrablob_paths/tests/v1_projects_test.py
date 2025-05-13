from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager.utils.terrablob_paths import (
    get_v1_projects_model_path,
    get_v1_projects_model_zip_path,
)


class V1ProjectsTest(TestCase):
    def test_get_v1_projects_model_path(self):
        project_id = "project_id"
        model_id = "model_id"
        self.assertEqual(
            get_v1_projects_model_path(project_id, model_id),
            f"/prod/michelangelo/v1_projects/{project_id}/trained_models/{model_id}",
        )

    def test_get_v1_projects_model_zip_path(self):
        project_id = "project_id"
        model_id = "model_id"
        self.assertEqual(
            get_v1_projects_model_zip_path(project_id, model_id),
            (f"/prod/michelangelo/v1_projects/{project_id}/trained_models/{model_id}/sparkml_proto/{project_id}-v2.zip"),
        )
