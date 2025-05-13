import os
import tempfile
import shutil
from pyspark.ml import PipelineModel
from pyspark.ml.feature import VectorAssembler
from uber.ai.michelangelo.shared.testing.spark import SparkTestCase
from michelangelo.lib.model_manager._private.packager.spark import create_model_zip


class ModelZipTest(SparkTestCase):
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
        self.model = PipelineModel(stages=[tx])

    def test_create_model_zip(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            dest_dir = os.path.join(temp_dir, "model")
            zip_path = create_model_zip(self.model, "model", dest_dir)
            self.assertEqual(zip_path, os.path.join(dest_dir, "model.zip"))

            # test unzipping the arhive
            shutil.unpack_archive(
                zip_path,
                os.path.join(temp_dir, "model_unpacked"),
                "zip",
            )

            self.assertEqual(
                sorted(os.listdir(os.path.join(temp_dir, "model_unpacked"))),
                ["metadata", "stages"],
            )
