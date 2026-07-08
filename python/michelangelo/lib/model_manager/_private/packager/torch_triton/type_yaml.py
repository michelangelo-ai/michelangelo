"""Generate the type.yaml file content for the torch raw model package."""

from __future__ import annotations

import yaml

from michelangelo.lib.model_manager.constants import RawModelType


def generate_type_yaml() -> str:
    """Generate the type.yaml file content for a torch raw model package.

    Returns:
        The type.yaml file content.
    """
    return yaml.dump({"type": RawModelType.TORCH}, sort_keys=False)
