import os
from typing import Optional
from michelangelo.lib.model_manager._private.utils.model_utils import download_assets_given_download_yaml


def retrieve_model_assets(model_package: str, timeout: Optional[str] = None, source_entity: Optional[str] = None):
    """
    Retrieve the model assets from the model package
    Download the assets referenced in the download.yaml files

    Args:
        model_package: The path to the model package
        timeout: The timeout for terrablob command. Defaults to None.
        source_entity: The source entity for terrablob command. Defaults to None.
    """
    for dirpath, _, filenames in os.walk(model_package):
        for filename in filenames:
            if filename == "download.yaml":
                download_yaml_path = os.path.join(dirpath, filename)
                download_assets_given_download_yaml(download_yaml_path, dirpath, timeout=timeout, source_entity=source_entity)
                os.remove(download_yaml_path)
