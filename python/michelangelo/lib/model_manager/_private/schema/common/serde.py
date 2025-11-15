import yaml
from dataclasses import asdict, fields
from michelangelo.lib.model_manager.schema import ModelSchema, ModelSchemaItem, DataType


def schema_to_yaml(schema: ModelSchema) -> str:
    schema_dict = schema_to_dict(schema)
    return yaml.dump(schema_dict, sort_keys=False)


def schema_to_dict(schema: ModelSchema) -> dict:
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
    return ModelSchema(
        input_schema=[dict_to_schema_item(item) for item in schema_dict["input_schema"]],
        feature_store_features_schema=[dict_to_schema_item(item) for item in schema_dict["feature_store_features_schema"]],
        output_schema=[dict_to_schema_item(item) for item in schema_dict["output_schema"]],
    )


def dict_to_schema_item(item: dict) -> ModelSchemaItem:
    return ModelSchemaItem(
        name=item["name"],
        data_type=DataType[item["data_type"].upper()],
        shape=item["shape"] if "shape" in item else None,
    )
