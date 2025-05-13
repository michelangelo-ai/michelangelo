import uuid
import yaml
from michelangelo.lib.model_manager.schema import (
    ModelSchema,
    ModelSchemaItem,
    DataType,
)
from michelangelo.lib.model_manager.constants import ModelKind


def generate_trained_model_yaml(
    model_schema: ModelSchema,
    project_name: str,
    model_desc: str,
    model_kind: str,
) -> str:
    """
    Generate the trained_model.yaml file content

    Args:
        model_schema: The model schema.
        project_name: The name of the project in MA Studio
        model_desc: The description of the model
        model_kind: The model kind

    Returns:
        The trained_model.yaml file content
    """
    schema_items = [
        *model_schema.input_schema,
        *model_schema.feature_store_features_schema,
    ]

    features = [
        {
            "feature": item.name,
            "transformation": get_transformations(item),
        }
        for item in schema_items
    ]

    model_kind = (
        f"canvas_{model_kind}"
        if model_kind
        in {
            ModelKind.CUSTOM,
            ModelKind.REGRESSION,
            ModelKind.BINARY_CLASSIFICATION,
            ModelKind.MULTICLASS_CLASSIFICATION,
        }
        else model_kind
    )

    content = {
        "training_job_id": str(uuid.uuid4()),
        "use_new": True,
        "project_id": project_name,
        "description": model_desc,
        "model": {
            "is_tm_model": True,
            "type": model_kind,
            "artifact_version": 2,
        },
        "published_fields": [],
        "features": {"derived": features},
        "response_variable": [
            {
                "feature": "prediction",
                "transformation": "sVal(preidction)",
            },
        ],
        "training_data": {
            "start_date": "",
            "end_date": "",
        },
    }

    return yaml.dump(content, sort_keys=False)


def get_transformations(schema_item: ModelSchemaItem):
    if schema_item.data_type == DataType.STRING:
        return f"sVal({schema_item.name})"

    return f"nVal({schema_item.name})"
