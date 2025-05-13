import yaml
from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.sdk.model_manager.constants import ModelKind
from uber.ai.michelangelo.sdk.model_manager.schema import (
    ModelSchema,
    ModelSchemaItem,
    DataType,
)
from uber.ai.michelangelo.sdk.model_manager._private.packager.spark import generate_trained_model_yaml


class TrainedModelYamlTest(TestCase):
    @patch("uuid.uuid4")
    def test_generate_trained_model_yaml(self, mock_uuid4):
        mock_uuid4.return_value = "00000000-0000-0000-0000-000000000000"
        project_name = "project_name"
        model_desc = "model_desc"
        model_kind = ModelKind.CUSTOM
        model_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="a", data_type=DataType.NUMERIC),
                ModelSchemaItem(name="b", data_type=DataType.STRING),
            ],
            feature_store_features_schema=[
                ModelSchemaItem(name="@palette:foo:bar:baz:key", data_type=DataType.NUMERIC),
            ],
        )

        content = generate_trained_model_yaml(
            model_schema,
            project_name,
            model_desc,
            model_kind,
        )

        expected_content = yaml.dump(
            {
                "training_job_id": "00000000-0000-0000-0000-000000000000",
                "use_new": True,
                "project_id": project_name,
                "description": model_desc,
                "model": {
                    "is_tm_model": True,
                    "type": "canvas_custom",
                    "artifact_version": 2,
                },
                "published_fields": [],
                "features": {
                    "derived": [
                        {
                            "feature": "a",
                            "transformation": "nVal(a)",
                        },
                        {
                            "feature": "b",
                            "transformation": "sVal(b)",
                        },
                        {
                            "feature": "@palette:foo:bar:baz:key",
                            "transformation": "nVal(@palette:foo:bar:baz:key)",
                        },
                    ],
                },
                "response_variable": [
                    {
                        "feature": "prediction",
                        "transformation": "sVal(preidction)",
                    },
                ],
                "training_data": {
                    "start_date": "",
                    "end_date": "",
                },
            },
            sort_keys=False,
        )

        self.assertEqual(
            content,
            expected_content,
        )
