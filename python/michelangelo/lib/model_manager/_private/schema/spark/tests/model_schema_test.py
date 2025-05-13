from pyspark.ml.linalg import Vectors
from pyspark.ml import Pipeline
from unittest.mock import MagicMock
from uber.ai.michelangelo.shared.testing.spark import SparkTestCase
from michelangelo.lib.model_manager.schema import (
    ModelSchemaItem,
    DataType,
)
from michelangelo.lib.model_manager._private.schema.spark import create_model_schema
from uber.ai.michelangelo.sdk.compat.pyspark.ml.feature import (
    DecisionThresholdTransformer,
    MichelangeloResultPacker,
    MichelangeloDSL,
    PaletteTransformer,
)
from uber.ai.michelangelo.sdk.compat.pyspark.ml.xgboost import XGBoostClassifier


class ModelSchemaTest(SparkTestCase):
    def setUp(self):
        super().setUp()
        self.model, self.df = self.create_model_and_df()

    def create_model_and_df(self):
        palette_exprs = [
            "@palette:driver:dispatch_cancel_v2:days_since_signup:supply_vvid:1",
            "@palette:driver:dispatch_cancel_v2:weeks_since_signup:driver_uuid:1",
            "@palette:driver:dispatch_cancel_v2:weeks_since_signup:derived_uuid:1",
        ]
        df = self.spark.createDataFrame(
            [
                (0, Vectors.dense([0.3, 0.8, 0.7]), 0, "foo", 100, 101, 102),
                (1, Vectors.dense([0.5, 0.8, 0.5]), 1, "bar", 365, 366, 367),
            ],
            ["id", "features", "indexed_label", "tag", *palette_exprs],
        )

        dsl_map = {
            "uid": "nVal(id)",
            "ttag": "nVal(tag)",
            "days_since_signup": 'nFill(nVal("@palette:driver:dispatch_cancel_v2:days_since_signup:supply_vvid:1"), 0)',
        }

        palette_tx = PaletteTransformer()
        palette_tx.set_palette_map({k: k for k in palette_exprs})

        palette_tx_mock = MagicMock(wraps=palette_tx, spec=PaletteTransformer)
        palette_tx_mock.transform.side_effect = lambda x: x
        palette_tx_mock._java_obj = palette_tx._java_obj

        pipeline = Pipeline(
            stages=[
                palette_tx_mock,
                MichelangeloDSL(lambdas=dsl_map),
                XGBoostClassifier(featuresCol="features", labelCol="indexed_label"),
                DecisionThresholdTransformer(inputCol="probability"),
                MichelangeloResultPacker(renameMap={"id": "uuid"}, passThruCols=["tag", "true"]),
            ],
        )

        model = pipeline.fit(df)

        return model, df

    def test_create_model_schema(self):
        model_schema = create_model_schema(self.model, self.df)

        def key_func(item):
            return item.name

        expected_input_schema = [
            ModelSchemaItem(name="features", data_type=DataType.VECTOR),
            ModelSchemaItem(name="id", data_type=DataType.NUMERIC),
            ModelSchemaItem(name="tag", data_type=DataType.STRING),
        ]

        self.assertEqual(len(model_schema.input_schema), len(expected_input_schema))
        for index, item in enumerate(
            sorted(
                model_schema.input_schema,
                key=key_func,
            ),
        ):
            self.assertEqual(item.name, expected_input_schema[index].name)
            self.assertEqual(item.data_type, expected_input_schema[index].data_type)

        expected_palette_schema = [
            ModelSchemaItem(
                name="@palette:driver:dispatch_cancel_v2:days_since_signup:supply_vvid:1",
                data_type=DataType.NUMERIC,
            ),
            ModelSchemaItem(
                name="@palette:driver:dispatch_cancel_v2:weeks_since_signup:driver_uuid:1",
                data_type=DataType.NUMERIC,
            ),
        ]

        self.assertEqual(len(model_schema.feature_store_features_schema), len(expected_palette_schema))
        for index, item in enumerate(
            sorted(
                model_schema.feature_store_features_schema,
                key=key_func,
            ),
        ):
            self.assertEqual(item.name, expected_palette_schema[index].name)
            self.assertEqual(item.data_type, expected_palette_schema[index].data_type)

    def test_create_model_schema_with_include_derived_join_keys(self):
        model_schema = create_model_schema(self.model, self.df, include_palette_features_with_derived_join_keys=True)

        expected_palette_schema = [
            ModelSchemaItem(
                name="@palette:driver:dispatch_cancel_v2:days_since_signup:supply_vvid:1",
                data_type=DataType.NUMERIC,
            ),
            ModelSchemaItem(
                name="@palette:driver:dispatch_cancel_v2:weeks_since_signup:derived_uuid:1",
                data_type=DataType.NUMERIC,
            ),
            ModelSchemaItem(
                name="@palette:driver:dispatch_cancel_v2:weeks_since_signup:driver_uuid:1",
                data_type=DataType.NUMERIC,
            ),
        ]

        def key_func(item):
            return item.name

        self.assertEqual(len(model_schema.feature_store_features_schema), len(expected_palette_schema))
        for index, item in enumerate(
            sorted(
                model_schema.feature_store_features_schema,
                key=key_func,
            ),
        ):
            self.assertEqual(item.name, expected_palette_schema[index].name)
            self.assertEqual(item.data_type, expected_palette_schema[index].data_type)
