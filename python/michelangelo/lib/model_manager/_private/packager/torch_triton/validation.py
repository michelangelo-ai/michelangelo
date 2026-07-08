"""Validation functions for the torch_triton packager."""

from __future__ import annotations

import os
from typing import TYPE_CHECKING, Any

import onnx
import torch
from numpy import ndarray

from michelangelo._internal.utils.reflection_utils import get_module_attr
from michelangelo.lib.model_manager._private.packager.torch_triton.constants import (
    MODEL_CLASS_FILE_NAME,
    RAW_SUBMODEL_SCHEMAS_FILE,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.submodel_schema import (  # noqa: E501
    capture_submodel_schemas,
    get_forward_param_names,
    write_submodel_schemas,
)
from michelangelo.lib.model_manager._private.utils.data_utils import (
    validate_output_data,
    validate_output_data_with_model_schema,
)
from michelangelo.lib.model_manager._private.utils.torch_utils import is_state_dict
from michelangelo.lib.model_manager.serde.model import load_raw_model

if TYPE_CHECKING:
    from michelangelo.lib.model_manager.schema import ModelSchema, ModelSchemaItem


def _validate_file_basics(
    file_path: str,
    allowed_extensions: tuple[str, ...] = (".pt", ".pth"),
) -> Exception | None:
    """Validate basic properties of a model file.

    Args:
        file_path: Path to the file to check.
        allowed_extensions: Permitted file extensions.

    Returns:
        An exception describing the first problem found, or None if valid.
    """
    if not os.path.exists(file_path):
        return FileNotFoundError(f"PyTorch file not found: {file_path}")
    if os.path.isdir(file_path):
        return ValueError(f"Path is not a file: {file_path}")
    if not file_path.endswith(allowed_extensions):
        return ValueError(f"File must have {allowed_extensions} extension: {file_path}")
    if os.path.getsize(file_path) == 0:
        return ValueError(f"File is empty: {file_path}")
    return None


def validate_state_dict_file(model_path: str) -> tuple[bool, Exception | None]:
    """Validate that a file contains a state_dict dictionary.

    Args:
        model_path: Path to the model file.

    Returns:
        A (is_valid, error) tuple where error is None when valid.
    """
    try:
        if error := _validate_file_basics(model_path):
            return False, error

        state_dict = torch.load(model_path, map_location="cpu", weights_only=True)
        if isinstance(state_dict, dict):
            return True, None
        return False, ValueError(
            f"File does not contain a state_dict dictionary: {model_path}"
        )
    except Exception as e:
        return False, RuntimeError(f"Cannot load file as state_dict: {e}")


def validate_torchscript_file(model_path: str) -> tuple[bool, Exception | None]:
    """Validate that a file contains a TorchScript model.

    Args:
        model_path: Path to the model file.

    Returns:
        A (is_valid, error) tuple where error is None when valid.
    """
    try:
        if error := _validate_file_basics(model_path):
            return False, error

        torch.jit.load(model_path, map_location="cpu")
    except Exception as e:
        return False, RuntimeError(f"File is not a valid TorchScript model: {e}")
    else:
        return True, None


def validate_pytorch_model_file(
    model_path: str,
    require_state_dict: bool = False,
) -> tuple[bool, Exception | None]:
    """Validate that a file contains a PyTorch nn.Module or state_dict.

    Args:
        model_path: Path to the model file.
        require_state_dict: If True, also require a state_dict method on a full
            model (used for raw packages).

    Returns:
        A (is_valid, error) tuple where error is None when valid.
    """
    try:
        if error := _validate_file_basics(model_path):
            return False, error

        model = torch.load(
            model_path, map_location="cpu", weights_only=False
        )  # weights_only=False: artifact may be a full nn.Module, not a state_dict
        if not is_state_dict(model) and not isinstance(model, torch.nn.Module):
            return False, ValueError(
                "File must contain a PyTorch nn.Module or state_dict"
            )

        if require_state_dict and not hasattr(model, "state_dict"):
            return False, ValueError("PyTorch model does not have state_dict method")
    except Exception as e:
        return False, RuntimeError(f"Cannot load as PyTorch model: {e}")
    else:
        return True, None


def validate_deployable_onnx_file(
    model_onnx_path: str,
) -> tuple[bool, Exception | None]:
    """Validate that a file is a loadable ONNX model.

    Validation is content-based, so the file extension may differ from .onnx.

    Args:
        model_onnx_path: Path to the candidate ONNX file.

    Returns:
        A (is_valid, error) tuple where error is None when valid.
    """
    try:
        if not os.path.exists(model_onnx_path):
            return False, FileNotFoundError(f"ONNX file not found: {model_onnx_path}")
        if os.path.isdir(model_onnx_path):
            return False, ValueError(f"Path is not a file: {model_onnx_path}")
        if os.path.getsize(model_onnx_path) == 0:
            return False, ValueError(f"File is empty: {model_onnx_path}")
        model_proto = onnx.load(model_onnx_path)
        onnx.checker.check_model(model_proto)
    except Exception as e:
        return False, RuntimeError(f"File is not a valid ONNX model: {e}")
    else:
        return True, None


def validate_deployable_model_file(
    model_pt_path: str,
) -> tuple[bool, Exception | None]:
    """Validate a PyTorch file for deployable packages.

    Accepts a TorchScript model or any convertible full nn.Module.

    Args:
        model_pt_path: Path to the .pt file.

    Returns:
        A (is_valid, error) tuple where error is None when valid.
    """
    is_torchscript, _ = validate_torchscript_file(model_pt_path)
    if is_torchscript:
        return True, None

    return validate_pytorch_model_file(model_pt_path)


def validate_raw_model_file(model_path: str) -> tuple[bool, Exception | None]:
    """Validate a PyTorch file for raw packages.

    Accepts a state_dict or any convertible full nn.Module that exposes a
    state_dict method.

    Args:
        model_path: Path to the .pt or .pth file.

    Returns:
        A (is_valid, error) tuple where error is None when valid.
    """
    is_state_dict_file, _ = validate_state_dict_file(model_path)
    if is_state_dict_file:
        return True, None

    return validate_pytorch_model_file(model_path, require_state_dict=True)


def _validate_package_structure(package_path: str) -> str:
    """Validate the raw package directory layout and locate the model file.

    Args:
        package_path: Path to the raw model package.

    Returns:
        Path to the single .pt file inside the package's model directory.

    Raises:
        FileNotFoundError: If a required directory or the model file is missing.
    """
    if not os.path.exists(package_path):
        raise FileNotFoundError(f"Raw model package not found: {package_path}")

    required_dirs = ["metadata", "model", "defs", "dependencies"]
    for dir_name in required_dirs:
        dir_path = os.path.join(package_path, dir_name)
        if not os.path.exists(dir_path):
            raise FileNotFoundError(f"Required directory missing: {dir_path}")

    model_dir = os.path.join(package_path, "model")
    pt_files = [
        f
        for f in os.listdir(model_dir)
        if f.endswith(".pt") and os.path.isfile(os.path.join(model_dir, f))
    ]
    if not pt_files:
        raise FileNotFoundError(f"No .pt file found in model directory: {model_dir}")

    return os.path.join(model_dir, pt_files[0])


def _load_model_class(package_path: str) -> Any:
    """Load the model class referenced by the package definitions.

    Args:
        package_path: Path to the raw model package.

    Returns:
        The resolved model class object.
    """
    with open(os.path.join(package_path, "defs", MODEL_CLASS_FILE_NAME)) as f:
        model_class = f.read().strip()
    return get_module_attr(model_class)


def _has_batch_dimension(tensor: torch.Tensor, expected_shape: list[int]) -> bool:
    """Check whether a tensor already carries a leading batch dimension.

    A tensor is treated as batched when it has exactly one more dimension than
    the schema's expected shape and its trailing dimensions match that shape.
    The batch dimension's size is not constrained, so batches larger than one
    are detected correctly.

    Args:
        tensor: The input tensor to inspect.
        expected_shape: Expected per-sample shape from the schema, without the
            batch dimension.

    Returns:
        True if the tensor appears to already have a batch dimension.
    """
    if tensor.ndim == 0:
        return False
    if not expected_shape:
        return False
    if tensor.ndim == len(expected_shape) + 1:
        return list(tensor.shape[1:]) == expected_shape
    return False


def _add_batch_dimension(
    tensor: torch.Tensor, expected_shape: list[int] | None = None
) -> torch.Tensor:
    """Add a leading batch dimension to a tensor when it lacks one.

    Args:
        tensor: The input tensor.
        expected_shape: Expected per-sample shape from the schema.

    Returns:
        The tensor with a batch dimension guaranteed.
    """
    if not _has_batch_dimension(tensor, expected_shape):
        return tensor.unsqueeze(0)
    return tensor


def _invoke_model(model: torch.nn.Module, batch_dict: dict[str, Any]) -> Any:
    """Call a model with batched inputs, handling both calling conventions.

    Most tabular models take positional features, e.g. forward(self, a, b), and
    are called as model(**batch_dict). Some take a single mapping argument, e.g.
    forward(self, inputs), and must be called as model(batch_dict).

    Args:
        model: The model to invoke.
        batch_dict: Batched inputs keyed by input name.

    Returns:
        The model's raw output.
    """
    params = get_forward_param_names(model)
    if set(batch_dict.keys()).issubset(set(params)):
        return model(**batch_dict)
    return model(batch_dict)


def _remove_batch_size_dimension(
    output: torch.Tensor, output_schema: ModelSchemaItem | None
) -> ndarray:
    """Drop the batch dimension from an output tensor and convert to numpy.

    Args:
        output: The output tensor with a leading batch dimension.
        output_schema: Schema item describing the expected output shape.

    Returns:
        A numpy array with the batch dimension removed. Scalars are reshaped to
        a 1D array when the schema expects at least one dimension.
    """
    output_array = output.squeeze(0).detach().cpu().numpy()
    expected_ndim = len(output_schema.shape) if output_schema else 1
    if output_array.ndim == 0 and expected_ndim > 0:
        output_array = output_array.reshape(-1)
    return output_array


def _build_batch_inputs(
    data: dict[str, ndarray],
    model_schema: ModelSchema,
) -> dict[str, torch.Tensor]:
    """Convert sample inputs to batched tensors keyed by input name.

    Args:
        data: Sample inputs keyed by input name.
        model_schema: Schema providing per-input expected shapes.

    Returns:
        Batched tensors keyed by input name.

    Raises:
        TypeError: If an input value is not a numpy array or torch tensor.
    """
    batch_dict: dict[str, torch.Tensor] = {}
    for key, value in data.items():
        schema_item = next(
            (item for item in model_schema.input_schema if item.name == key), None
        )
        expected_shape = schema_item.shape if schema_item else []

        if isinstance(value, ndarray):
            tensor = torch.from_numpy(value)
        elif isinstance(value, torch.Tensor):
            tensor = value
        else:
            raise TypeError(
                f"Unsupported input type for key '{key}': {type(value)}. "
                "Expected numpy.ndarray or torch.Tensor. Please convert your "
                "input data to numpy arrays before validation."
            )

        batch_dict[key] = _add_batch_dimension(tensor, expected_shape)
    return batch_dict


def _collect_outputs(output: Any, model_schema: ModelSchema) -> dict[str, ndarray]:
    """Normalize a model's raw output into a name -> numpy array mapping.

    Handles a single tensor, a named tuple, and a plain list/tuple, mapping each
    element to its schema output name and stripping the batch dimension.

    Args:
        output: The model's raw output.
        model_schema: Schema providing output names and shapes.

    Returns:
        Output arrays keyed by output name.
    """
    if isinstance(output, torch.Tensor):
        schema_item = (
            model_schema.output_schema[0] if model_schema.output_schema else None
        )
        output_field_name = schema_item.name if schema_item else "output"
        return {output_field_name: _remove_batch_size_dimension(output, schema_item)}

    if isinstance(output, dict):
        return {
            k: _remove_batch_size_dimension(
                v,
                next(
                    (item for item in model_schema.output_schema if item.name == k),
                    None,
                ),
            )
            if isinstance(v, torch.Tensor)
            else v
            for k, v in output.items()
        }

    if isinstance(output, (list, tuple)):
        output_dict: dict[str, ndarray] = {}
        asdict = getattr(output, "_asdict", None)
        if asdict is not None:
            for field_name, out in asdict().items():
                schema_item = next(
                    (
                        item
                        for item in model_schema.output_schema
                        if item.name == field_name
                    ),
                    None,
                )
                output_dict[field_name] = (
                    _remove_batch_size_dimension(out, schema_item)
                    if isinstance(out, torch.Tensor)
                    else out
                )
        else:
            for i, out in enumerate(output):
                schema_item = (
                    model_schema.output_schema[i]
                    if i < len(model_schema.output_schema)
                    else None
                )
                field_name = schema_item.name if schema_item else f"output_{i}"
                output_dict[field_name] = (
                    _remove_batch_size_dimension(out, schema_item)
                    if isinstance(out, torch.Tensor)
                    else out
                )
        return output_dict

    raise TypeError(f"Unsupported model output type: {type(output).__name__}")


def _test_model_predictions(
    package_path: str,
    sample_data: list[dict[str, ndarray]] | dict[str, ndarray],
    model_schema: ModelSchema,
) -> dict | None:
    """Run a forward pass with sample data and validate the outputs.

    Args:
        package_path: Path to the raw model package.
        sample_data: Sample inputs for the forward pass.
        model_schema: Schema used to validate the outputs.

    Returns:
        Captured per-submodel schemas, or None when no sample data is provided.

    Raises:
        TypeError: If the loaded model is not an instance of the package class.
        RuntimeError: If the forward pass fails.
    """
    model = load_raw_model(package_path)
    model_class = _load_model_class(package_path)

    if not isinstance(model, model_class):
        raise TypeError(f"The loaded model is not an instance of {model_class}")

    if not sample_data:
        return None

    data = sample_data[0] if isinstance(sample_data, list) else sample_data

    try:
        model.eval()
        with torch.no_grad():
            batch_dict = _build_batch_inputs(data, model_schema)
            output, submodel_schemas = capture_submodel_schemas(
                model, lambda: _invoke_model(model, batch_dict)
            )
            output = _collect_outputs(output, model_schema)
    except Exception as e:
        raise RuntimeError(
            f"Error during test prediction with the model. Error: {e}"
        ) from e

    is_valid, err = validate_output_data(output)
    if not is_valid:
        raise err

    is_valid, err = validate_output_data_with_model_schema(output, model_schema)
    if not is_valid:
        raise err

    return submodel_schemas


def validate_raw_model_package(
    package_path: str,
    sample_data: list[dict[str, ndarray]] | dict[str, ndarray] | None = None,
    model_schema: ModelSchema | None = None,
) -> None:
    """Validate a PyTorch raw model package.

    When sample_data and model_schema are both provided, a forward pass is run
    to validate outputs and per-submodel schemas are written to the package's
    metadata directory as ``submodel_schemas.yaml``.

    Args:
        package_path: Path to the raw model package.
        sample_data: Optional sample inputs for prediction testing.
        model_schema: Optional schema for output validation.

    Raises:
        RuntimeError: If the package's model file is invalid.
    """
    pt_file_path = _validate_package_structure(package_path)
    _load_model_class(package_path)

    is_valid, error = validate_raw_model_file(pt_file_path)
    if not is_valid:
        raise RuntimeError(f"Invalid raw model file {pt_file_path}: {error}") from error

    if sample_data and model_schema:
        submodel_schemas = _test_model_predictions(
            package_path, sample_data, model_schema
        )
        if submodel_schemas:
            write_submodel_schemas(
                package_path, submodel_schemas, RAW_SUBMODEL_SCHEMAS_FILE
            )


def validate_model_class(model_class: str) -> tuple[bool, Exception | None]:
    """Validate that a model class is a torch.nn.Module subclass.

    Args:
        model_class: The model class import path, e.g. "my.module.ModelClass".

    Returns:
        A (is_valid, error) tuple where error is None when valid.
    """
    try:
        resolved_class = get_module_attr(model_class)
    except (ValueError, ImportError) as e:
        return False, e

    if not issubclass(resolved_class, torch.nn.Module):
        return False, TypeError(
            f"Model class {model_class} must be a subclass of torch.nn.Module"
        )

    return True, None
