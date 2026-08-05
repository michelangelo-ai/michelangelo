"""Combine a feature schema with a model schema into an end-to-end schema.

Unlike model_fuser (which fuses two model artifacts/schemas, e.g. native
transform + predictor), this module composes a FeatureSchema (what a
feature-computation stage consumes/derives) with a ModelSchema (what the
model itself consumes/produces) into the schema actually needed at serving
time.
"""

from typing import Optional

from michelangelo.lib.model_manager.schema import ModelSchema, ModelSchemaItem
from michelangelo.lib.model_manager.schema.feature_schema import FeatureSchema
from michelangelo.lib.model_manager.schema.feature_schema_item import FeatureSchemaItem


def feature_schema_item_to_model_schema_item(item: FeatureSchemaItem) -> ModelSchemaItem:
    """Convert a FeatureSchemaItem to a ModelSchemaItem.

    FeatureSchemaItem and ModelSchemaItem share the same DataType enum, so
    this is a direct field copy.
    """
    return ModelSchemaItem(name=item.name, data_type=item.data_type, shape=item.shape)


def _dedup_convert(
    feature_items: "list[FeatureSchemaItem]",
    model_items: "list[ModelSchemaItem]",
    exclude: frozenset = frozenset(),
) -> "list[ModelSchemaItem]":
    """Union feature_items (converted to ModelSchemaItem) and model_items, deduped by name
    (feature side wins on name collision, since it holds the raw/serving-facing values), minus
    any name in exclude.
    """
    seen = set()
    items: list[ModelSchemaItem] = []
    for item in feature_items or []:
        if item.name in exclude or item.name in seen:
            continue
        seen.add(item.name)
        items.append(feature_schema_item_to_model_schema_item(item))
    for item in model_items or []:
        if item.name in exclude or item.name in seen:
            continue
        seen.add(item.name)
        items.append(item)
    return items


def fuse_e2e_schema(feature_schema: Optional[FeatureSchema], model_schema: ModelSchema) -> ModelSchema:
    """Combine a feature schema with a model schema into the end-to-end serving schema.

    Serving tensor inputs = (feature_input_features union model_input_schema_fields)
                             - feature_derived_feature_fields

    feature_store_features_schema from both sides is unioned (deduped by name) into the result,
    even though it isn't part of the input-composition formula above.
    """
    if feature_schema is None:
        return model_schema
    derived_names = {item.name for item in feature_schema.derived_features_schema}
    input_items = _dedup_convert(feature_schema.input_schema, model_schema.input_schema, exclude=derived_names)
    palette_items = _dedup_convert(feature_schema.feature_store_features_schema, model_schema.feature_store_features_schema)
    return ModelSchema(
        input_schema=input_items,
        feature_store_features_schema=palette_items,
        output_schema=list(model_schema.output_schema),
    )
