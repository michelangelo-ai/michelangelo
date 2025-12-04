import os
import shutil
from pathlib import PurePath


def save_module_files(files: dict[str, str], target_dir: str):
    """Save the given module files to the target directory.

    Args:
        files: A dictionary whose keys are the module names
            and values are the module file paths
        target_dir (str): The target directory to save the module files.
    """
    for m, f in files.items():
        module_path = PurePath(f"{m.replace('.', os.sep)}.py")
        target_sub_dir = os.path.join(target_dir, module_path.parent)
        target_path = os.path.join(target_dir, module_path)
        if not os.path.exists(target_path):
            os.makedirs(target_sub_dir, exist_ok=True)
            shutil.copyfile(f, os.path.join(target_dir, module_path))
