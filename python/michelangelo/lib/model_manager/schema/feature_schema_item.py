"""Feature schema item definition."""

from dataclasses import dataclass

from michelangelo.lib.model_manager.schema.data_type import DataType


@dataclass
class FeatureSchemaItem:
    """Represents a single feature produced by a feature-computation stage.

    Attributes:
        name: The name of the feature.
        data_type: The data type of the feature, specified as a DataType enum
            value.
        shape: The shape of the feature as a list of integers, following the
            same conventions as ModelSchemaItem.shape. If None, the shape is
            unspecified.
    """

    name: str
    data_type: DataType = DataType.UNKNOWN
    shape: list[int] = None
