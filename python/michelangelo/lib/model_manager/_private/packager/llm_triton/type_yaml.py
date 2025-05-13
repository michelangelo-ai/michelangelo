import yaml
from michelangelo.lib.model_manager._private.constants import RawModelType


def generate_type_yaml() -> str:
    """
    Generate the type.yaml file content

    Returns:
        The type.yaml file content
    """
    content = {
        "type": RawModelType.HUGGINGFACE,
    }
    return yaml.dump(content, sort_keys=False)
