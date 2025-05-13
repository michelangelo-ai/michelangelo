import os
import yaml
from typing import Optional
from uber.ai.michelangelo.sdk.model_manager.constants import PackageType


def infer_model_package_type(model_path: str) -> str:
    """
    Infer the package type given the model package dir

    Args:
        model_path: the path of the model in local

    Returns:
        The package type
    """
    if not os.path.exists(model_path):
        raise FileNotFoundError(f"{model_path} does not exists.")

    if not os.path.isdir(model_path):
        return PackageType.RAW

    if os.path.exists(
        os.path.join(model_path, "deploy_jar", "model.jar.gz"),
    ):
        return PackageType.SPARK

    if any(file == "config.pbtxt" for dirpath, _, filenames in os.walk(model_path) for file in filenames):
        return PackageType.TRITON

    return PackageType.RAW


def infer_raw_model_package_type(model_path: str) -> Optional[str]:
    """
    Infer the package type given the raw model package dir

    Args:
        model_path: the path of the model in local

    Returns:
        The raw model package type
    """
    if not os.path.exists(model_path) or not os.path.isdir(model_path):
        return None

    type_yaml_path = os.path.join(model_path, "metadata", "type.yaml")

    if not os.path.exists(type_yaml_path):
        return None

    with open(type_yaml_path) as f:
        content = yaml.safe_load(f)
        if content and "type" in content:
            return content["type"]
