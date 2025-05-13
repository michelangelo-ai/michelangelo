import yaml
from typing import Optional
from michelangelo.lib.model_manager._private.constants import RawModelType


def generate_type_yaml(batch_inference: Optional[bool] = False) -> str:
    """
    Generate the type.yaml file content

    Returns:
        The type.yaml file content
    """
    content = {
        "type": RawModelType.CUSTOM_PYTHON,
    }
    if batch_inference:
        content["batch_inference"] = batch_inference
    return yaml.dump(content, sort_keys=False)
