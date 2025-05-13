from unittest import TestCase
from michelangelo.lib.model_manager.constants import PackageType
from michelangelo.lib.model_manager._private.uploader.crd_utils import get_deployable_artifact_uri


class DeployableArtifactUriTest(TestCase):
    def test_get_deployable_artifact_uri(self):
        project_name = "test_project"
        model_name = "test_model"
        model_revision = "0"

        uri = get_deployable_artifact_uri(project_name, model_name, model_revision, PackageType.SPARK)
        self.assertEqual(uri, f"/prod/michelangelo/v2_projects/{project_name}/trained_models/{model_name}/{model_revision}/deploy_jar/model.jar.gz")

        uri = get_deployable_artifact_uri(project_name, model_name, model_revision, PackageType.MOBILE)
        self.assertEqual(
            uri,
            f"/prod/michelangelo/deployable_models/projects/{project_name}/models/{model_name}/revisions/{model_revision}/package/mobile/deploy_gz/model.gz",
        )

        for package_type in (PackageType.TRITON, PackageType.RAW, "any"):
            uri = get_deployable_artifact_uri(project_name, model_name, model_revision, package_type)
            self.assertEqual(
                uri,
                f"/prod/michelangelo/deployable_models/projects/{project_name}/models/{model_name}/revisions/{model_revision}/package/{package_type}/deploy_tar/model.tar",
            )
