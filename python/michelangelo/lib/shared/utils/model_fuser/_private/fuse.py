"""Private helpers for :mod:`michelangelo.lib.shared.utils.model_fuser.fuse`.

Module loading/forward-signature helpers that back the public fusing API.
ONNX export helpers have moved to
:mod:`michelangelo.lib.model_manager._private.utils.onnx_utils`, shared with
the non-fused Triton packager. Nothing here is part of the public interface
— import from ``fuse`` instead.
"""

from __future__ import annotations

import inspect
import os
from typing import Any

import torch

from michelangelo.lib.model_manager.schema import DataType, ModelSchema
from michelangelo.lib.model_manager.utils.torch.data_type import (
    data_type_to_torch_dtype,
)
from michelangelo.uniflow.core.utils import import_attribute

from ..fuse_schema import fuse_input_schema
from ..fused_model import FusedModel

# ---------------------------------------------------------------------------
# Module loading / forward-signature helpers
# ---------------------------------------------------------------------------


def _forward_accepts_dict(module: torch.nn.Module) -> bool:
    """Return whether ``module.forward``'s first parameter is dict-annotated."""
    try:
        sig = inspect.signature(module.forward)
    except (ValueError, TypeError):
        return False
    for name, param in sig.parameters.items():
        if name == "self":
            continue
        if param.annotation is inspect.Parameter.empty:
            return False
        return "dict" in str(param.annotation).lower()
    return False


def _forward_param_order(module: torch.nn.Module) -> list[str]:
    """Return ``module.forward``'s parameter names (excluding ``self``), in order."""
    try:
        sig = inspect.signature(module.forward)
    except (ValueError, TypeError):
        return []
    return [name for name in sig.parameters if name != "self"]


def _schema_input_keys(schema: ModelSchema | None) -> list[str]:
    """Return input feature names in schema order, or ``[]`` if unset."""
    if schema is None:
        return []
    return [item.name for item in schema.input_schema]


def _schema_output_keys(schema: ModelSchema | None) -> list[str]:
    """Return output feature names in schema order, or ``[]`` if unset."""
    if schema is None:
        return []
    return [item.name for item in schema.output_schema]


def _build_fused_sample_input(
    tx_model_schema: ModelSchema | None,
    model_schema: ModelSchema | None,
    batch_size: int = 1,
) -> dict[str, torch.Tensor]:
    """Build a sample input dict for the fused model, for tracing/inference.

    Uses the same input feature set as :func:`fuse_input_schema`. Each tensor
    has shape ``[batch_size, *feature_shape]`` and dtype derived from the
    item's ``data_type``. Tensors are placed on CUDA when available, else CPU
    (matching the device the fused module is traced on).

    Args:
        tx_model_schema: Native-transform model schema, or ``None``.
        model_schema: Predictor model schema, or ``None``.
        batch_size: Batch dimension for the sample tensors.

    Returns:
        Mapping of fused input feature name to a zero-filled sample tensor.
        Empty if the fused input schema is empty.
    """
    input_items = fuse_input_schema(tx_model_schema, model_schema)
    if not input_items:
        return {}
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    sample: dict[str, torch.Tensor] = {}
    for item in input_items:
        feature_shape = list(item.shape) if item.shape else [1]
        data_type = item.data_type if item.data_type is not None else DataType.UNKNOWN
        shape = [batch_size] + [max(1, int(s)) for s in feature_shape]
        dtype = data_type_to_torch_dtype(data_type)
        sample[item.name] = torch.zeros(shape, dtype=dtype, device=device)
    return sample


def _is_state_dict(obj: Any) -> bool:
    """Return whether ``obj`` is a state_dict (a dict of name -> Tensor)."""
    return isinstance(obj, dict) and all(
        isinstance(v, torch.Tensor) for v in obj.values()
    )


def _load_module_from_path(
    path: str,
    model_class: str,
    hyperparameters: dict[str, Any],
) -> torch.nn.Module:
    """Load an ``nn.Module`` from a local file (state_dict or full module).

    Args:
        path: Local path to a ``.pt``/``.pth`` file.
        model_class: Dotted class name to instantiate when the file contains
            a state_dict.
        hyperparameters: Constructor kwargs for ``model_class``.

    Returns:
        The loaded module in eval mode.

    Raises:
        FileNotFoundError: If ``path`` does not exist.
        TypeError: If the file contains neither a state_dict nor an
            ``nn.Module``.
    """
    if not os.path.isfile(path):
        raise FileNotFoundError(f"Model file not found: {path}")

    # weights_only=False: the file may hold a full nn.Module (not just a
    # state_dict), which torch's restricted unpickler can't reconstruct.
    # Safe only because model artifacts here come from a trusted pipeline
    # (this package's own storage backend), never directly from an
    # unauthenticated end user.
    loaded = torch.load(path, map_location="cpu", weights_only=False)

    if _is_state_dict(loaded):
        model_cls = import_attribute(model_class)
        model = model_cls(**(hyperparameters or {}))
        model.load_state_dict(loaded)
    else:
        model = loaded

    if isinstance(model, torch.nn.Module):
        model.eval()
        return model
    raise TypeError(f"File {path} did not contain a state_dict or nn.Module")


def _align_predictor_input_keys(
    pred_module: torch.nn.Module,
    predictor_input_keys: list[str],
    predictor_takes_dict: bool,
) -> list[str]:
    """Reorder predictor input keys to match ``forward()``'s parameter order.

    For dict-accepting predictors, order does not matter and the keys are
    returned unchanged. For positional predictors, schema keys are aligned to
    the ``forward()`` signature order.

    Args:
        pred_module: The predictor module.
        predictor_input_keys: Feature names from the predictor's input schema.
        predictor_takes_dict: Whether the predictor's ``forward`` takes a
            single dict argument.

    Returns:
        ``predictor_input_keys``, reordered to match ``forward()`` when the
        predictor takes positional tensors.

    Raises:
        ValueError: If a positional predictor's ``forward()`` is missing a
            parameter named in the schema.
    """
    if predictor_takes_dict:
        return predictor_input_keys
    forward_params = _forward_param_order(pred_module)
    schema_set = set(predictor_input_keys)
    if forward_params and schema_set:
        forward_param_set = set(forward_params)
        if not schema_set.issubset(forward_param_set):
            unknown = sorted(schema_set - forward_param_set)
            raise ValueError(
                "Predictor model_schema includes input names that are not "
                f"parameters of forward(): {unknown}. forward() parameters "
                f"(excluding self): {forward_params}. Align the model_schema "
                "with the module's forward(), or use "
                "forward(inputs: dict[str, torch.Tensor]) so the fused model "
                "passes a dict."
            )
        return [p for p in forward_params if p in schema_set]
    return predictor_input_keys


def _build_fused_model_and_sample(
    torch_model_path: str,
    tx_model_path: str,
    model_class: str,
    hyperparameters: dict[str, Any],
    tx_model_class: str,
    tx_hyperparameters: dict[str, Any],
    tx_model_schema: ModelSchema | None = None,
    model_schema: ModelSchema | None = None,
) -> tuple[FusedModel, dict[str, torch.Tensor], list[str]]:
    """Load transform + predictor, build the ``FusedModel``, and a sample batch.

    Args:
        torch_model_path: Local path to the predictor model.
        tx_model_path: Local path to the native-transform model.
        model_class: Dotted class name for the predictor.
        hyperparameters: Constructor kwargs for the predictor.
        tx_model_class: Dotted class name for the transform.
        tx_hyperparameters: Constructor kwargs for the transform.
        tx_model_schema: Native-transform model schema.
        model_schema: Predictor model schema.

    Returns:
        A tuple of ``(fused_module, sample_input, input_key_order)`` where
        ``fused_module`` is on the trace device, ``sample_input`` is a dict
        of sample tensors, and ``input_key_order`` is the sample's key order
        (matching ONNX/Triton input names).

    Raises:
        ValueError: If the fused input schema is empty, so no sample input
            can be built for tracing.
    """
    tx_hyperparameters = tx_hyperparameters or {}
    hyperparameters = hyperparameters or {}

    transform_module = _load_module_from_path(
        tx_model_path, tx_model_class, tx_hyperparameters
    )
    predictor_module = _load_module_from_path(
        torch_model_path, model_class, hyperparameters
    )

    transform_input_keys = _schema_input_keys(tx_model_schema)
    predictor_takes_dict = _forward_accepts_dict(predictor_module)
    predictor_input_keys = _schema_input_keys(model_schema)
    predictor_input_keys = _align_predictor_input_keys(
        predictor_module, predictor_input_keys, predictor_takes_dict
    )

    fused = FusedModel(
        transform_module=transform_module,
        predictor_module=predictor_module,
        transform_input_keys=transform_input_keys,
        predictor_input_keys=predictor_input_keys,
        predictor_takes_dict=predictor_takes_dict,
    )
    fused.eval()

    sample_input = _build_fused_sample_input(tx_model_schema, model_schema)
    if not sample_input:
        raise ValueError(
            "Cannot build sample input for trace: the fused input schema "
            "(from fuse_input_schema) is empty."
        )
    trace_device = next(iter(sample_input.values())).device
    fused = fused.to(trace_device)

    input_key_order = list(sample_input.keys())
    return fused, sample_input, input_key_order


def _build_tx_hydra_spec(tx_hyperparameters: dict[str, Any]) -> dict[str, Any]:
    """Build a Hydra reconstruction spec for a fused native-transform layer stack.

    Not yet implemented in OSS michelangelo: reconstructing a native-transform
    module/layer stack from a stored transform specification dict requires
    the native-transform package, which has not been migrated. This blocks
    only the Python-backend *raw* package for a native-transform-fused model
    (:func:`~michelangelo.lib.shared.utils.model_fuser.fuse.fuse_models_to_python`);
    the plain (no native-transform) path and the TorchScript/ONNX fused
    deployable paths do not call this function.

    Args:
        tx_hyperparameters: The transform model's serialized hyperparameters
            dict (the shape a future native-transform package's own
            ``to_dict()`` would produce).

    Raises:
        NotImplementedError: Always, until native-transform support lands.
    """
    raise NotImplementedError(
        "Building a Hydra reconstruction spec for a fused native-transform "
        "model requires the native-transform package, which is not yet "
        "available in OSS michelangelo."
    )
