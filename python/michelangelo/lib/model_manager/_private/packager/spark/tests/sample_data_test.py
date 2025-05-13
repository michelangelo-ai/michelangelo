import os
import tempfile
from pyspark.ml.linalg import Vectors
from michelangelo._internal.testing.spark import SparkTestCase
from michelangelo.lib.model_manager._private.packager.spark import create_sample_data_csv


class SampleDataTest(SparkTestCase):
    def setUp(self):
        super().setUp()
        self.sample_data = self.spark.createDataFrame(
            [
                ("x", 1, Vectors.dense([1, 2, 3]), Vectors.sparse(4, {1: 1, 3: 3})),
                (None, 2, Vectors.dense([2, 3, 4]), None),
                ("y", 3, Vectors.dense([2, 3, 4]), None),
            ],
            ["a", "b", "c", "d"],
        )

    def test_create_sample_data_csv(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            csv_path = os.path.join(temp_dir, "sample_data.csv")
            create_sample_data_csv(self.sample_data, csv_path)
            with open(csv_path) as f:
                content = f.read()
                self.assertEqual(
                    content,
                    (
                        "a,b,c,d\n"
                        'x,1,"{""c"":{""type"":1,""values"":[1.0,2.0,3.0]}}","{""d"":{""type"":0,""size"":4,""indices"":[1,3],""values"":[1.0,3.0]}}"\n'
                        ',2,"{""c"":{""type"":1,""values"":[2.0,3.0,4.0]}}",{}\n'
                        'y,3,"{""c"":{""type"":1,""values"":[2.0,3.0,4.0]}}",{}\n'
                    ),
                )
