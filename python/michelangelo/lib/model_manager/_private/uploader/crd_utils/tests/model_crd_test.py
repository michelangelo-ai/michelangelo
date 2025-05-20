from unittest import TestCase
from michelangelo.lib.model_manager.constants import (
    ModelKind,
    PackageType,
)
from michelangelo.lib.model_manager.schema import (
    DataType,
    ModelSchema,
    ModelSchemaItem,
)
from michelangelo.lib.model_manager._private.uploader.crd_utils import create_model_crd
from michelangelo.gen.api.v2.model_pb2 import (
    Model,
    MODEL_KIND_CUSTOM,
    DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON,
)
from michelangelo.gen.api.v2.schema_pb2 import (
    DataSchemaItem,
    DATA_TYPE_INT,
    DATA_TYPE_STRING,
)


class ModelCrdTest(TestCase):
    def setUp(self):
        super().setUp()

        model_schema = ModelSchema()
        model_schema.input_schema = [
            ModelSchemaItem(name="input1", data_type=DataType.INT, shape=[1, 2]),
        ]
        model_schema.feature_store_features_schema = [
            ModelSchemaItem(name="feature1", data_type=DataType.STRING),
        ]
        model_schema.output_schema = [
            ModelSchemaItem(name="output1", data_type=DataType.INT, shape=[1]),
        ]
        self.model_schema = model_schema

    def test_create_model_crd_bare_minimum(self):
        model = create_model_crd(
            project_name="project_name",
            model_name="model_name",
        )
        expected_model = Model()
        expected_model.metadata.namespace = "project_name"
        expected_model.metadata.name = "model_name"
        expected_model.spec.kind = MODEL_KIND_CUSTOM
        expected_model.spec.package_type = DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON
        expected_model.spec.sealed = True
        expected_model.spec.deployable_artifact_uri.append(
            "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
        )
        self.assertEqual(model, expected_model)

    def test_create_model_crd(self):
        project_name = "project_name"
        model_name = "model_name"
        model_family = "model_family"
        model_desc = "model_desc"
        revision_id = 0
        performance_report_name = "performance_report_name"
        feature_report_name = "feature_report_name"
        labels = {"k1": "v1", "k2": "v2"}
        annotations = {"k1": "v1", "k2": "v2"}
        source_pipeline_run_name = "source_pipeline_run_name"
        llm_vendor = "llm_vendor"
        llm_fine_tuned_model_id = "llm_fine_tuned_model_id"
        training_framework = "custom-python"

        model = create_model_crd(
            project_name=project_name,
            model_name=model_name,
            package_type=PackageType.TRITON,
            model_kind=ModelKind.CUSTOM,
            model_family=model_family,
            model_desc=model_desc,
            model_schema=self.model_schema,
            revision_id=revision_id,
            performance_report_name=performance_report_name,
            feature_report_name=feature_report_name,
            sealed=True,
            deployable_artifact_uri=["deployable_artifact_uri"],
            raw_model_artifact_uri=["raw_model_artifact_uri"],
            labels=labels,
            annotations=annotations,
            source_pipeline_run_name=source_pipeline_run_name,
            llm_vendor=llm_vendor,
            llm_fine_tuned_model_id=llm_fine_tuned_model_id,
            training_framework=training_framework,
        )

        expected_model = Model()
        expected_model.metadata.namespace = project_name
        expected_model.metadata.name = model_name
        expected_model.metadata.labels["k1"] = "v1"
        expected_model.metadata.labels["k2"] = "v2"
        expected_model.metadata.annotations["k1"] = "v1"
        expected_model.metadata.annotations["k2"] = "v2"
        expected_model.spec.kind = MODEL_KIND_CUSTOM
        expected_model.spec.package_type = DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON
        expected_model.spec.model_family.namespace = project_name
        expected_model.spec.model_family.name = model_family
        expected_model.spec.sealed = True
        expected_model.spec.deployable_artifact_uri.append("deployable_artifact_uri")
        expected_model.spec.model_artifact_uri.append("raw_model_artifact_uri")
        expected_model.spec.description = model_desc
        expected_model.spec.input_schema.schema_items.append(
            DataSchemaItem(name="input1", data_type=DATA_TYPE_INT, shape=[1, 2]),
        )
        expected_model.spec.palette_features.schema_items.append(
            DataSchemaItem(name="feature1", data_type=DATA_TYPE_STRING),
        )
        expected_model.spec.output_schema.schema_items.append(
            DataSchemaItem(name="output1", data_type=DATA_TYPE_INT, shape=[1]),
        )
        expected_model.spec.performance_evaluation_report.name = performance_report_name
        expected_model.spec.feature_evaluation_report.name = feature_report_name
        expected_model.spec.source_pipeline_run.name = source_pipeline_run_name
        expected_model.spec.source_pipeline_run.namespace = project_name
        expected_model.spec.llm_spec.vendor = llm_vendor
        expected_model.spec.llm_spec.fine_tuned_model_id = llm_fine_tuned_model_id
        expected_model.spec.training_framework = training_framework

        self.assertEqual(model, expected_model)
