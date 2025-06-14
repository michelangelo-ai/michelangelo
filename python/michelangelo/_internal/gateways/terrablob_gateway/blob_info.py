import logging
import json
from michelangelo._internal.gateways.terrablob_gateway.common import (
    construct_terrablob_cmd,
    execute_terrablob_cmd_with_exception,
    TerrablobOptions,
    validate_kwargs,
)

_logger = logging.getLogger(__name__)


def get_blob_info(blob_path: str, **kwargs) -> dict:
    """
    Get blob info from Terrablob.

    Args:
        blob_path: The path to the blob.
        timeout: The timeout for the command.
        source_entity: The source entity for the command.
        is_staging: Whether the blob is in terrablob-staging.
        auth_mode: The authentication mode for the command.
            e.g. auto, legacy, usso

    Returns:
        A dictionary containing the blob info.
    """
    _logger.info("Getting blob info from Terrablob")

    validate_kwargs(kwargs)

    options = TerrablobOptions(**kwargs)

    cmd = [
        "tb-cli",
        "blobInfo",
        blob_path,
        "--json",
    ]

    cmd = construct_terrablob_cmd(cmd, options)

    _logger.info(f"Getting blob info for {blob_path}.")

    message = execute_terrablob_cmd_with_exception(
        cmd, f"Error getting blob info for {blob_path}."
    )

    data = json.loads(message)

    return data.get("result", {})
