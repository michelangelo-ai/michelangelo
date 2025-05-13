from dataclasses import dataclass, field
from typing import Optional


@dataclass
class TerrablobOptions:
    """
    Common options for all Terrablob commands.
    """

    timeout: Optional[str] = field(default=None, metadata={"description": "The timeout for the command."})
    keepalive: Optional[bool] = field(
        default=False, metadata={"description": "When keepalive flag is set, timeout is applied to blob parts. Default blob part timeout is 30s."}
    )
    source_entity: Optional[str] = field(default=None, metadata={"description": "The source entity for when executing the command."})
    is_staging: Optional[bool] = field(default=False, metadata={"description": "Whether the command is used on terrablob-staging."})
    auth_mode: Optional[str] = field(default=None, metadata={"description": "The authentication mode for the command, e.g., auto, legacy, usso."})


def validate_kwargs(kwargs: dict) -> None:
    """
    Validate the kwargs for Terrablob commands.

    Args:
        kwargs: The kwargs to validate.
    """
    for key in kwargs:
        if key not in TerrablobOptions.__annotations__:
            raise TypeError(f"Invalid keyword argument {key}. Do you mean one of {', '.join(TerrablobOptions.__annotations__.keys())}?")
