"""Feature schema definition."""

from dataclasses import dataclass, field

from michelangelo.lib.model_manager.schema.feature_schema_item import FeatureSchemaItem


@dataclass
class FeatureSchema:
    """Schema of the features produced by a feature-computation stage that
    precedes a model, e.g. a feature store lookup or a batch feature pipeline.

    This is intentionally shaped like ModelSchema so that a feature schema's
    input/feature-store/derived features can be fused into a model's own
    schema before packaging (see lib/shared/utils/model_metadata).

    Attributes:
        input_schema: The features provided directly by the caller.
        feature_store_features_schema: Additional features looked up from a
            feature store based on the input schema.
        derived_features_schema: Features computed from the input and
            feature-store features (e.g. via transforms), rather than
            supplied or looked up directly.
    """

    input_schema: list[FeatureSchemaItem] = field(default_factory=list)
    feature_store_features_schema: list[FeatureSchemaItem] = field(default_factory=list)
    derived_features_schema: list[FeatureSchemaItem] = field(default_factory=list)
