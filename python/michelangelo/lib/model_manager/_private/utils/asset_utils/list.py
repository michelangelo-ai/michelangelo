import os
import re
from pathlib import PurePath
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo._internal.gateways.terrablob_gateway import list_terrablob_dir
from michelangelo._internal.utils.fsspec_utils import ls_files
from michelangelo.lib.model_manager._private.utils.terrablob_utils import get_terrablob_auth_mode

URL_PATTERN = r"^\w+://.*"


def list_assets(directory: str, source_type: str) -> list[str]:
    """
    List the assets in the directory

    Args:
        directory: The directory path
        source_type: The source type of the directory, either 'hdfs', 'terrablob' or 'local'

    Returns:
        The list of paths relative to the directory
    """
    if source_type == StorageType.HDFS:
        # TODO: add StorageType.GCS and use similar logic for gcs storage
        hdfs_model_path = directory if re.match(URL_PATTERN, directory) else f"hdfs://{directory}"
        return ls_files(hdfs_model_path, recursive=True, output_relative_path=True)

    if source_type == StorageType.TERRABLOB:
        return list_terrablob_dir(
            directory,
            recursive=True,
            output_relative_path=True,
            source_entity="michelangelo-apiserver",
            auth_mode=get_terrablob_auth_mode(),
        )

    if source_type == StorageType.LOCAL:
        return [str(PurePath(os.path.join(dirpath, file)).relative_to(directory)) for dirpath, _, filenames in os.walk(directory) for file in filenames]
