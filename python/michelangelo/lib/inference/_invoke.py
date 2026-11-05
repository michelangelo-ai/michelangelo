"""Calling conventions shared by the built-in backends.

Each helper calls a model the way the packager's validation and Triton serving
do, so offline scores match what the deployed model returns.
"""

from __future__ import annotations

import os
from typing import Any

import numpy as np
import yaml

#: Name given to a single unnamed output when the model has no output schema.
DEFAULT_OUTPUT_NAME = "prediction"


def to_torch_inputs(inputs: dict[str, np.ndarray], device: str) -> dict[str, Any]:
    """Convert numpy columns to what a PyTorch model takes.

    Numeric columns become tensors on ``device``. String columns become a list
    of ``str`` with one item per row, which is how Triton passes a ``STRING``
    input to a TorchScript ``List[str]`` argument.

    Args:
        inputs: Column name to array, with the batch on axis 0.
        device: Device for the tensors.

    Returns:
        Column name to tensor or list of strings.
    """
    import torch

    converted: dict[str, Any] = {}
    for name, arr in inputs.items():
        if arr.dtype.kind in ("O", "S", "U"):
            flat = (
                arr.reshape(arr.shape[0])
                if arr.ndim == 2 and arr.shape[1] == 1
                else arr
            )
            converted[name] = [
                x.decode("utf-8") if isinstance(x, bytes) else x for x in flat.tolist()
            ]
        else:
            # Arrow zero-copy batches are read-only, which torch warns about
            # and cannot safely share, so copy only those.
            converted[name] = torch.as_tensor(
                arr if arr.flags.writeable else arr.copy(), device=device
            )
    return converted


def call_torchscript(model: Any, inputs: dict[str, Any], order: list[str]) -> Any:
    """Call a TorchScript module.

    A module whose only argument accepts a ``Dict`` (including a ``Union``
    with one) gets the whole batch as one dict. Otherwise the arguments are
    matched by name when every argument name is a column, and by position in
    ``order`` when not, which is how Triton's PyTorch backend binds inputs.

    Args:
        model: The ``torch.jit.ScriptModule``.
        inputs: Column name to tensor or list of strings.
        order: Input names in schema order, for positional binding.

    Returns:
        The module's raw output.
    """
    args = model.forward.schema.arguments[1:]
    if len(args) == 1 and _accepts_dict(args[0].type):
        return model(inputs)
    if args and all(a.name in inputs for a in args):
        return model(**{a.name: inputs[a.name] for a in args})
    return model(*(inputs[name] for name in order))


def _accepts_dict(arg_type: Any) -> bool:
    """Whether a TorchScript argument type is a ``Dict`` or a ``Union`` with one."""
    kind = arg_type.kind()
    if kind == "DictType":
        return True
    if kind in ("UnionType", "OptionalType"):
        return any(_accepts_dict(t) for t in arg_type.containedTypes())
    return False


def call_module(model: Any, inputs: dict[str, Any]) -> Any:
    """Call a PyTorch ``nn.Module`` the way packaging validation does.

    Args:
        model: The module.
        inputs: Column name to tensor or list of strings.

    Returns:
        The module's raw output.
    """
    from michelangelo.lib.model_manager._private.packager.torch_triton.validation import (  # noqa: E501
        _invoke_model,
    )

    return _invoke_model(model, inputs)


def name_outputs(output: Any, output_names: list[str] | None) -> dict[str, np.ndarray]:
    """Turn a model's output into named numpy arrays.

    Args:
        output: A dict of tensors, a tuple (a ``NamedTuple`` is matched by
            field name), or a single tensor.
        output_names: Output names from the schema, in schema order.

    Returns:
        Output name to array.

    Raises:
        ValueError: If the model returned a different number of outputs than
            the schema names.
    """
    if isinstance(output, dict):
        return {k: _to_numpy(v) for k, v in output.items()}

    if isinstance(output, (tuple, list)):
        fields = list(getattr(output, "_fields", ()))
        if fields and (not output_names or set(fields) == set(output_names)):
            return {name: _to_numpy(getattr(output, name)) for name in fields}
        values, names = list(output), output_names or fields
    else:
        values, names = [output], output_names or [DEFAULT_OUTPUT_NAME]

    if len(names) != len(values):
        raise ValueError(
            f"The model returned {len(values)} output(s) but the schema "
            f"names {len(names)}: {names}."
        )
    return {name: _to_numpy(value) for name, value in zip(names, values)}


def predict_custom(
    model: Any, inputs: dict[str, np.ndarray], batch_inference: bool
) -> dict[str, np.ndarray]:
    """Call a custom ``Model`` on a batch, or once per row.

    A custom model packaged without batch processing takes one record at a
    time, and the serving wrapper loops over the batch. This does the same.

    Args:
        model: The custom ``Model``.
        inputs: Column name to array, with the batch on axis 0.
        batch_inference: Whether ``predict`` takes the whole batch.

    Returns:
        Output name to array, with the batch on axis 0.
    """
    if batch_inference:
        return model.predict(inputs)

    num_rows = len(next(iter(inputs.values())))
    results = [
        model.predict({k: v[i] for k, v in inputs.items()}) for i in range(num_rows)
    ]
    return {name: np.stack([r[name] for r in results]) for name in results[0]}


def read_batch_inference(model_path: str) -> bool:
    """Read ``batch_inference`` from a package's ``metadata/type.yaml``.

    Args:
        model_path: A raw or deployable model package path.

    Returns:
        The flag, or ``False`` when the file or the key is absent, which is the
        packager's default of one record per ``predict`` call.
    """
    path = os.path.join(model_path, "metadata", "type.yaml")
    if not os.path.exists(path):
        return False
    with open(path) as f:
        content = yaml.safe_load(f) or {}
    return bool(content.get("batch_inference", False))


def _to_numpy(value: Any) -> np.ndarray:
    """Convert a tensor to a numpy array on the CPU."""
    if isinstance(value, np.ndarray):
        return value
    return value.detach().cpu().numpy()
