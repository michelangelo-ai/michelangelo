from dataclasses import dataclass
from uber.ai.michelangelo.sdk.model_manager.schema.data_type import DataType


@dataclass
class ModelSchemaItem:
    """
    ModelSchemaItem represents a single feature in a model schema.

    Attributes:
        name: The name of the feature.
        data_type: The data type of the feature.
        shape: The shape of the feature. For example, [10, 5] for a 2D array with 10 rows and 5 columns.
    """

    name: str
    data_type: DataType = DataType.UNKNOWN
    shape: list[int] = None
