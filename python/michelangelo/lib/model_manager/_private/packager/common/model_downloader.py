import os
import tempfile
from typing import Optional
from uber.ai.michelangelo.sdk.model_manager.constants import StorageType
from uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils import download_assets


def download_model(
    model_path: str,
    dest_model_path: Optional[str] = None,
    model_path_source_type: Optional[str] = StorageType.HDFS,
    include: Optional[list[str]] = None,
) -> str:
    """
    Download the raw model from source to destination

    Args:
        model_path: The path of the model
        dest_model_path: The destination path to store the model.
            If not specified, a temporary directory will be created.
        model_path_source_type: The source type of the model path,
            either 'hdfs', 'terrablob' or 'local'
        include: The list of files/directories to include in the download
            If not specified, download all files in the model path

    Returns:
        The destination path of the model
    """
    if not model_path_source_type:
        return None

    if not dest_model_path:
        dest_model_path = tempfile.mkdtemp()

    if include:
        for sub_path in include:
            download_assets(
                f"{model_path}/{sub_path}",
                os.path.join(dest_model_path, sub_path),
                model_path_source_type,
                timeout="2h",
                source_entity="michelangelo-apiserver",
            )
    else:
        download_assets(
            model_path,
            dest_model_path,
            model_path_source_type,
            timeout="2h",
            source_entity="michelangelo-apiserver",
        )

    return dest_model_path
