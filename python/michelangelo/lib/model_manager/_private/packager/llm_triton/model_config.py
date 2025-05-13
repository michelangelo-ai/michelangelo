import os
import tempfile
import logging
from typing import Optional
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager._private.packager.common import download_model
from michelangelo._internal.errors.terrablob_error import TerrablobFileNotFoundError
from michelangelo._internal.errors.hdfs_error import HDFSError

_logger = logging.getLogger(__name__)

CONFIG_FILE_NAME = "config.json"


def download_model_config(
    model_path: str,
    dest_file_path: Optional[str] = None,
    model_path_source_type: Optional[str] = StorageType.TERRABLOB,
) -> str:
    """
    Download the config.json file of HuggingFace model from the model path

    Args:
        model_path: The path of the model
        dest_file_path: The destination path to store the config.json file.
            If not specified, a temporary file will be created.

    Returns:
        The path of the downloaded config.json file.
        If the config file is not found, return None
    """
    dest_model_path = os.path.dirname(dest_file_path) if dest_file_path else tempfile.mkdtemp()

    try:
        download_model(
            model_path,
            dest_model_path,
            model_path_source_type=model_path_source_type,
            include=[CONFIG_FILE_NAME],
        )
    except (TerrablobFileNotFoundError, FileNotFoundError, HDFSError):
        _logger.warning(f"Config file {CONFIG_FILE_NAME} not found in model path {model_path}")
        return None

    if dest_file_path:
        return dest_file_path
    else:
        return os.path.join(dest_model_path, CONFIG_FILE_NAME)
