import torch
from typing import Union
from michelangelo.lib.model_manager.interface.custom_model import Model
from michelangelo.lib.model_manager.constants import RawModelType
from michelangelo.lib.model_manager._private.serde.model import (
    get_raw_model_type,
    load_custom_raw_model,
)


def load_raw_model(model_path: str) -> Union[Model, torch.nn.Module]:
    """
    Load the raw model from the model package

    Args:
        model_path: The model package path

    Returns:
        The raw model
        For custom python model, it returns the custom Model instance
        For torch model, it returns the PyTorch nn.Module instance
    """
    raw_model_type = get_raw_model_type(model_path)

    if raw_model_type == RawModelType.CUSTOM_PYTHON:
        return load_custom_raw_model(model_path)

    raise NotImplementedError(
        f"The loader for {raw_model_type} model is not supported yet. Please check back in future updates."
    )
