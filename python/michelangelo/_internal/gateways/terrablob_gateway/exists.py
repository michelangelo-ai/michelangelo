from uber.ai.michelangelo.shared.gateways.terrablob_gateway.common import validate_kwargs
from uber.ai.michelangelo.shared.gateways.terrablob_gateway.blob_info import get_blob_info
from uber.ai.michelangelo.shared.gateways.terrablob_gateway.list import list_terrablob_dir
from uber.ai.michelangelo.shared.errors.terrablob_error import (
    TerrablobFileNotFoundError,
    TerrablobFailedPreconditionError,
)


def path_exists(path: str, **kwargs) -> bool:
    """
    Check if a blob or directory exists in Terrablob.

    Args:
        path: The path to check.
        timeout: The timeout for the command.
        source_entity: The source entity to use when checking the path.
        is_staging: Whether the path is in terrablob-staging.
        auth_mode: The authentication mode for the command.
            e.g. auto, legacy, usso

    Returns:
        bool: True if the path exists, False otherwise.
    """
    validate_kwargs(kwargs)

    try:
        get_blob_info(path, **kwargs)
    except TerrablobFailedPreconditionError:
        try:
            list_terrablob_dir(path, limit=1, **kwargs)
        except TerrablobFileNotFoundError:
            return False
        else:
            return True
    except TerrablobFileNotFoundError:
        return False
    else:
        return True
