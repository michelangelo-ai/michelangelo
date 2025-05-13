import os
import tempfile
import shutil
import yaml
from pyspark.ml import Pipeline
from pyspark.ml.feature import VectorAssembler
from pyspark.ml.linalg import Vectors
from michelangelo._internal.testing.spark import SparkTestCase
from michelangelo.lib.model_manager.schema import (
    ModelSchema,
    ModelSchemaItem,
    DataType,
)
from michelangelo.lib.model_manager.packager.spark import SparkModelPackager
from michelangelo.lib.model_manager._private.utils.file_utils.gzip import gzip_decompress
from michelangelo.lib.model_manager._private.utils.file_utils import cd
from uber.ai.michelangelo.sdk.compat.pyspark.ml.feature import (
    MichelangeloResultPacker,
    MichelangeloDSL,
)


class SparkModelPackagerTest(SparkTestCase):
    def setUp(self):
        super().setUp()
        self.setUpSimpleModel()
        self.setUpMAModel()

    def setUpSimpleModel(self):
        tx = VectorAssembler(
            inputCols=[f"test{i}" for i in range(1, 3)],
            outputCol="outputTestVector",
        )

        self.simple_sample_data = self.spark.createDataFrame(
            [(1, 2)],
            ["test1", "test2"],
        )

        pipeline = Pipeline(stages=[tx])
        self.simple_model = pipeline.fit(self.simple_sample_data)

    def setUpMAModel(self):
        """
        Create a model with Michelangelo transformers.
        """
        self.ma_sample_data = self.spark.createDataFrame(
            [
                (0, Vectors.dense([0.3, 0.8, 0.7]), 0, "foo", 1),
                (1, Vectors.dense([0.5, 0.8, 0.5]), 1, "bar", 2),
            ],
            ["id", "features", "indexed_label", "tag", "x"],
        )

        dsl_map = {
            "derived_tag": "sVal(tag)",
            "derived_x": "nVal(x)",
        }

        pipeline = Pipeline(
            stages=[
                MichelangeloDSL(lambdas=dsl_map),
                MichelangeloResultPacker(renameMap={"id": "uuid"}, passThruCols=["tag", "true"]),
            ],
        )

        self.ma_model = pipeline.fit(self.ma_sample_data)

    def assertModelPackage(
        self,
        model_path: str,
        expected_files: list[str],
    ):
        files = sorted([os.path.join(dirpath, file) for dirpath, _, filenames in os.walk(model_path) for file in filenames])

        self.assertEqual(
            files,
            sorted(
                [os.path.join(model_path, "deploy", file) for file in expected_files]
                + [
                    os.path.join(model_path, "deploy_jar", "model.jar.gz"),
                ],
            ),
        )

        with tempfile.TemporaryDirectory() as temp_dir:
            gzip_decompress(
                os.path.join(model_path, "deploy_jar/model.jar.gz"),
                os.path.join(temp_dir, "model.jar"),
            )

            model_unpacked = os.path.join(temp_dir, "model")
            os.makedirs(model_unpacked)

            with cd(model_unpacked):
                os.system("jar xf ../model.jar")

            files = sorted([os.path.join(dirpath, file) for dirpath, _, filenames in os.walk(model_unpacked) for file in filenames])

            self.assertEqual(
                files,
                sorted(
                    [os.path.join(model_unpacked, file) for file in expected_files]
                    + [
                        os.path.join(model_unpacked, "project_name.zip"),
                    ],
                ),
            )

            model_binary = os.path.join(temp_dir, "model_binary")
            shutil.unpack_archive(
                os.path.join(model_unpacked, "project_name.zip"),
                model_binary,
                "zip",
            )

            self.assertEqual(
                sorted(os.listdir(model_binary)),
                ["metadata", "stages"],
            )

    def assertFeatureSchema(
        self,
        model_path: str,
        expected_schema: list[str],
    ):
        with open(
            os.path.join(
                model_path,
                "deploy",
                "trained_model.yaml",
            ),
        ) as f:
            content = yaml.safe_load(f)
            self.assertEqual(
                sorted(
                    content["features"]["derived"],
                    key=lambda x: x["feature"],
                ),
                sorted(
                    expected_schema,
                    key=lambda x: x["feature"],
                ),
            )

    def test_create_model_package(self):
        packager = SparkModelPackager()
        model_path = packager.create_model_package(
            project_name="project_name",
            assembled_model=self.simple_model,
            sample_data=self.simple_sample_data,
        )

        self.assertModelPackage(
            model_path,
            [
                "_COLUMN_STATS.yaml",
                "basis_columns_type.yaml",
                "project.yaml",
                "sample_data.csv",
                "trained_model.yaml",
            ],
        )

        self.assertFeatureSchema(model_path, [])

    def test_create_model_package_with_schema_generation(self):
        packager = SparkModelPackager()
        model_path = packager.create_model_package(
            project_name="project_name",
            assembled_model=self.ma_model,
            sample_data=self.ma_sample_data,
        )

        self.assertModelPackage(
            model_path,
            [
                "_COLUMN_STATS.yaml",
                "basis_columns_type.yaml",
                "project.yaml",
                "sample_data.csv",
                "trained_model.yaml",
            ],
        )

        self.assertFeatureSchema(
            model_path,
            [
                {
                    "feature": "tag",
                    "transformation": "sVal(tag)",
                },
                {
                    "feature": "x",
                    "transformation": "nVal(x)",
                },
            ],
        )

    def test_create_model_package_with_custom_schema(self):
        model_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="feature", data_type=DataType.NUMERIC),
            ],
            feature_store_features_schema=[
                ModelSchemaItem(name="@palette:foo:bar:baz:key", data_type=DataType.STRING),
            ],
        )
        packager = SparkModelPackager()
        model_path = packager.create_model_package(
            project_name="project_name",
            assembled_model=self.simple_model,
            sample_data=self.simple_sample_data,
            model_schema=model_schema,
        )

        self.assertModelPackage(
            model_path,
            [
                "_COLUMN_STATS.yaml",
                "basis_columns_type.yaml",
                "project.yaml",
                "sample_data.csv",
                "trained_model.yaml",
            ],
        )

        self.assertFeatureSchema(
            model_path,
            [
                {
                    "feature": "feature",
                    "transformation": "nVal(feature)",
                },
                {
                    "feature": "@palette:foo:bar:baz:key",
                    "transformation": "sVal(@palette:foo:bar:baz:key)",
                },
            ],
        )
