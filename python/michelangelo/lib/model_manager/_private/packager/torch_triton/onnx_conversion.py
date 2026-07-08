"""Export PyTorch models to ONNX for the onnxruntime Triton backend."""

from __future__ import annotations

import logging
import os
import shutil
from typing import TYPE_CHECKING, Any

import numpy as np
import onnx
import torch

from michelangelo.lib.model_manager._private.packager.torch_triton.validation import (
    validate_deployable_onnx_file,
)
from michelangelo.lib.model_manager._private.utils.torch_utils import (
    is_state_dict,
    load_model_from_state_dict,
)

if TYPE_CHECKING:
    from michelangelo.lib.model_manager.schema import ModelSchema

OPSET_VERSION = 17

_logger = logging.getLogger(__name__)


def _expand_sample_inputs(
    sample_inputs: tuple[torch.Tensor, ...],
) -> tuple[torch.Tensor, ...]:
    """Expand the batch dimension to at least 2 when it is 1.

    A batch size of 1 during tracing can bake a fixed batch size into the
    exported ONNX ops, which breaks dynamic batching at serve time.

    Args:
        sample_inputs: Trace tensors in input order.

    Returns:
        The same tensors with any size-1 batch dimension repeated to size 2.
    """
    expanded_batch_size = 2
    expanded: list[torch.Tensor] = []
    for inp in sample_inputs:
        if inp.size(0) > 1:
            expanded.append(inp)
        else:
            expanded.append(inp.repeat(expanded_batch_size, *[1] * (inp.dim() - 1)))
    return tuple(expanded)


def _prepare_sample_inputs(
    input_names: list[str],
    sample_data: dict[str, Any],
) -> tuple[torch.Tensor, ...]:
    """Build trace tensors from sample_data ordered by input_names.

    Args:
        input_names: Input tensor names in schema order.
        sample_data: Mapping of input name to a torch.Tensor or numpy.ndarray.

    Returns:
        Trace tensors with batch dimensions expanded for ONNX export.

    Raises:
        ValueError: If sample_data is missing a required input.
        TypeError: If a sample value is not a tensor or ndarray.
    """
    sample_list: list[torch.Tensor] = []
    for name in input_names:
        if name not in sample_data:
            raise ValueError(f"onnx_sample_data missing required input '{name}'")
        val = sample_data[name]
        if isinstance(val, torch.Tensor):
            sample_list.append(val)
        elif isinstance(val, np.ndarray):
            sample_list.append(torch.from_numpy(val))
        else:
            raise TypeError(
                f"Sample data for '{name}' must be torch.Tensor or "
                f"numpy.ndarray, got {type(val)}"
            )
    return _expand_sample_inputs(tuple(sample_list))


def _load_torch_model(
    source_model_path: str,
    model_class: str | None,
    hyperparameters: dict | None,
) -> torch.nn.Module:
    """Load a TorchScript, pickled nn.Module, or state_dict checkpoint.

    Args:
        source_model_path: Path to the artifact on disk.
        model_class: Import path of the nn.Module subclass, required when the
            artifact is a state_dict.
        hyperparameters: Constructor kwargs used to rebuild the model from a
            state_dict.

    Returns:
        An evaluation-mode nn.Module ready for ONNX export.

    Raises:
        ValueError: If a state_dict is loaded without a model_class.
        TypeError: If the file does not contain a convertible model.
    """
    try:
        return torch.jit.load(source_model_path, map_location="cpu")
    except Exception:
        pass

    try:
        loaded_model = torch.load(
            source_model_path, map_location="cpu", weights_only=False
        )

        if is_state_dict(loaded_model):
            if not model_class:
                raise ValueError(
                    "model_class is required when model_path contains a state_dict"
                )
            model = load_model_from_state_dict(
                loaded_model, model_class, hyperparameters
            )
        else:
            model = loaded_model

        if not isinstance(model, torch.nn.Module):
            raise TypeError(
                "File does not contain a convertible PyTorch module: "
                f"{source_model_path}"
            )
        model.eval()
    except ValueError:
        raise
    except Exception as e:
        raise TypeError(
            "File does not contain a convertible model for ONNX export: "
            f"{source_model_path}"
        ) from e
    else:
        return model


def convert_to_onnx(
    source_model_path: str,
    dest_onnx_path: str,
    model_schema: ModelSchema,
    sample_data: dict[str, Any] | None = None,
    model_class: str | None = None,
    hyperparameters: dict | None = None,
    enable_dynamic_batching: bool = True,
) -> None:
    """Export a PyTorch artifact to ONNX, or copy an existing ONNX file.

    Input names follow the model schema's input order; output names follow the
    schema's output order (matching the Triton config). When the source is
    already a valid ONNX file it is copied as-is and sample_data is unused.
    Otherwise sample_data must map each input name to a batched torch.Tensor or
    numpy.ndarray for tracing.

    When enable_dynamic_batching is True, axis 0 (batch dimension) of every
    input and output is marked dynamic. Other axes (e.g. sequence length,
    spatial dimensions) are frozen to the trace shape. If your model requires
    additional dynamic axes, pre-export the ONNX artifact with the desired
    dynamic_axes configuration and pass the resulting .onnx file as
    source_model_path — it will be copied as-is.

    Args:
        source_model_path: Path to a .onnx file or a PyTorch artifact to export.
        dest_onnx_path: Destination path for the exported ONNX model.
        model_schema: Schema providing input and output tensor names.
        sample_data: Trace inputs keyed by input name; required for PyTorch
            sources.
        model_class: Import path of the nn.Module subclass, required when the
            source is a state_dict.
        hyperparameters: Constructor kwargs used to rebuild a state_dict model.
        enable_dynamic_batching: Whether to mark axis 0 dynamic for batching.

    Raises:
        FileNotFoundError: If source_model_path does not exist.
        ValueError: If sample_data is missing for a PyTorch source.
    """
    if not os.path.exists(source_model_path):
        raise FileNotFoundError(f"File does not exist: {source_model_path}")

    is_onnx, _ = validate_deployable_onnx_file(source_model_path)
    if is_onnx:
        shutil.copy2(source_model_path, dest_onnx_path)
        return

    input_names = [item.name for item in model_schema.input_schema]
    output_names = [item.name for item in model_schema.output_schema]

    if not sample_data:
        raise ValueError(
            "Sample data is missing. Cannot export to ONNX without sample inputs."
        )

    model = _load_torch_model(source_model_path, model_class, hyperparameters)
    sample_inputs = _prepare_sample_inputs(input_names, sample_data)

    try:
        sample_output = model(*sample_inputs)
        _logger.info("Sample output from model before ONNX export: %s", sample_output)
        if hasattr(sample_output, "_fields"):
            output_names = list(sample_output._fields)
    except Exception as e:
        _logger.warning(
            "Failed to run model with sample inputs before ONNX export: %s", e
        )

    dynamic_axes = (
        {name: {0: "b"} for name in input_names + output_names}
        if enable_dynamic_batching
        else {}
    )

    torch.onnx.export(
        model,
        sample_inputs,
        dest_onnx_path,
        input_names=input_names,
        output_names=output_names,
        opset_version=OPSET_VERSION,
        dynamic_axes=dynamic_axes or None,
        do_constant_folding=True,
    )

    exported_model = onnx.load(dest_onnx_path)
    onnx.checker.check_model(exported_model)
