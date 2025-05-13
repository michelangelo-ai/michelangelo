from michelangelo._internal.gateways.terrablob_gateway.common import validate_kwargs
from michelangelo._internal.gateways.terrablob_gateway.list import list_terrablob_dir
from michelangelo._internal.errors.terrablob_error import (
    TerrablobFailedPreconditionError,
)


def path_is_dir(path: str, **kwargs) -> bool:
    """
    Check if a path is a directory in Terrablob.
    Note, if the path is a directory, it implies that the path exists.

    Args:
        path: The Terrablob path to check.
        timeout: The timeout for the command.
        source_entity: The source entity to use when checking the path.
        is_staging: Whether the path is in terrablob-staging
        auth_mode: The authentication mode for the command.
            e.g. auto, legacy, usso

    Returns:
        bool: True if the path is a directory, False otherwise.
    """
    validate_kwargs(kwargs)

    try:
        list_terrablob_dir(path, limit=1, **kwargs)
    except TerrablobFailedPreconditionError:
        return False
    else:
        return True
