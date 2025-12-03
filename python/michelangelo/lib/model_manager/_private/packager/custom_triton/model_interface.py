import os
import shutil
import michelangelo.lib.model_manager.interface.custom_model as custom_model
from michelangelo.lib.shared.utils.reflection_utils import get_module_attr

module_path = os.path.join("michelangelo", "lib", "model_manager", "interface", "custom_model.py")


def serialize_model_interface(target_dir: str):
    """Serialize the model interface to the target dir.

    Args:
        target_dir: the target dir to serialize the model interface

    Returns:
        None
    """
    target_path = os.path.join(target_dir, module_path)

    if not os.path.exists(target_path):
        os.makedirs(os.path.dirname(target_path), exist_ok=True)
        shutil.copyfile(custom_model.__file__, target_path)


def validate_model_class(model_class: str) -> tuple[bool, Exception]:
    """Validate the model class.

    Args:
        model_class: the model class

    Returns:
        A tuple of a boolean indicating whether the model class is valid 
        and an exception if the model class is invalid
    """
    try:
        Model = get_module_attr(model_class)
    except (ValueError, ImportError) as e:
        return False, e

    if not issubclass(Model, custom_model.Model):
        return False, TypeError(
            f"Model class {model_class} must be a subclass of "
            "michelangelo.lib.model_manager.interface.custom_model.Model"
        )

    return True, None
