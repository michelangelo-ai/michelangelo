import os
import sys
import logging
import importlib
from uber.ai.michelangelo.sdk.model_manager.interface.custom_model import Model
from uber.ai.michelangelo.sdk.model_manager._private.serde.loader.custom_model_loader import load_custom_model

_logger = logging.getLogger(__name__)


def load_custom_raw_model(model_path: str) -> Model:
    """
    Load a custom raw model from the given model path.

    Args:
        model_path: The path to the model.

    Returns:
        The loaded custom raw model in the Model wrapper.
    """
    model_bin_path = os.path.join(model_path, "model")
    defs_path = os.path.join(model_path, "defs")

    if not os.path.exists(os.path.join(defs_path, "model_class.txt")):
        raise ValueError("Missing defs/model_class.txt in the model package.")

    model_class = None
    with open(os.path.join(defs_path, "model_class.txt")) as f:
        model_class = f.read().strip()

    if not model_class:
        raise ValueError("defs/model_class.txt is empty in the model package.")

    module_def, _, class_name = model_class.rpartition(".")

    if not module_def or not class_name:
        raise ValueError(f"Invalid model class definition {model_class}. Please specify the full import path to the model class.")

    try:
        module = importlib.import_module(module_def)
    except (ImportError, ModuleNotFoundError):
        _logger.info(f"Module {module_def} not found in the system path. Trying to load from the model package.")
        sys.path.append(os.path.abspath(defs_path))
        module = importlib.import_module(module_def)

    try:
        ModelClass = getattr(module, class_name)
    except AttributeError as err:
        raise AttributeError(f"Class {class_name} not found in module {module_def}.") from err

    return load_custom_model(model_bin_path, ModelClass, defs_path)
