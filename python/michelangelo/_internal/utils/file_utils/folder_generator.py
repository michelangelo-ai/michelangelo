import os
import re
import shutil
from pathlib import PurePath

FILE_PATTERN = "^file://.*"
DIR_PATTERN = "^dir://.*"


def generate_folder(
    folder_structure: dict,
    folder_path: str,
):
    """
    Create the folder given a Python dictionary that represents the folder structure
    If the file already exists under folder_path, it will not be overwritten

    Args:
        folder_structure: The folder structure, example:
            {
                "file1.txt": "abc",
                "file2.txt": "file://path/to/file2.txt",
                "sub_folder1": {
                    "file3.tar": "file://path/to/file3.tar",
                    "sub_sub_folder": {
                        "file4.txt": "xyz",
                    },
                },
                "sub_folder2": "dir://path/to/sub_folder2",
            }
        folder_path: The path of the generated folder

    Returns:
        None
    """
    if not os.path.exists(folder_path):
        os.makedirs(folder_path)

    for name in folder_structure:  # noqa: PLC0206
        content = folder_structure[name]
        sub_path = os.path.join(folder_path, name)
        if isinstance(content, dict):
            generate_folder(content, sub_path)
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
