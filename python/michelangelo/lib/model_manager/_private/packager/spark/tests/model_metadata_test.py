from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager.schema import (
    ModelSchema,
    ModelSchemaItem,
    DataType,
)
from uber.ai.michelangelo.sdk.model_manager.utils.model import SparkModelMetadata
from uber.ai.michelangelo.sdk.model_manager._private.packager.spark import generate_model_metadata_content


class ModelMetadataContentTest(TestCase):
    def test_generate_model_metadata_content(self):
        model_schema = ModelSchema()
        model_metadata = SparkModelMetadata(
            column_stats={"a": {"mean": 1, "stddev": 2}},
            basis_columns_type={"numeric": ["a"], "non-numeric": [], "vector": []},
            response_percentiles={"a": [0.1, 0.9]},
            feature_stats={"a": {"mean": 1, "stddev": 2}},
        )

        content = generate_model_metadata_content(model_metadata, model_schema)

        self.assertEqual(
            content,
            {
                "_COLUMN_STATS.yaml": "a:\n  mean: 1\n  stddev: 2\n",
                "basis_columns_type.yaml": "numeric:\n- a\nnon-numeric: []\nvector: []\n",
                "_RESPONSE_PERCENTILES.yaml": "a:\n- 0.1\n- 0.9\n",
                "_FEATURE_STATS.yaml": "a:\n  mean: 1\n  stddev: 2\n",
            },
        )

    def test_generate_model_metadata_content_with_defaults(self):
        model_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="a", data_type=DataType.NUMERIC),
                ModelSchemaItem(name="b", data_type=DataType.BOOLEAN),
                ModelSchemaItem(name="c", data_type=DataType.VECTOR),
            ],
        )
        model_metadata = SparkModelMetadata()

        content = generate_model_metadata_content(model_metadata, model_schema)

        self.assertEqual(
            content,
            {
                "_COLUMN_STATS.yaml": "mu: {}\nsigma: {}\nmin: {}\nmax: {}\n",
                "basis_columns_type.yaml": "numeric:\n- a\nnon-numeric:\n- b\nvector:\n- c\n",
            },
        )
