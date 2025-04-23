import importlib
import inspect
import logging

RESHAPE_LAYER_FILE = "reshape_layer.bin"
REDUCTION_SCRIP_MODEL_FILE = "reduction_script_model.pt"

logger = logging.getLogger(__name__)


def get_class(full_obj_name: str) -> type:
    """
    Return the type of obj from name represented as string

    :param full_obj_name: The fully qualified name of obj
    :type full_obj_name: str
    :return: A type that can be instantiated
    :rtype: type
    """
    if inspect.isclass(full_obj_name):
        # if full_obj_name is already a class, return directly
        logger.warning(f"reflection::get_class full_obj_name is a class, return as is. {full_obj_name}")
        return full_obj_name

    groups = full_obj_name.split(".")

    assert len(groups) > 1, "missing module director"
    module = ".".join(groups[:-1])
    module_name = groups[-1]
    mod = importlib.import_module(module)
    return getattr(mod, module_name)
