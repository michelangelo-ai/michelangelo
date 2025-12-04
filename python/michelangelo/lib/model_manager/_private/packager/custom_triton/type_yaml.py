"""Generate the type.yaml file content."""

from typing import Optional

import yaml

from michelangelo.lib.model_manager.constants import RawModelType


def generate_type_yaml(batch_inference: Optional[bool] = False) -> str:
    """Generate the type.yaml file content.

    Args:
        batch_inference: Whether to enable batch inference.

    Returns:
        The type.yaml file content
    """
    content = {
        "type": RawModelType.CUSTOM_PYTHON,
    }
    if batch_inference:
        content["batch_inference"] = batch_inference
    return yaml.dump(content, sort_keys=False)
