"""Schema serialization and deserialization."""

from dataclasses import asdict, fields

import yaml

from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem


def schema_to_yaml(schema: ModelSchema) -> str:
    """Convert a ModelSchema to a YAML string.

    Args:
        schema: The ModelSchema to convert.

    Returns:
        A YAML string representation of the schema.
    """
    schema_dict = schema_to_dict(schema)
    return yaml.dump(schema_dict, sort_keys=False)


def schema_to_dict(schema: ModelSchema) -> dict:
    """Convert a ModelSchema to a dictionary.

    Args:
        schema: The ModelSchema to convert.

    Returns:
        A dictionary representation of the schema.
    """
    schema_dict = asdict(schema)

    for field in fields(ModelSchema):
        for schema_item in schema_dict[field.name]:
            for key in list(schema_item.keys()):
                if schema_item[key] is None:
                    del schema_item[key]
            if "data_type" in schema_item:
                schema_item["data_type"] = schema_item["data_type"].name.lower()

    return schema_dict


def dict_to_schema(schema_dict: dict) -> ModelSchema:
    """Convert a dictionary to a ModelSchema.

    Args:
        schema_dict: The dictionary to convert.

    Returns:
        The corresponding ModelSchema.
    """
    return ModelSchema(
        input_schema=[
            dict_to_schema_item(item) for item in schema_dict["input_schema"]
        ],
        feature_store_features_schema=[
            dict_to_schema_item(item)
            for item in schema_dict["feature_store_features_schema"]
        ],
        output_schema=[
            dict_to_schema_item(item) for item in schema_dict["output_schema"]
        ],
    )


def dict_to_schema_item(item: dict) -> ModelSchemaItem:
    """Convert a dictionary to a ModelSchemaItem.

    Args:
        item: The dictionary to convert.

    Returns:
        The corresponding ModelSchemaItem.
    """
    return ModelSchemaItem(
        name=item["name"],
        data_type=DataType[item["data_type"].upper()],
        shape=item.get("shape"),
    )
