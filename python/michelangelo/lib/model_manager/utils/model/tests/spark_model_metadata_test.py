from unittest import TestCase
from michelangelo.lib.model_manager.utils.model import SparkModelMetadata


class SparkModelMetadataTest(TestCase):
    def test_empty_spark_model_metadata(self):
        model_metadata = SparkModelMetadata()
        self.assertEqual(
            model_metadata.list_files(),
            [
                (None, "_COLUMN_STATS.yaml"),
                (None, "basis_columns_type.yaml"),
                (None, "_RESPONSE_PERCENTILES.yaml"),
                (None, "_FEATURE_STATS.yaml"),
            ],
        )

    def test_spark_model_metadata(self):
        model_metadata = SparkModelMetadata(
            column_stats={},
            basis_columns_type={},
            response_percentiles={},
            feature_stats={},
        )

        self.assertEqual(
            model_metadata.list_files(),
            [
                ({}, "_COLUMN_STATS.yaml"),
                ({}, "basis_columns_type.yaml"),
                ({}, "_RESPONSE_PERCENTILES.yaml"),
                ({}, "_FEATURE_STATS.yaml"),
            ],
        )
