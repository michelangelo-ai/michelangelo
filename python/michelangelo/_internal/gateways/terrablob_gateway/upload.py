import os
import logging
from typing import Optional
from concurrent.futures import ThreadPoolExecutor, as_completed
from michelangelo._internal.gateways.terrablob_gateway.common import (
    construct_terrablob_cmd,
    execute_terrablob_cmd_with_exception,
    TerrablobOptions,
    validate_kwargs,
)

_logger = logging.getLogger(__name__)

DEFAULT_NUM_THREADS = 8


def upload_to_terrablob(
    src_path: str,
    des_path: str,
    use_kraken: Optional[bool] = False,
    use_threads: Optional[bool] = True,
    num_threads: Optional[int] = DEFAULT_NUM_THREADS,
    multipart: Optional[bool] = False,
    concurrency: Optional[int] = None,
    **kwargs,
) -> dict:
    """
    Upload to Terrablob.

    Args:
        src_path: The path to the file/directory to upload.
        des_path: The destination path in Terrablob. If src_path is a directory,
            des_path should designate a directory.
        use_kraken: Whether to use Kraken to upload the file.
        use_threads: Whether to use multi-threads for uploading.
        num_threads: The number of threads to use for uploading.
            If use_threads is False, this argument has no effect.
        multipart: Enables the multipart upload. max part size will be 128MiB. This makes the max possible blob size to be 1250GiB
        concurrency: The number of concurrent workers doing the transfer (default 2)
        timeout: The timeout for the upload.
        source_entity: The source entity for the upload.
        is_staging: Whether the upload is to terrablob-staging.
        auth_mode: The authentication mode for the command.
            e.g. auto, legacy, usso

    Returns:
        A dictionary containing the exit code, message, error of the command.
    """
    _logger.info("Uploading to Terrablob")

    validate_kwargs(kwargs)

    options = TerrablobOptions(**kwargs)

    if not os.path.exists(src_path):
        raise FileNotFoundError(f"Path {src_path} does not exist.")

    if os.path.isfile(src_path):
        return upload_file_to_terrablob(
            src_path,
            des_path,
            options,
            use_kraken=use_kraken,
            multipart=multipart,
            concurrency=concurrency,
        )

    def upload_file(file_path: str) -> dict:
        rel_path = os.path.relpath(file_path, src_path).replace("\\", "/")
        return upload_file_to_terrablob(file_path, f"{des_path}/{rel_path}", options, use_kraken=use_kraken, multipart=multipart, concurrency=concurrency)

    files = list_files(src_path)

    if use_threads:
        num_threads = max(num_threads, 1) if num_threads is not None else DEFAULT_NUM_THREADS
        with ThreadPoolExecutor(max_workers=num_threads) as executor:
            futures = [executor.submit(upload_file, file) for file in files]
            for future in as_completed(futures):
                future.result()
    else:
        for file in files:
            upload_file(file)

    return {
        "exitcode": 0,
        "message": f"Uploaded directory {src_path} to Terrablob {des_path}. File count: {len(files)}.",
        "error": "",
    }


def upload_file_to_terrablob(
    src_path: str,
    des_path: str,
    options: TerrablobOptions,
    use_kraken: Optional[bool] = False,
    multipart: Optional[bool] = False,
    concurrency: Optional[int] = None,
) -> dict:
    """
    Upload one file to Terrablob.
    This is an internal function. Use upload_to_terrablob instead.

    Args:
        src_path: The path to the file to upload.
        des_path: The destination path in Terrablob.
        options: additional options for the command.
        use_kraken: Whether to use Kraken to upload the file.
        multipart: Enables the multipart upload. max part size will be 128MiB. This makes the max possible blob size to be 1250GiB
        concurrency: The number of concurrent workers doing the transfer (default 2)

    Returns:
        A dictionary containing the exit code, message, error of the command.
    """
    _logger.info("Uploading file to Terrablob")

    cmd = [
        "tb-cli",
        "put",
        src_path,
        des_path,
        "-p",
    ]

    if use_kraken:
        cmd.append("--kraken")

    if multipart:
        cmd.append("-m")

    if concurrency is not None:
        cmd.extend(["-C", str(concurrency)])

    cmd = construct_terrablob_cmd(cmd, options)

    stats = os.stat(src_path)

    _logger.info(f"Uploading {src_path} to Terrablob {des_path}. File size: {stats.st_size} bytes.")

    message = execute_terrablob_cmd_with_exception(cmd, f"Error uploading {src_path} to Terrablob {des_path}.")

    result = {
        "exitcode": 0,
        "message": message,
        "error": "",
    }

    _logger.info(f"upload_to_terrablob result: {result}")

    return result


def list_files(directory):
    """
    List all files recursively in a directory.

    Args:
        directory: The directory to list.

    Returns:
        A list of paths to all files in the directory.
    """
    return [os.path.join(dirpath, file) for dirpath, _, filenames in os.walk(directory) for file in filenames]
