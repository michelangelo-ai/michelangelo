import os
import tempfile
from unittest.mock import patch
from pyspark.ml import Pipeline
from pyspark.ml.feature import VectorAssembler
from michelangelo._internal.testing.spark import SparkTestCase
from michelangelo.lib.model_manager.constants import ModelKind
from michelangelo.lib.model_manager.schema import (
    ModelSchema,
)
from michelangelo.lib.model_manager.utils.model import SparkModelMetadata
from michelangelo.lib.model_manager._private.packager.spark import generate_model_package_content

trained_model_yaml = """training_job_id: '0'
use_new: true
project_id: test_project
description: model description
model:
  is_tm_model: true
  type: canvas_custom
  artifact_version: 2
published_fields: []
features:
  derived: []
response_variable:
- feature: prediction
  transformation: sVal(preidction)
training_data:
  start_date: ''
  end_date: ''
"""


class ModelPackageTest(SparkTestCase):
    def setUp(self):
        super().setUp()
        tx = VectorAssembler(
            inputCols=[
                f"test{i}"
                for i in reversed(
                    range(1, 3),
                )
            ],
            outputCol="outputTestVector",
        )
        self.model = Pipeline(stages=[tx])

        self.sample_data = self.spark.createDataFrame(
            [(1, 2)],
            ["a", "b"],
        )

        self.model_metadata = SparkModelMetadata(
            column_stats={},
            basis_columns_type={},
        )
        self.model_schema = ModelSchema()

    @patch("uuid.uuid4")
    def test_generate_model_package_content(self, mock_uuid4):
        self.maxDiff = None
        mock_uuid4.return_value = "0"
        project_name = "test_project"
        model_desc = "model description"
        model_kind = ModelKind.CUSTOM

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")

            content = generate_model_package_content(
                project_name,
                self.model,
                self.sample_data,
                self.model_schema,
                self.model_metadata,
                model_desc,
                model_kind,
                dest_model_path,
            )

            expected_content = {
                "deploy": {
                    "_COLUMN_STATS.yaml": "{}\n",
                    "basis_columns_type.yaml": "{}\n",
                    "sample_data.csv": f"file://{dest_model_path}/deploy/sample_data.csv",
                    "trained_model.yaml": trained_model_yaml,
                    "project.yaml": "project:\n  id: test_project\n",
                },
                "deploy_jar": {
                    "model.jar.gz": f"file://{dest_model_path}/deploy_jar/model.jar.gz",
                },
            }
            self.assertEqual(content, expected_content)
