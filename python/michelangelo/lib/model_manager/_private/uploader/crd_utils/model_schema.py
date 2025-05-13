from michelangelo.lib.model_manager.schema import ModelSchema
from michelangelo.lib.model_manager._private.uploader.crd_utils.data_type import convert_data_type
from uber.gen.code_uber_internal.uberai.michelangelo.api.v2beta1.model_proto import Model
from uber.gen.code_uber_internal.uberai.michelangelo.api.v2beta1.schema_proto import DataSchemaItem


def set_model_schema(model: Model, model_schema: ModelSchema):
    """
    Set the schema of the model.

    Args:
        model (Model): Model CRD object.
        model_schema (ModelSchema): Schema of the model.

    Returns:
        None
    """
    model.spec.input_schema.schema_items.extend(
        [
            DataSchemaItem(
                name=item.name,
                data_type=convert_data_type(item.data_type),
                shape=item.shape,
            )
            for item in model_schema.input_schema
        ]
    )

    model.spec.palette_features.schema_items.extend(
        [
            DataSchemaItem(
                name=item.name,
                data_type=convert_data_type(item.data_type),
                shape=item.shape,
            )
            for item in model_schema.feature_store_features_schema
        ]
    )

    model.spec.output_schema.schema_items.extend(
        [
            DataSchemaItem(
                name=item.name,
                data_type=convert_data_type(item.data_type),
                shape=item.shape,
            )
            for item in model_schema.output_schema
        ]
    )
