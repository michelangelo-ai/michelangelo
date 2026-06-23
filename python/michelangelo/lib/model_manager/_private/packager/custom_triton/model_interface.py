"""Validate and serialize the model interface."""

import michelangelo.lib.model_manager.interface.custom_model as custom_model
from michelangelo._internal.utils.reflection_utils import get_module_attr
from michelangelo.lib.model_manager._private.packager.common.serialization import (
    serialize_model_interface,
)

__all__ = ["serialize_model_interface", "validate_model_class"]


def validate_model_class(model_class: str) -> tuple[bool, Exception]:
    """Validate the model class.

    Args:
        model_class: the model class

    Returns:
        A tuple of a boolean indicating whether the model class is valid
        and an exception if the model class is invalid
    """
    try:
        model_cls = get_module_attr(model_class)
    except (ValueError, ImportError) as e:
        return False, e

    if not issubclass(model_cls, custom_model.Model):
        return False, TypeError(
            f"Model class {model_class} must be a subclass of "
            "michelangelo.lib.model_manager.interface.custom_model.Model"
        )

    return True, None
