"""Model schema definition."""

from dataclasses import dataclass, field

from michelangelo.lib.model_manager.schema.model_schema_item import ModelSchemaItem


@dataclass
class ModelSchema:
    """Schema definition for model inputs, outputs, and feature store features.

    The model schema specifies the structure and data types of all features that
    a model consumes and produces. This includes:
    - Input features: Data provided directly in prediction requests
    - Palette (feature store) features: Additional features retrieved from a
      feature store based on input keys
    - Output features: Predictions or results produced by the model

    The schema is used for:
    - Generating Triton configuration files
    - Validating input/output data
    - Type conversion and serialization
    - Documentation and API contracts

    Each schema component is a list of ModelSchemaItem objects that define the
    name, data type, shape, and optionality of individual features.

    Attributes:
        input_schema: A list of ModelSchemaItem objects representing the input
            features that must be provided in prediction requests. These are
            typically the raw features or keys used to look up additional
            features from the feature store.
        feature_store_features_schema: A list of ModelSchemaItem objects
            representing palette features that are retrieved from a feature
            store. These features are looked up based on keys in the input
            schema and joined with the input features before being passed to
            the model.
        output_schema: A list of ModelSchemaItem objects representing the
            output features produced by the model's predictions. These define
            the structure and types of the prediction results.
    """

    input_schema: list[ModelSchemaItem] = field(default_factory=list)
    feature_store_features_schema: list[ModelSchemaItem] = field(default_factory=list)
    output_schema: list[ModelSchemaItem] = field(default_factory=list)
