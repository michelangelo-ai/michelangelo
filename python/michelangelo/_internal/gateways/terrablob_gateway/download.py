import logging
import os
from typing import Optional
from concurrent.futures import ThreadPoolExecutor, as_completed
from michelangelo._internal.gateways.terrablob_gateway.common import (
    construct_terrablob_cmd,
    execute_terrablob_cmd_with_exception,
    TerrablobOptions,
    validate_kwargs,
)
from michelangelo._internal.gateways.terrablob_gateway.is_dir import path_is_dir
from michelangelo._internal.gateways.terrablob_gateway.list import list_terrablob_dir

_logger = logging.getLogger(__name__)

DEFAULT_NUM_THREADS = 8


def download_from_terrablob(
    src_path: str,
    des_path: str,
    use_threads: Optional[bool] = True,
    num_threads: Optional[int] = DEFAULT_NUM_THREADS,
    multipart: Optional[bool] = False,
    **kwargs,
) -> dict:
    """
    Download from Terrablob.

    Args:
        src_path: The path to the file/directory to download.
        des_path: The destination path in local. If src_path is a directory,
            des_path should designate a directory.
        use_threads: Whether to use multi-threads for downloading.
        num_threads: The number of threads to use for downloading.
            If use_threads is False, this argument has no effect.
        multipart: Enables the multipart download for a blob larger than 128MiB. A large blob will be downloaded in chunks of 128MiB.
        timeout: The timeout for the download.
        source_entity: The source entity for the download.
        is_staging: Whether the download is from terrablob-staging.
        auth_mode: The authentication mode for the command.

    Returns:
        A dictionary containing the exit code, message, error of the command.
    """
    _logger.info("Downloading from Terrablob")

    validate_kwargs(kwargs)

    options = TerrablobOptions(**kwargs)

    if path_is_dir(src_path, **kwargs):

        def download_file(file_path: str) -> dict:
            rel_path = os.path.relpath(file_path, src_path)
            target = os.path.join(des_path, rel_path)
            parent = os.path.dirname(target)
            if parent:
                os.makedirs(parent, exist_ok=True)
            return download_file_from_terrablob(file_path, target, options, multipart)

        paths = list_terrablob_dir(src_path, recursive=True, **kwargs)

        if use_threads:
            num_threads = (
                max(num_threads, 1) if num_threads is not None else DEFAULT_NUM_THREADS
            )
            with ThreadPoolExecutor(max_workers=num_threads) as executor:
                futures = [executor.submit(download_file, path) for path in paths]
                for future in as_completed(futures):
                    future.result()
        else:
            for path in paths:
                download_file(path)

        return {
            "exitcode": 0,
            "message": f"Downloaded from Terrablob {src_path} {'(staging) ' if options.is_staging else ''}to {des_path}. File count: {len(paths)}.",
            "error": "",
        }

    # if src_path is not a dir
    parent = os.path.dirname(des_path)
    if parent:
        os.makedirs(parent, exist_ok=True)

    result = download_file_from_terrablob(src_path, des_path, options, multipart)

    return result


def download_file_from_terrablob(
    src_path: str,
    des_path: str,
    options: TerrablobOptions,
    multipart: Optional[bool] = False,
) -> dict:
    """
    Download one file from Terrablob.
    This is an internal function. Use download_from_terrablob instead.

    Args:
        src_path: The path to the file to download.
        des_path: The destination path in local.
        options: additional options for the command.
        multipart: Enables the multipart download for a blob larger than 128MiB. A large blob will be downloaded in chunks of 128MiB.

    Returns:
        A dictionary containing the exit code, message, error of the command.
    """
    _logger.info("Downloading file from Terrablob")

    result = {}

    cmd = [
        "tb-cli",
        "get",
        src_path,
        des_path,
    ]

    if multipart:
        cmd.append("-m")

    cmd = construct_terrablob_cmd(cmd, options)

    _logger.info(f"Downloading {src_path} to Terrablob {des_path}.")

    message = execute_terrablob_cmd_with_exception(
        cmd, f"Error downloading from Terrablob {src_path} to {des_path}."
    )

    stats = os.stat(des_path)

    result = {
        "exitcode": 0,
        "message": message,
        "error": "",
    }

    _logger.info(
        f"download_from_terrablob result: {result}. File size: {stats.st_size} bytes."
    )

    return result
