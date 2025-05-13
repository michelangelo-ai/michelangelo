import os
import shutil
from typing import Optional
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo._internal.gateways.hdfs_gateway import download_from_hdfs
from michelangelo._internal.gateways.terrablob_gateway import download_from_terrablob
from michelangelo.lib.model_manager._private.utils.terrablob_utils import get_terrablob_auth_mode


def download_assets(
    src: str,
    des: str,
    source_type: str,
    timeout: Optional[str] = None,
    source_entity: Optional[str] = None,
):
    """
    Download the assets from source to destination

    Args:
        src: The path of the source
        des: The destination path to store the assets.
        source_type: The source type of the source path,
            either 'hdfs', 'terrablob' or 'local'
        timeout: The timeout for terrablob command. Defaults to None.
        source_entity: The source entity for terrablob command. Defaults to None.
    """
    if source_type == StorageType.HDFS:
        download_from_hdfs(src, des)  # TODO: use fsspec
    elif source_type == StorageType.TERRABLOB:
        download_from_terrablob(src, des, source_entity=source_entity, auth_mode=get_terrablob_auth_mode(), timeout=timeout)
    elif source_type == StorageType.LOCAL and src != des:
        if os.path.isdir(src):
            shutil.copytree(src, des, dirs_exist_ok=True)
        else:
            shutil.copy(src, des)
