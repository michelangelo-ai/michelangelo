import os
import shutil
from pathlib import PurePath

from michelangelo.lib.model_manager._private.utils.reflection_utils import (
    get_root_import_path,
)


def save_module_files(files: list[str], target_dir: str):
    """Save the given module files to the target directory.

    Args:
        files (list[str]): The list of module files to save.
        target_dir (str): The target directory to save the module files.
    """
    for f in files:
        module_path = PurePath(f)
        root_import_path = PurePath(get_root_import_path(f))
        sub_path = module_path.relative_to(root_import_path)
        target_sub_dir = os.path.join(target_dir, sub_path.parent)
        target_path = os.path.join(target_dir, sub_path)
        if not os.path.exists(target_path):
            os.makedirs(target_sub_dir, exist_ok=True)
            shutil.copyfile(f, os.path.join(target_dir, sub_path))
