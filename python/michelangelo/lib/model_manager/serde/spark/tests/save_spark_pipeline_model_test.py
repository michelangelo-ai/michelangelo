import os
import tempfile
from unittest.mock import patch
from pyspark.ml import Pipeline
from pyspark.ml.feature import VectorAssembler
from uber.ai.michelangelo.shared.testing.spark import SparkTestCase
from uber.ai.michelangelo.sdk.model_manager.serde.spark import save_spark_pipeline_model
from uber.ai.michelangelo.sdk.model_manager._private.constants.hdfs_paths import (
    HDFS_TMP_MODELS_DIR,
)


def download_from_hdfs(
    src_path: str,  # noqa: ARG001
    des_path: str,
):
    os.makedirs(des_path, exist_ok=True)
    with open(f"{des_path}/test.txt", "w") as f:
        f.write("test")


class SaveSparkPipelineModelTest(SparkTestCase):
    def setUp(self):
        super().setUp()
        tx = VectorAssembler(
            inputCols=[f"test{i}" for i in range(1, 3)],
            outputCol="outputTestVector",
        )
        self.model = Pipeline(stages=[tx])

    def test_save_spark_pipeline_model_locally(self):
        model_path = save_spark_pipeline_model(self.model)
        self.assertTrue(os.listdir(model_path), ["metadata", "stages"])

    def test_save_spark_pipeline_model_locally_with_path(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = save_spark_pipeline_model(self.model, dest_model_path=temp_dir)
            self.assertTrue(os.listdir(model_path), ["metadata", "stages"])

            dest_model_path = os.path.join(temp_dir, "model")
            model_path = save_spark_pipeline_model(self.model, dest_model_path=dest_model_path)
            self.assertTrue(os.listdir(model_path), ["metadata", "stages"])

    @patch("uber.ai.michelangelo.sdk.model_manager.serde.spark.pipeline_model.get_spark_session")
    @patch("uber.ai.michelangelo.sdk.model_manager.serde.spark.pipeline_model.download_from_hdfs", wraps=download_from_hdfs)
    def test_save_spark_pipeline_model_hdfs(
        self,
        mock_download_from_hdfs,
        mock_get_spark_session,
    ):
        mock_get_spark_session.return_value = self.spark
        with patch.object(self.spark, "_jsc", autospec=True) as mock_jsc, patch.object(self.model, "save") as mock_model_save:
            mock_jsc.isLocal.return_value = False
            model_path = save_spark_pipeline_model(self.model)

            mock_model_save.assert_called_once()
            arg = mock_model_save.call_args.args[0]
            self.assertTrue(arg.startswith(f"{HDFS_TMP_MODELS_DIR}/model-"))

            mock_download_from_hdfs.assert_called_once()
            args = mock_download_from_hdfs.call_args.args
            self.assertTrue(args[0].startswith(f"{HDFS_TMP_MODELS_DIR}/model-"))
            self.assertTrue(args[1].startswith("/tmp/"))
            self.assertIsNotNone(model_path)

            with open(os.path.join(model_path, "test.txt")) as f:
                self.assertEqual(f.read(), "test")
