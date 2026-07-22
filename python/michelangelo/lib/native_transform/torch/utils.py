"""Utility helpers for PyTorch native transform layers.

Helpers for dtype resolution, layer-name generation, and the
dict-of-tensors input/output contract shared by every transform layer. The
``format_inputs`` / ``format_outputs`` pair defines the package's I/O convention:
layers receive and return ``dict[str, torch.Tensor]`` (TorchScript-friendly),
stacking the selected columns into a single tensor for vectorized computation.
"""

from __future__ import annotations

import random
import re
import string

import torch

from michelangelo.lib.constants.sentinel import FLOAT_SENTINEL, INT32_SENTINEL
from michelangelo.lib.native_transform.torch.constants import (
    STRING_DATA_TYPE_TO_TORCH_TYPE_MAP,
    TORCH_DTYPE_CLASS_NAME_TO_TORCH_TYPE_MAP,
)

__all__ = [
    "format_inputs",
    "format_outputs",
    "generate_layer_name",
    "id_generator",
    "initialize_dtype",
    "resolve_torch_dtype",
    "sentinel_for_torch_dtype",
    "to_snake_case",
]


def sentinel_for_torch_dtype(dtype: torch.dtype) -> float | int:
    """Return the type-native sentinel value for a torch dtype.

    Args:
        dtype: The torch dtype to look up a sentinel for.

    Returns:
        ``FLOAT_SENTINEL`` (NaN) for floating-point dtypes and ``INT32_SENTINEL``
        for integer dtypes.

    Raises:
        ValueError: If no sentinel is defined for ``dtype``.
    """
    if dtype in (torch.float32, torch.float64):
        return FLOAT_SENTINEL
    if dtype in (torch.int32, torch.int64):
        return INT32_SENTINEL
    raise ValueError(f"No sentinel defined for dtype: {dtype}")


def id_generator(
    size: int = 10, chars: str = string.ascii_uppercase + string.digits
) -> str:
    """Generate a random identifier string.

    Args:
        size: Number of characters in the generated string.
        chars: Character set to sample from. Defaults to uppercase ASCII letters
            and digits.

    Returns:
        A random string of length ``size`` drawn from ``chars``.
    """
    return "".join(random.choice(chars) for _ in range(size))


def to_snake_case(name: str) -> str:
    """Convert a class-style name to snake_case.

    Adapted from the Keras backend helper. Names that would begin with an
    underscore (i.e. from private class names) are prefixed with ``"private"``,
    since a leading underscore is not a valid TorchScript scope name.

    Args:
        name: The name to convert (e.g. a class name in ``CamelCase``).

    Returns:
        The snake_case form of ``name``.
    """
    intermediate = re.sub("(.)([A-Z][a-z0-9]+)", r"\1_\2", name)
    insecure = re.sub("([a-z])([A-Z])", r"\1_\2", intermediate).lower()
    # A leading underscore (from a private class name) is not a valid scope
    # name, so prefix it with "private". An empty string has no leading
    # underscore, so it is returned unchanged.
    if not insecure or insecure[0] != "_":
        return insecure
    return "private" + insecure


def generate_layer_name(layer_name: str) -> str:
    """Generate a unique snake_case layer name.

    Args:
        layer_name: The base name to derive from (typically a layer class name).

    Returns:
        The snake_case form of ``layer_name`` suffixed with a random identifier,
        e.g. ``"concatenate_A1B2C3D4E5"``.
    """
    return f"{to_snake_case(layer_name)}_{id_generator()}"


def resolve_torch_dtype(dtype_spec: torch.dtype | str) -> torch.dtype | str:
    """Resolve a dtype spec to a concrete torch dtype.

    Args:
        dtype_spec: Either a ``torch.dtype`` or a string alias. Recognized
            strings include the ``"torch."``-prefixed class names (e.g.
            ``"torch.float32"``) and the bare aliases (e.g. ``"float32"``). The
            special value ``"string"`` resolves to itself.

    Returns:
        The resolved ``torch.dtype`` (or ``"string"`` for the string alias).

    Raises:
        ValueError: If ``dtype_spec`` cannot be resolved.
    """
    if isinstance(dtype_spec, torch.dtype):
        return dtype_spec
    if isinstance(dtype_spec, str):
        if dtype_spec in TORCH_DTYPE_CLASS_NAME_TO_TORCH_TYPE_MAP:
            return TORCH_DTYPE_CLASS_NAME_TO_TORCH_TYPE_MAP[dtype_spec]
        if dtype_spec in STRING_DATA_TYPE_TO_TORCH_TYPE_MAP:
            return STRING_DATA_TYPE_TO_TORCH_TYPE_MAP[dtype_spec]
    raise ValueError(f"Unsupported dtype specification: {dtype_spec}")


def initialize_dtype(
    raw_dtype: torch.dtype | str | None, default_dtype: torch.dtype | None
) -> torch.dtype | str | None:
    """Resolve a layer's dtype argument, falling back to a default.

    String inputs are resolved through :func:`resolve_torch_dtype`, so the two
    functions agree on every string: both the ``"torch."``-prefixed class names
    (e.g. ``"torch.float32"``) and the bare aliases (e.g. ``"float32"``) are
    recognized, and an unrecognized string raises ``ValueError`` rather than
    silently resolving to ``None``.

    Args:
        raw_dtype: The dtype value from a layer spec. May be a ``torch.dtype``, a
            string alias (e.g. ``"float32"`` or ``"torch.float32"``), or
            ``None``.
        default_dtype: The dtype to return when ``raw_dtype`` is neither a
            ``torch.dtype`` nor a string (e.g. ``None``).

    Returns:
        The resolved ``torch.dtype`` for a dtype or recognized string, ``"string"``
        for the string-type alias, or ``default_dtype`` when ``raw_dtype`` is
        neither a ``torch.dtype`` nor a string.

    Raises:
        ValueError: If ``raw_dtype`` is a string that names no recognized dtype.
    """
    if isinstance(raw_dtype, torch.dtype):
        return raw_dtype
    if isinstance(raw_dtype, str):
        return resolve_torch_dtype(raw_dtype)
    return default_dtype


def format_inputs(
    input_columns: list[str], inputs: dict[str, torch.Tensor]
) -> torch.Tensor:
    """Stack selected input columns into a single tensor.

    Args:
        input_columns: The column names to select, in order.
        inputs: Mapping from column name to tensor.

    Returns:
        A tensor stacking ``inputs[col]`` for each column in ``input_columns``
        along a new leading dimension.
    """
    return torch.stack([inputs[col] for col in input_columns])


def format_outputs(
    output_columns: list[str], outputs: torch.Tensor
) -> dict[str, torch.Tensor]:
    """Split a stacked output tensor into a column-keyed dictionary.

    Inverse of :func:`format_inputs`: unbinds ``outputs`` along its leading
    dimension and maps each slice to the corresponding output column name.

    Args:
        output_columns: The output column names, in order.
        outputs: The stacked output tensor to split.

    Returns:
        A mapping from each output column name to its tensor slice.
    """
    output_tensors = torch.unbind(outputs)
    return {output_columns[i]: output_tensors[i] for i in range(len(output_columns))}
