import os
import shutil
from michelangelo.lib.model_manager.constants import StorageType


def download_assets(
    src: str,
    des: str,
    source_type: str,
):
    """
    Download the assets from source to destination

    Args:
        src: The path of the source
        des: The destination path to store the assets.
        source_type: The source type of the source path,
            either 'hdfs', 'terrablob' or 'local'
    """
    if source_type == StorageType.LOCAL and src != des:
        if os.path.isdir(src):
            shutil.copytree(src, des, dirs_exist_ok=True)
        else:
            shutil.copy(src, des)
