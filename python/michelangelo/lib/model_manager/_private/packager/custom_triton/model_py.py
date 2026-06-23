"""Generate model.py file content for Triton deployment."""

from michelangelo.lib.model_manager._private.packager.common.serialization import (
    generate_model_py_content,
)

__all__ = ["generate_model_py_content"]
