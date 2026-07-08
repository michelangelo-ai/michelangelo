"""Generate the requirements.txt file content."""

from michelangelo.lib.model_manager._private.packager.common.serialization import (
    generate_requirements_txt,
)

__all__ = ["generate_requirements_txt"]
