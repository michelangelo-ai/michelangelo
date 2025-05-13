import os
import tempfile
from unittest.mock import patch
from pyspark.ml import PipelineModel
from pyspark.ml.feature import VectorAssembler
from michelangelo._internal.testing.spark import SparkTestCase
from michelangelo.lib.model_manager.serde.spark import load_spark_pipeline_model
from michelangelo.lib.model_manager._private.constants.hdfs_paths import (
    HDFS_TMP_MODELS_DIR,
)


class LoadSparkPipelineModelTest(SparkTestCase):
    def setUp(self):
        super().setUp()
        tx = VectorAssembler(
            inputCols=[f"test{i}" for i in range(1, 3)],
            outputCol="outputTestVector",
        )
        self.model = PipelineModel([tx])

    def test_load_spark_pipeline_model_locally(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            self.model.write().save(model_path)
            loaded_model = load_spark_pipeline_model(model_path)
            self.assertEqual(loaded_model.stages[0].getInputCols(), ["test1", "test2"])
            self.assertEqual(loaded_model.stages[0].getOutputCol(), "outputTestVector")

    @patch("michelangelo.lib.model_manager.serde.spark.pipeline_model.get_spark_session")
    @patch("michelangelo.lib.model_manager.serde.spark.pipeline_model.create_dir_in_hdfs")
    @patch("michelangelo.lib.model_manager.serde.spark.pipeline_model.upload_to_hdfs")
    @patch("michelangelo.lib.model_manager.serde.spark.pipeline_model.MichelangeloPipelineModel.load")
    def test_load_spark_pipeline_model_hdfs(
        self,
        mock_model_load,
        mock_upload_to_hdfs,
        mock_create_dir_in_hdfs,
        mock_get_spark_session,
    ):
        mock_get_spark_session.return_value = self.spark
        with patch.object(self.spark, "_jsc", autospec=True) as mock_jsc:
            mock_jsc.isLocal.return_value = False
            mock_model_load.return_value = self.model
            loaded_model = load_spark_pipeline_model("model_path")

            mock_model_load.assert_called_once()
            arg = mock_model_load.call_args.args[0]
            self.assertTrue(arg.startswith(f"{HDFS_TMP_MODELS_DIR}/model-"))

            mock_create_dir_in_hdfs.assert_called_once_with(HDFS_TMP_MODELS_DIR)

            mock_upload_to_hdfs.assert_called_once()
            args = mock_upload_to_hdfs.call_args.args
            self.assertEqual(args[0], "model_path")
            self.assertTrue(args[1].startswith(f"{HDFS_TMP_MODELS_DIR}/model-"))

            self.assertIsNotNone(loaded_model)
