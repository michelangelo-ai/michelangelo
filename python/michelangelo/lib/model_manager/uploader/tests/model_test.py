import os
import yaml
import tempfile
from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager.constants import PackageType, ModelKind
from michelangelo.lib.model_manager.schema import ModelSchema
from michelangelo.lib.model_manager.uploader import upload_model
from michelangelo.gen.api.v2.model_pb2 import (
    Model,
    MODEL_KIND_CUSTOM,
    DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON,
)


class ModelTest(TestCase):
    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager.uploader.raw_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager.uploader.model.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager.uploader.model.infer_model_package_type")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelService.create_model")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelFamilyService.create_model_family")
    @patch.dict(os.environ, {"UF_TASK_IMAGE": "image"})
    def test_upload_model(
        self,
        mock_create_model_family,
        mock_create_model,
        mock_infer_model_package_type,
        mock_get_latest_model_revision_id,
        mock_upload_raw_model_to_terrablob,
        mock_upload_deployable_to_terrablob,
    ):
        expected_tb_deployable_path = (
            "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
        )
        expected_tb_raw_path = "/prod/michelangelo/raw_models/projects/project_name/models/model_name/revisions/0/main"
        expected_model = Model()
        expected_model.metadata.namespace = "project_name"
        expected_model.metadata.name = "model_name"
        expected_model.metadata.labels["label1"] = "value1"
        expected_model.metadata.annotations["annotation1"] = "value1"
        expected_model.spec.model_family.name = "model_family"
        expected_model.spec.model_family.namespace = "project_name"
        expected_model.spec.description = "model_desc"
        expected_model.spec.kind = MODEL_KIND_CUSTOM
        expected_model.spec.package_type = DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON
        expected_model.spec.sealed = True
        expected_model.spec.input_schema.schema_items.extend([])
        expected_model.spec.output_schema.schema_items.extend([])
        expected_model.spec.deployable_artifact_uri.append(expected_tb_deployable_path)
        expected_model.spec.model_artifact_uri.append(expected_tb_raw_path)
        expected_model.spec.performance_evaluation_report.name = "performance_report_name"
        expected_model.spec.feature_evaluation_report.name = "feature_report_name"
        expected_model.spec.source_pipeline_run.name = "source_pipeline_run_name"
        expected_model.spec.source_pipeline_run.namespace = "project_name"
        expected_model.spec.llm_spec.vendor = "llm_vendor"
        expected_model.spec.llm_spec.fine_tuned_model_id = "fine_tuned_model_id"
        expected_model.spec.training_framework = "custom-python"

        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            raw_model_path = os.path.join(temp_dir, "raw_model")
            os.makedirs(model_path)
            os.makedirs(os.path.join(raw_model_path, "metadata"))
            with open(os.path.join(raw_model_path, "metadata", "type.yaml"), "w") as f:
                yaml.dump({"type": "custom-python"}, f)

            tb_deployable_model, tb_raw_model, model = upload_model(
                project_name="project_name",
                deployable_model_path=model_path,
                raw_model_path=raw_model_path,
                model_name="model_name",
                revision_id=0,
                package_type=PackageType.TRITON,
                timeout="2h",
                model_kind=ModelKind.CUSTOM,
                model_family="model_family",
                model_desc="model_desc",
                model_schema=ModelSchema(),
                performance_report_name="performance_report_name",
                feature_report_name="feature_report_name",
                sealed=True,
                labels={"label1": "value1"},
                annotations={"annotation1": "value1"},
                source_pipeline_run_name="source_pipeline_run_name",
                llm_vendor="llm_vendor",
                llm_fine_tuned_model_id="fine_tuned_model_id",
            )
            mock_infer_model_package_type.assert_not_called()
            mock_get_latest_model_revision_id.assert_not_called()
            mock_upload_deployable_to_terrablob.assert_called()
            mock_create_model.assert_called_once()

            args, kwargs = mock_upload_deployable_to_terrablob.call_args
            self.assertTrue(args[0].endswith("model.tar"))
            self.assertEqual(args[1], expected_tb_deployable_path)
            self.assertEqual(kwargs["timeout"], "2h")
            self.assertEqual(kwargs["source_entity"], "michelangelo-apiserver")

            mock_upload_raw_model_to_terrablob.assert_called_once_with(
                raw_model_path,
                "/prod/michelangelo/raw_models/projects/project_name/models/model_name/revisions/0/main",
                timeout="2h",
                source_entity="michelangelo-apiserver",
                auth_mode=None,
            )

            mock_create_model_family.assert_called_once()

            self.assertEqual(tb_deployable_model, expected_tb_deployable_path)
            self.assertEqual(tb_raw_model, expected_tb_raw_path)
            self.assertEqual(model, expected_model)

    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager.uploader.raw_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager.uploader.model.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager.uploader.model.infer_model_package_type")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelService.create_model")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelFamilyService.create_model_family")
    def test_upload_model_with_no_raw_model(
        self,
        mock_create_model_family,
        mock_create_model,
        mock_infer_model_package_type,
        mock_get_latest_model_revision_id,
        mock_upload_raw_model_to_terrablob,
        mock_upload_deployable_to_terrablob,
    ):
        expected_tb_deployable_path = (
            "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
        )
        expected_model = Model()
        expected_model.metadata.namespace = "project_name"
        expected_model.metadata.name = "model_name"
        expected_model.metadata.labels["label1"] = "value1"
        expected_model.metadata.annotations["annotation1"] = "value1"
        expected_model.spec.model_family.name = "model_family"
        expected_model.spec.model_family.namespace = "project_name"
        expected_model.spec.description = "model_desc"
        expected_model.spec.kind = MODEL_KIND_CUSTOM
        expected_model.spec.package_type = DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON
        expected_model.spec.sealed = True
        expected_model.spec.input_schema.schema_items.extend([])
        expected_model.spec.output_schema.schema_items.extend([])
        expected_model.spec.deployable_artifact_uri.append(expected_tb_deployable_path)
        expected_model.spec.performance_evaluation_report.name = "performance_report_name"
        expected_model.spec.feature_evaluation_report.name = "feature_report_name"

        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            tb_deployable_model, tb_raw_model, model = upload_model(
                project_name="project_name",
                deployable_model_path=model_path,
                model_name="model_name",
                revision_id=0,
                package_type=PackageType.TRITON,
                timeout="2h",
                model_kind=ModelKind.CUSTOM,
                model_family="model_family",
                model_desc="model_desc",
                model_schema=ModelSchema(),
                performance_report_name="performance_report_name",
                feature_report_name="feature_report_name",
                sealed=True,
                labels={"label1": "value1"},
                annotations={"annotation1": "value1"},
            )
            mock_infer_model_package_type.assert_not_called()
            mock_get_latest_model_revision_id.assert_not_called()
            mock_upload_deployable_to_terrablob.assert_called()
            mock_create_model.assert_called_once()

            args, kwargs = mock_upload_deployable_to_terrablob.call_args
            self.assertTrue(args[0].endswith("model.tar"))
            self.assertEqual(args[1], expected_tb_deployable_path)
            self.assertEqual(kwargs["timeout"], "2h")
            self.assertEqual(kwargs["source_entity"], "michelangelo-apiserver")

            mock_upload_raw_model_to_terrablob.assert_not_called()

            mock_create_model_family.assert_called_once()

            self.assertEqual(tb_deployable_model, expected_tb_deployable_path)
            self.assertIsNone(tb_raw_model)
            self.assertEqual(model, expected_model)

    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager.uploader.raw_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager.uploader.model.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager.uploader.model.infer_model_package_type")
    @patch("michelangelo.lib.model_manager.uploader.model.generate_random_name")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelService.create_model")
    @patch.dict(os.environ, {"UF_TASK_IMAGE": "image"})
    def test_upload_model_with_autogen_params(
        self,
        mock_create_model,
        mock_generate_random_name,
        mock_infer_model_package_type,
        mock_get_latest_model_revision_id,
        mock_upload_raw_model_to_terrablob,
        mock_upload_deployable_to_terrablob,
    ):
        mock_generate_random_name.return_value = "model_name"
        mock_get_latest_model_revision_id.return_value = -1
        mock_infer_model_package_type.return_value = PackageType.TRITON

        expected_tb_deployable_path = (
            "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
        )
        expected_tb_raw_path = "/prod/michelangelo/raw_models/projects/project_name/models/model_name/revisions/0/main"
        expected_model = Model()
        expected_model.metadata.namespace = "project_name"
        expected_model.metadata.name = "model_name"
        expected_model.spec.kind = MODEL_KIND_CUSTOM
        expected_model.spec.package_type = DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON
        expected_model.spec.sealed = True
        expected_model.spec.deployable_artifact_uri.append(expected_tb_deployable_path)
        expected_model.spec.model_artifact_uri.append(expected_tb_raw_path)

        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            tb_deployable_model, tb_raw_model, model = upload_model(
                project_name="project_name",
                deployable_model_path=model_path,
                raw_model_path="raw_model",
            )
            mock_infer_model_package_type.assert_called_once()
            mock_get_latest_model_revision_id.assert_called_once()
            mock_generate_random_name.assert_called_once()
            mock_upload_deployable_to_terrablob.assert_called()
            mock_create_model.assert_called_once()

            args, kwargs = mock_upload_deployable_to_terrablob.call_args
            self.assertTrue(args[0].endswith("model.tar"))
            self.assertEqual(args[1], expected_tb_deployable_path)
            self.assertEqual(kwargs["timeout"], None)
            self.assertEqual(kwargs["source_entity"], "michelangelo-apiserver")

            mock_upload_raw_model_to_terrablob.assert_called_once_with(
                "raw_model",
                "/prod/michelangelo/raw_models/projects/project_name/models/model_name/revisions/0/main",
                timeout=None,
                source_entity="michelangelo-apiserver",
                auth_mode=None,
            )

            self.assertEqual(tb_deployable_model, expected_tb_deployable_path)
            self.assertEqual(tb_raw_model, expected_tb_raw_path)
            self.assertEqual(model, expected_model)

    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager.uploader.raw_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager.uploader.model.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager.uploader.model.infer_model_package_type")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelService.create_model")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelFamilyService.create_model_family")
    @patch.dict(os.environ, {"UF_TASK_IMAGE": "image"})
    def test_upload_model_with_source_entity(
        self,
        mock_create_model_family,
        mock_create_model,
        mock_infer_model_package_type,
        mock_get_latest_model_revision_id,
        mock_upload_raw_model_to_terrablob,
        mock_upload_deployable_to_terrablob,
    ):
        expected_tb_deployable_path = (
            "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
        )
        expected_tb_raw_path = "/prod/michelangelo/raw_models/projects/project_name/models/model_name/revisions/0/main"
        expected_model = Model()
        expected_model.metadata.namespace = "project_name"
        expected_model.metadata.name = "model_name"
        expected_model.metadata.labels["label1"] = "value1"
        expected_model.metadata.annotations["annotation1"] = "value1"
        expected_model.spec.model_family.name = "model_family"
        expected_model.spec.model_family.namespace = "project_name"
        expected_model.spec.description = "model_desc"
        expected_model.spec.kind = MODEL_KIND_CUSTOM
        expected_model.spec.package_type = DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON
        expected_model.spec.sealed = True
        expected_model.spec.input_schema.schema_items.extend([])
        expected_model.spec.output_schema.schema_items.extend([])
        expected_model.spec.deployable_artifact_uri.append(expected_tb_deployable_path)
        expected_model.spec.model_artifact_uri.append(expected_tb_raw_path)
        expected_model.spec.performance_evaluation_report.name = "performance_report_name"
        expected_model.spec.feature_evaluation_report.name = "feature_report_name"
        expected_model.spec.source_pipeline_run.name = "source_pipeline_run_name"
        expected_model.spec.source_pipeline_run.namespace = "project_name"
        expected_model.spec.llm_spec.vendor = "llm_vendor"
        expected_model.spec.llm_spec.fine_tuned_model_id = "fine_tuned_model_id"

        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            tb_deployable_model, tb_raw_model, model = upload_model(
                project_name="project_name",
                deployable_model_path=model_path,
                raw_model_path="raw_model",
                model_name="model_name",
                revision_id=0,
                package_type=PackageType.TRITON,
                timeout="2h",
                model_kind=ModelKind.CUSTOM,
                model_family="model_family",
                model_desc="model_desc",
                model_schema=ModelSchema(),
                performance_report_name="performance_report_name",
                feature_report_name="feature_report_name",
                sealed=True,
                labels={"label1": "value1"},
                annotations={"annotation1": "value1"},
                source_pipeline_run_name="source_pipeline_run_name",
                llm_vendor="llm_vendor",
                llm_fine_tuned_model_id="fine_tuned_model_id",
                source_entity="test_entity",
            )
            mock_infer_model_package_type.assert_not_called()
            mock_get_latest_model_revision_id.assert_not_called()
            mock_upload_deployable_to_terrablob.assert_called()
            mock_create_model.assert_called_once()

            args, kwargs = mock_upload_deployable_to_terrablob.call_args
            self.assertTrue(args[0].endswith("model.tar"))
            self.assertEqual(args[1], expected_tb_deployable_path)
            self.assertEqual(kwargs["timeout"], "2h")
            self.assertEqual(kwargs["source_entity"], "test_entity")

            mock_upload_raw_model_to_terrablob.assert_called_once_with(
                "raw_model",
                "/prod/michelangelo/raw_models/projects/project_name/models/model_name/revisions/0/main",
                timeout="2h",
                source_entity="test_entity",
                auth_mode=None,
            )

            mock_create_model_family.assert_called_once()

            self.assertEqual(tb_deployable_model, expected_tb_deployable_path)
            self.assertEqual(tb_raw_model, expected_tb_raw_path)
            self.assertEqual(model, expected_model)
