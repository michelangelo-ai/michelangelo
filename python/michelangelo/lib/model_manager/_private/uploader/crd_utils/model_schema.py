from michelangelo.lib.model_manager.schema import ModelSchema
from michelangelo.lib.model_manager._private.uploader.crd_utils.data_type import convert_data_type
from michelangelo.gen.api.v2.model_pb2 import Model
from michelangelo.gen.api.v2.schema_pb2 import DataSchemaItem


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
