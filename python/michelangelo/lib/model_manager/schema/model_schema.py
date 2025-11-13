from dataclasses import dataclass, field
from michelangelo.lib.model_manager.schema.model_schema_item import ModelSchemaItem


@dataclass
class ModelSchema:
    """
    The model schema specifies the input/palette features a model requires to make predictions,
    along with the data types of these features.

    Attributes:
        input_schema: A list of ModelSchemaItem representing the input features of the model.
        feature_store_features_schema: A list of ModelSchemaItem representing the palette features of the model.
        output_schema: A list of ModelSchemaItem representing the output features of the model.
    """

    input_schema: list[ModelSchemaItem] = field(default_factory=list)
    feature_store_features_schema: list[ModelSchemaItem] = field(default_factory=list)
    output_schema: list[ModelSchemaItem] = field(default_factory=list)