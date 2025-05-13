from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager.utils.terrablob_paths import (
    get_deployable_model_base_path,
    get_deployable_model_path,
    get_deployable_model_tar_path,
)


class DeployableModelTest(TestCase):
    def test_get_deployable_model_base_path(self):
        path = get_deployable_model_base_path(
            "test_project",
            "test_model",
        )
        self.assertEqual(
            path,
            "/prod/michelangelo/deployable_models/projects/test_project/models/test_model",
        )

    def test_get_deployable_model_path(self):
        path = get_deployable_model_path(
            "test_project",
            "test_model",
            "test_revision",
            "test_package_type",
        )
        self.assertEqual(
            path,
            "/prod/michelangelo/deployable_models/projects/test_project/models/test_model/revisions/test_revision/package/test_package_type",
        )

    def test_get_deployable_model_tar_path(self):
        path = get_deployable_model_tar_path(
            "test_project",
            "test_model",
            "test_revision",
            "test_package_type",
        )
        self.assertEqual(
            path,
            "/prod/michelangelo/deployable_models/projects/test_project/models/test_model/revisions/test_revision/package/test_package_type/deploy_tar/model.tar",
        )
