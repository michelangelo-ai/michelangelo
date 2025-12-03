from typing import Optional

from numpy import ndarray

from michelangelo.lib.model_manager.schema import DataType, ModelSchemaItem


def validate_numpy_data(
    data: list[dict[str, ndarray]],
) -> tuple[bool, Optional[Exception]]:
    """Validate the numpy data.

    Args:
        data: The data to validate

    Returns:
        Tuple containing a boolean indicating whether the sample data is valid and
        an exception if the sample data is invalid
    """
    if not isinstance(data, list):
        return False, TypeError("data must be a list of dictionaries of numpy arrays")

    for record in data:
        is_valid, err = validate_numpy_data_record(record)
        if not is_valid:
            return False, err

    return True, None


def validate_numpy_data_record(
    record: dict[str, ndarray],
) -> tuple[bool, Optional[Exception]]:
    """Validate the data record.

    Args:
        record: The data record to validate

    Returns:
        Tuple containing a boolean indicating whether the data record is valid and
        an exception if the data record is invalid
    """
    if not isinstance(record, dict):
        return False, TypeError("data must dictionaries of numpy arrays")

    for key, value in record.items():
        if not isinstance(key, str):
            return False, TypeError("data keys must be strings")

        if not isinstance(value, ndarray):
            return False, TypeError("data values must be numpy arrays")

    return True, None


def validate_numpy_data_with_model_schema(
    data: list[dict[str, ndarray]],
    schema_items: list[ModelSchemaItem],
    batch_inference: Optional[bool] = False,
) -> tuple[bool, Optional[Exception]]:
    """Validate the numpy data with the model schema.

    Args:
        data: The data to validate
        schema_items: The model schema items to validate against
        batch_inference: Optional flag for batch inference validation

    Returns:
        Tuple containing a boolean indicating whether the sample data is valid and
        an exception if the sample data is invalid
    """
    for record in data:
        is_valid, err = validate_numpy_data_record_with_model_schema(
            record, schema_items, batch_inference
        )
        if not is_valid:
            return False, err

    return True, None


def validate_numpy_data_record_with_model_schema(
    record: dict[str, ndarray],
    schema_items: list[ModelSchemaItem],
    batch_inference: Optional[bool] = False,
) -> tuple[bool, Optional[Exception]]:
    """Validate the data record with the model schema.

    Args:
        record: The data record to validate
        schema_items: The model schema items to validate against
        batch_inference: Optional flag for batch inference validation

    Returns:
        Tuple containing a boolean indicating whether the data record is valid and
        an exception if the data record is invalid
    """
    record_keys = set(record.keys())
    schema_keys = {item.name for item in schema_items}

    if record_keys != schema_keys:
        return False, ValueError(
            "Data fields do not match schema fields\n"
            f"Fields in data but missing in schema: {record_keys - schema_keys}\n"
            f"Fields in schema but missing in data: {schema_keys - record_keys}"
        )

    for schema_item in schema_items:
        name = schema_item.name
        item = record[name]
        is_valid, err = validate_data_type(name, item, schema_item.data_type)
        if not is_valid:
            return False, err
        is_valid, err = validate_shape(name, item, schema_item.shape, batch_inference)
        if not is_valid:
            return False, err

    return True, None


def validate_data_type(
    name: str, arr: ndarray, data_type: DataType
) -> tuple[bool, Optional[Exception]]:
    """Validate the data type.

    Args:
        name: The name of the field
        arr: The numpy array to validate
        data_type: The data type to validate against

    Returns:
        Tuple containing a boolean indicating whether the data type is valid and
        an exception if the data type is invalid
    """
    kind = arr.dtype.kind

    if kind == "c":
        return False, TypeError(
            f'Data contains unsupported data type "{arr.dtype}" in {name}: {arr}'
        )

    type_err = TypeError(
        f"Found incompatible data type between data and model schema for field {name}. "
        f"Expected data type {data_type.name} in schema but got {arr.dtype} in {arr}"
    )

    if data_type == DataType.BOOLEAN and kind != "b":
        return False, type_err

    if data_type in {
        DataType.BYTE,
        DataType.CHAR,
        DataType.INT,
        DataType.SHORT,
        DataType.LONG,
    } and kind not in {"i", "u"}:
        return False, type_err

    if data_type in {DataType.FLOAT, DataType.DOUBLE} and kind not in {"f", "i", "u"}:
        return False, type_err

    if data_type == DataType.STRING and kind not in {
        "U",
        "S",
        "O",
        "M",
        "m",
        "V",
    }:
        return False, type_err

    return True, None


def validate_shape(
    name: str,
    arr: ndarray,
    shape: list[int],
    batch_inference: Optional[bool] = False,
) -> tuple[bool, Optional[Exception]]:
    """Validate the shape.

    Args:
        name: The name of the field
        arr: The numpy array to validate
        shape: The shape to validate against
        batch_inference: Optional flag for batch inference validation

    Returns:
        Tuple containing a boolean indicating whether the shape is valid and an
        exception if the shape is invalid
    """
    sp = [-1, *shape] if batch_inference else shape

    if len(arr.shape) != len(sp):
        msg = (
            "Found mismatching number of dimensions between data and model "
            f"schema for field {name}. "
            f"Expected {len(sp)} dimensions in schema but got {arr.ndim} in {arr}."
        )

        batch_inference_warning = (
            "\nNote: batch inference is enabled for the model, "
            "meaning the input/output of the model takes an additional batch "
            "dimension on top of the existing shape.\n"
            "For example, "
            "if the model schema specify the input shape to be [n,..., m], "
            "the input data should have a shape of [b, n, ..., m] "
            "where b is the batch size.\n"
            "If you do not want this behavior, you can try to set "
            "custom_batch_processing=False in the packager. "
            "For example, "
            "packager = CustomTritonPackager(custom_batch_processing=False)."
        )

        if batch_inference:
            msg += batch_inference_warning

        return False, ValueError(msg)

    if any(d != s and s > 0 for d, s in zip(arr.shape, sp)):
        return False, ValueError(
            "Found mismatching dimensions between data and model schema for "
            f"field {name}. "
            f"Expected {tuple(shape)} in schema but got {arr.shape} in {arr}"
        )

    return True, None
