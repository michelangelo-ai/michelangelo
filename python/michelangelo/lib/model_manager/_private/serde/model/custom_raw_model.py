"""The Custom Raw Model Loader."""

import os

from michelangelo.lib.model_manager._private.serde.loader.custom_model_loader import (
    load_custom_model,
)
from michelangelo.lib.model_manager._private.utils.loader_utils import (
    import_model_class,
)
from michelangelo.lib.model_manager.interface.custom_model import Model


def load_custom_raw_model(model_path: str) -> Model:
    """Load a custom raw model from the given model path.

    Args:
        model_path: The path to the model.

    Returns:
        The loaded custom raw model in the Model wrapper.
    """
    model_bin_path = os.path.join(model_path, "model")
    defs_path = os.path.join(model_path, "defs")

    if not os.path.exists(os.path.join(defs_path, "model_class.txt")):
        raise ValueError("Missing defs/model_class.txt in the model package.")

    with open(os.path.join(defs_path, "model_class.txt")) as f:
        model_class_str = f.read().strip()

    if not model_class_str:
        raise ValueError("defs/model_class.txt is empty in the model package.")

    model_class = import_model_class(defs_path, model_class_str)

    return load_custom_model(model_bin_path, model_class, defs_path)
