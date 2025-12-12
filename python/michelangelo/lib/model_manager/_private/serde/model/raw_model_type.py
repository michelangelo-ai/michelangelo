"""Raw model type."""

import os

import yaml

from michelangelo.lib.model_manager.constants import RawModelType


def get_raw_model_type(model_path: str) -> str:
    """Get the raw model type from the model package.

    Args:
        model_path: The model package path

    Returns:
        The raw model type
    """
    type_yaml_path = os.path.join(model_path, "metadata", "type.yaml")

    if not os.path.exists(type_yaml_path):
        raise FileNotFoundError("type.yaml file not found in the model package.")

    with open(type_yaml_path) as f:
        content = yaml.safe_load(f)
        model_type = content.get("type")

    if not model_type:
        raise ValueError("Model type is empty in the type.yaml file.")

    supported_raw_model_types = get_supported_raw_model_types()
    if model_type not in supported_raw_model_types:
        supported_types_str = ", ".join(supported_raw_model_types)
        raise ValueError(
            f"Invalid model type {model_type} in the type.yaml file. "
            f"Supported model types are {supported_types_str}."
        )

    return model_type


def get_supported_raw_model_types() -> set[str]:
    """Get the supported raw model types.

    Returns:
        The supported raw model types
    """
    return {
        getattr(RawModelType, attr)
        for attr in dir(RawModelType)
        if not attr.startswith("__")
    }
