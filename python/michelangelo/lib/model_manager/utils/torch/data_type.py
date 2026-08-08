"""Conversions between ModelSchema DataType and torch.dtype."""

import torch

from michelangelo.lib.model_manager.schema import DataType

_DATA_TYPE_TO_TORCH_DTYPE: dict[DataType, torch.dtype] = {
    DataType.FLOAT: torch.float32,
    DataType.DOUBLE: torch.float64,
    DataType.INT: torch.int32,
    DataType.SHORT: torch.int16,
    DataType.BYTE: torch.int8,
    DataType.LONG: torch.int64,
    DataType.BOOLEAN: torch.bool,
}

_TORCH_DTYPE_TO_DATA_TYPE: dict[torch.dtype, DataType] = {
    torch.float32: DataType.FLOAT,
    torch.float64: DataType.DOUBLE,
    torch.int32: DataType.INT,
    torch.int16: DataType.SHORT,
    torch.int8: DataType.BYTE,
    torch.int64: DataType.LONG,
    torch.bool: DataType.BOOLEAN,
}


def data_type_to_torch_dtype(data_type: DataType) -> torch.dtype:
    """Map a ModelSchema DataType to the equivalent torch.dtype.

    Used when building sample tensors (e.g. for tracing/scripting) from a
    ModelSchema whose feature types are expressed as DataType values.

    Args:
        data_type: The ModelSchema data type to convert.

    Returns:
        The equivalent torch.dtype.

    Raises:
        ValueError: If ``data_type`` has no torch.dtype equivalent.
    """
    if data_type not in _DATA_TYPE_TO_TORCH_DTYPE:
        raise ValueError(f"Cannot convert data type {data_type} to torch.dtype")
    return _DATA_TYPE_TO_TORCH_DTYPE[data_type]


def torch_dtype_to_data_type(dtype: torch.dtype) -> DataType:
    """Map a torch.dtype to the equivalent ModelSchema DataType.

    Note: float16 (half) and bfloat16 are not yet supported — ModelSchema has
    no corresponding DataType for reduced-precision floats.
    # TODO: support torch.float16 and torch.bfloat16 once DataType gains
    # FLOAT16 / BFLOAT16 variants.

    Args:
        dtype: The torch.dtype to convert.

    Returns:
        The equivalent ModelSchema DataType.

    Raises:
        ValueError: If ``dtype`` has no DataType equivalent (including
            torch.float16 and torch.bfloat16, which are not yet supported).
    """
    if dtype not in _TORCH_DTYPE_TO_DATA_TYPE:
        raise ValueError(
            f"Cannot convert torch.dtype {dtype} to DataType. "
            f"float16 and bfloat16 are not yet supported."
        )
    return _TORCH_DTYPE_TO_DATA_TYPE[dtype]
