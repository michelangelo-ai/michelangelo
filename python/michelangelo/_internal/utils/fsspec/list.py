import fsspec
from typing import Optional
from pathlib import PurePath


def ls_files(path: str, recursive: Optional[bool] = False, output_relative_path: Optional[bool] = False) -> list[str]:
    """
    List the files in a directory.

    Args:
        path: The directory to list.
        recursive: Whether to list the contents of subdirectories.
        output_relative_path: Whether to output the relative path of each item.

    Returns:
        A list of paths in the directory
    """
    fs, p = fsspec.core.url_to_fs(path)

    output_paths = []
    ls_files_internal(fs, p, output_paths, p, recursive, output_relative_path)
    return output_paths


def ls_files_internal(
    fs: any,
    path: str,
    output_paths: list[str],
    root_directory: str,
    recursive: Optional[bool] = False,
    output_relative_path: Optional[bool] = False,
):
    """
    List the files in a directory.

    Args:
        fs: The filesystem object.
        path: The directory to list.
        output_paths: The list of output paths.
        root_directory: The root directory of the listing.
        recursive: Whether to list the contents of subdirectories.
        output_relative_path: Whether to output the relative path of each item.
    """
    results = fs.ls(path, detail=True)

    root_path = PurePath(root_directory)

    def construct_path(name: str):
        if not output_relative_path:
            return name

        p = PurePath(name).relative_to(root_path)
        return str(p)

    items = [item for item in results if item["name"] != root_directory]

    output_paths.extend([construct_path(item["name"]) for item in items if item.get("type") == "file"])

    if recursive:
        for item in items:
            if item.get("type") == "directory":
                ls_files_internal(fs, item["name"], output_paths, root_directory, recursive, output_relative_path)
