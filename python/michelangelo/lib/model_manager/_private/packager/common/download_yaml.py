from typing import Optional
from uber.ai.michelangelo.sdk.model_manager.constants import StorageType
from uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils import list_assets


def generate_download_yaml_content(
    model_path: str,
    model_path_source_type: Optional[str] = StorageType.TERRABLOB,
    target_prefix: Optional[str] = None,
    source_prefix: Optional[str] = None,
    output_source_type: Optional[str] = None,
) -> dict:
    """
    Generate the download.yaml file content

    Args:
        model_path: The model path in terrablob
        model_path_source_type: The source type of the model path, e.g. 'hdfs', 'terrablob'
        target_prefix: The prefix to add to the target path
        source_prefix: The prefix to add to the source path

    Returns:
        The download.yaml file content
    """
    assets = list_assets(model_path, model_path_source_type)

    source_prefix = source_prefix or model_path + "/"

    return {
        "assets": [
            {
                f"{target_prefix or ''}{file_path}": f"{source_prefix}{file_path}",
            }
            for file_path in assets
        ],
        "source_type": output_source_type or model_path_source_type or StorageType.TERRABLOB,
        "source_prefix": source_prefix,
    }
