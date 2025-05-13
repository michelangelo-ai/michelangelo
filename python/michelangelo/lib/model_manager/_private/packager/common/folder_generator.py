import os
import re
import shutil
from pathlib import PurePath

FILE_PATTERN = "^file://.*"
DIR_PATTERN = "^dir://.*"


def generate_model_package_folder(
    model_package_content: dict,
    folder_path: str,
):
    """
    Create the model package folder given the model package content
    If the file already exists under folder_path, it will not be overwritten

    Args:
        model_package_content: The model package content
        folder_path: The path of the model package folder

    Returns:
        None
    """
    if not os.path.exists(folder_path):
        os.makedirs(folder_path)

    for name in model_package_content:  # noqa: PLC0206
        content = model_package_content[name]
        sub_path = os.path.join(folder_path, name)
        if isinstance(content, dict):
            generate_model_package_folder(content, sub_path)
        elif re.match(FILE_PATTERN, content) and not os.path.exists(sub_path):
            shutil.copy(content.replace("file://", ""), sub_path)
        elif re.match(DIR_PATTERN, content):
            target_path = folder_path if name == "." else sub_path

            shutil.copytree(
                content.replace("dir://", ""),
                target_path,
                ignore=make_ignore_files(target_path),
                dirs_exist_ok=True,
            )
        elif not os.path.exists(sub_path):
            with open(sub_path, "w+") as file:
                file.write(content)


def make_ignore_files(target_path: str):
    def ignore_files(directory: str, files: list[str]) -> list[str]:
        """
        ignore the files that already exist in the target path

        Args:
            directory: The directory in the current iteration of the subtree
            files: The files in the directory

        Returns:
            The files that already exist in the target path
        """
        target_dir = PurePath(target_path)
        current_dir = PurePath(directory)

        if target_dir not in current_dir.parents and current_dir != target_dir:
            return []

        sub_dir = current_dir.relative_to(target_dir)

        return [f for f in files if os.path.exists(os.path.join(target_path, sub_dir, f)) and os.path.isfile(os.path.join(target_path, sub_dir, f))]

    return ignore_files
