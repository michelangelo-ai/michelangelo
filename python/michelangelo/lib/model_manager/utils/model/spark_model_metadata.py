from __future__ import annotations
from dataclasses import dataclass


@dataclass
class SparkModelMetadata:
    """
    SparkModelMetadata is a dataclass that holds metadata about a Michelangelo spark model.

    Attributes:
        column_stats: A dictionary of column statistics of input features. Format:
            {
                "mu": {"feature1": 0.0, "feature2": 0.0},
                "sigma": {"feature1": 1.0, "feature2": 1.0},
                "min": {"feature1": 0.0, "feature2": 0.0},
                "max": {"feature1": 1.0, "feature2": 1.0},
            }
        basis_column_types: A dictionary of basis features. Format:
            {
                "numeric": ["feature1", "feature2"],
                "non-numeric": ["feature3", "feature4"]
                "vector": ["feature5"]
            }
        response_percentiles: A dictionary of response percentiles. Format:
            {
                "p01: {"feature1": 0.1, "feature2": 0.2},
                "p05: {"feature1": 0.5, "feature2": 0.6},
                "p10: {"feature1": 1.0, "feature2": 1.0},
                "p25: {"feature1": 2.0, "feature2": 2.0},
                "p50: {"feature1": 3.0, "feature2": 3.0},
                "p75: {"feature1": 4.0, "feature2": 4.0},
                "p90: {"feature1": 5.0, "feature2": 5.0},
                "p95: {"feature1": 6.0, "feature2": 6.0},
                "p99: {"feature1": 7.0, "feature2": 7.0},
            }
        feature_stats: A dictionary of feature statistics. Format:
            {
                "numerical_stats": {
                    "feature1": {... stats ...},
                    ...
                },
                "categorical_stats": {
                    "feature2": {... stats ...},
                    ...
                },
            }
    """

    column_stats: dict = None
    basis_columns_type: dict = None
    response_percentiles: dict = None
    feature_stats: dict = None

    def list_files(self):
        """
        list the metadata file contents along with the file name in a list of tuples

        Returns:
            A list of tuples containing metadata file contents and file name
        """
        return [
            (self.column_stats, "_COLUMN_STATS.yaml"),
            (self.basis_columns_type, "basis_columns_type.yaml"),
            (self.response_percentiles, "_RESPONSE_PERCENTILES.yaml"),
            (self.feature_stats, "_FEATURE_STATS.yaml"),
        ]
