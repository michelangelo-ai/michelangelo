from __future__ import annotations
import json
import logging
from michelangelo._internal.gateways.terrablob_gateway.common import (
    construct_terrablob_cmd,
    execute_terrablob_cmd_with_exception,
    TerrablobOptions,
    validate_kwargs,
)

_logger = logging.getLogger(__name__)


def list_terrablob_dir(
    directory: str,
    recursive: bool | None = False,
    output_relative_path: bool | None = False,
    limit: int | None = None,
    include_dir: bool | None = False,
    **kwargs,
) -> list[str]:
    """
    List the contents of a Terrablob directory.

    Args:
        directory: The Terrablob directory to list.
        recursive: Whether to list the contents of subdirectories.
        output_relative_path: Whether to output the relative path of each item.
        limit: The maximum number of items to list.
        include_dir: Whether to include directories in the list.
        timeout: The timeout for the command.
        source_entity: The source entity to use when listing the directory.
        is_staging: Whether the directory is in terrablob-staging
        auth_mode: The authentication mode for the command.
            e.g. auto, legacy, usso

    Returns:
        A list of paths in the Terrablob directory.
    """
    validate_kwargs(kwargs)

    options = TerrablobOptions(**kwargs)

    paths = []

    list_terrablob_dir_internal(
        directory,
        paths,
        root_directory=directory,
        options=options,
        recursive=recursive,
        output_relative_path=output_relative_path,
        limit=limit,
        include_dir=include_dir,
    )

    return paths


def list_terrablob_dir_internal(
    directory: str,
    paths: list[str],
    root_directory: str,
    options: TerrablobOptions,
    recursive: bool | None = False,
    output_relative_path: bool | None = True,
    limit: int | None = None,
    include_dir: bool | None = False,
):
    """
    List the contents of a Terrablob directory.
    This is an internal function. Use list_terrablob_dir instead.
    """
    cmd = ["tb-cli", "ls", directory, "--json"]

    if limit is not None:
        cmd.extend(["--limit", str(limit)])

    cmd = construct_terrablob_cmd(cmd, options)

    message = execute_terrablob_cmd_with_exception(
        cmd, f"Error listing Terrablob directory {directory}."
    )

    data = json.loads(message)

    result = data["result"]

    def construct_path(name: str):
        if not output_relative_path:
            return f"{directory}/{name}"

        if directory == root_directory:
            return name

        return f"{directory[len(root_directory) + 1 :]}/{name}"

    paths.extend(
        [
            construct_path(item["name"])
            for item in result
            if include_dir or item.get("type", None) == "blob"
        ],
    )

    if recursive:
        for item in result:
            if item.get("type", "blob") == "dir":
                list_terrablob_dir_internal(
                    f"{directory}/{item['name']}",
                    paths,
                    root_directory,
                    options,
                    recursive=recursive,
                    output_relative_path=output_relative_path,
                    limit=limit,
                    include_dir=include_dir,
                )
