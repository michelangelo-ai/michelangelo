import yaml
from dataclasses import asdict, fields
from uber.ai.michelangelo.sdk.model_manager.schema import ModelSchema


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
