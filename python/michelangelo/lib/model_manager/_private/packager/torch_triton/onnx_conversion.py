"""Export PyTorch models to ONNX for the onnxruntime Triton backend."""

from __future__ import annotations

import logging
import os
import shutil
from typing import TYPE_CHECKING, Any

import pytorch_lightning as pl
import torch

from michelangelo.lib.model_manager._private.packager.torch_triton.validation import (
    validate_deployable_onnx_file,
)
from michelangelo.lib.model_manager._private.utils.torch_utils import (
    is_state_dict,
    load_model_from_state_dict,
)
from michelangelo.lib.model_manager.utils.onnx.torch_onnx import (
    export_torch_to_onnx,
    prepare_sample_inputs,
)

if TYPE_CHECKING:
    from michelangelo.lib.model_manager.schema import ModelSchema

_logger = logging.getLogger(__name__)


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


def _find_onnx_file(directory: str) -> str | None:
    """Return the path to the single ``.onnx`` file in ``directory``, or None.

    Args:
        directory: Directory to search (non-recursive).

    Returns:
        The path to the sole ``.onnx`` file, or None if there isn't exactly one.
    """
    onnx_files = [
        f
        for f in os.listdir(directory)
        if f.endswith(".onnx") and os.path.isfile(os.path.join(directory, f))
    ]
    if len(onnx_files) == 1:
        return os.path.join(directory, onnx_files[0])
    return None


def convert_to_onnx(
    source_model_path: str,
    dest_onnx_path: str,
    model_schema: ModelSchema,
    sample_data: dict[str, Any] | None = None,
    model_class: str | None = None,
    hyperparameters: dict | None = None,
    enable_dynamic_batching: bool = True,
) -> None:
    """Export a PyTorch artifact to ONNX, or move an existing ONNX file into place.

    Three cases, selected by inspecting ``source_model_path``:

    1. ``source_model_path`` is a **directory** containing a prebuilt ONNX graph
       plus its external-data sidecar files (weights split into sibling files
       because the graph exceeds the 2 GiB protobuf serialization limit). Every
       file in the directory is moved next to ``dest_onnx_path`` so onnxruntime
       can resolve the sidecars relative to the ``.onnx`` path, and the graph
       file is renamed to ``dest_onnx_path``'s basename.
    2. ``source_model_path`` is a **file that is already a valid ONNX graph**
       (small, self-contained) — moved as-is to ``dest_onnx_path``.
    3. ``source_model_path`` is a **PyTorch artifact** — exported to ONNX at
       ``dest_onnx_path`` with ``external_data=True``, so weights auto-split
       into sidecar files next to ``dest_onnx_path`` if the export exceeds the
       2 GiB limit (a no-op for smaller models).

    Input names follow the model schema's input order; output names follow the
    schema's output order (matching the Triton config). Cases 1 and 2 don't use
    sample_data; case 3 requires it, mapping each input name to a batched
    torch.Tensor or numpy.ndarray for tracing.

    When enable_dynamic_batching is True, axis 0 (batch dimension) of every
    input and output is marked dynamic. Other axes (e.g. sequence length,
    spatial dimensions) are frozen to the trace shape. If your model requires
    additional dynamic axes, pre-export the ONNX artifact with the desired
    dynamic_axes configuration and pass the resulting .onnx file (or its
    containing directory, if it has external-data sidecars) as
    source_model_path.

    ``source_model_path`` is moved from, not copied — callers are expected to
    pass a staging path they own (e.g. a temporary directory), not a path a
    caller still needs afterward.

    Args:
        source_model_path: Path to a directory of ONNX + sidecars, a .onnx
            file, or a PyTorch artifact to export.
        dest_onnx_path: Destination path for the resulting ONNX model.
        model_schema: Schema providing input and output tensor names.
        sample_data: Trace inputs keyed by input name; required for PyTorch
            sources.
        model_class: Import path of the nn.Module subclass, required when the
            source is a state_dict.
        hyperparameters: Constructor kwargs used to rebuild a state_dict model.
        enable_dynamic_batching: Whether to mark axis 0 dynamic for batching.

    Raises:
        FileNotFoundError: If source_model_path does not exist.
        ValueError: If a source directory doesn't contain exactly one .onnx
            file (or contains a subdirectory, or a sidecar filename colliding
            with the destination graph filename), if that file fails ONNX
            validation, or if sample_data is missing for a PyTorch source.

    Note:
        When the installed torch's ``onnx.export`` doesn't support
        ``external_data`` (older, non-Dynamo exporters), PyTorch-source
        exports fall back to the legacy single-file path with no
        external-data support — a model exceeding the 2 GiB protobuf limit
        will fail during ``torch.onnx.export`` itself. Pre-export such models
        externally (e.g. via ``onnx.save_model(..., save_as_external_data=
        True)``) and pass the resulting directory as source_model_path.
    """
    if not os.path.exists(source_model_path):
        raise FileNotFoundError(f"File does not exist: {source_model_path}")

    if os.path.isdir(source_model_path):
        onnx_src = _find_onnx_file(source_model_path)
        if onnx_src is None:
            raise ValueError(
                f"ONNX directory {source_model_path} must contain exactly one "
                f".onnx file (plus any external-data sidecar files); found: "
                f"{os.listdir(source_model_path)}"
            )
        is_valid, error = validate_deployable_onnx_file(onnx_src)
        if not is_valid:
            raise ValueError(
                f"ONNX file in directory {source_model_path} is not valid: {error}"
            )
        dest_dir = os.path.dirname(dest_onnx_path)
        dest_basename = os.path.basename(dest_onnx_path)
        entries = os.listdir(source_model_path)
        for name in entries:
            src = os.path.join(source_model_path, name)
            if os.path.isdir(src):
                raise ValueError(
                    f"ONNX directory {source_model_path} must contain only "
                    f"the graph and its external-data sidecar files; found "
                    f"subdirectory: {name}"
                )
        sidecar_names = [
            name
            for name in entries
            if os.path.join(source_model_path, name) != onnx_src
        ]
        if dest_basename in sidecar_names:
            raise ValueError(
                f"Sidecar file {dest_basename!r} in {source_model_path} "
                f"collides with the destination graph filename "
                f"{dest_basename!r}"
            )
        for name in entries:
            src = os.path.join(source_model_path, name)
            target_name = dest_basename if src == onnx_src else name
            shutil.move(src, os.path.join(dest_dir, target_name))
        return

    is_onnx, _ = validate_deployable_onnx_file(source_model_path)
    if is_onnx:
        shutil.move(source_model_path, dest_onnx_path)
        return

    input_names = [item.name for item in model_schema.input_schema]
    output_names = [item.name for item in model_schema.output_schema]

    if not sample_data:
        raise ValueError(
            "Sample data is missing. Cannot export to ONNX without sample inputs."
        )

    model = _load_torch_model(source_model_path, model_class, hyperparameters)
    sample_inputs = prepare_sample_inputs(input_names, sample_data)

    try:
        sample_output = model(*sample_inputs)
        _logger.info("Sample output from model before ONNX export: %s", sample_output)
        if hasattr(sample_output, "_fields"):
            output_names = list(sample_output._fields)
    except Exception as e:
        _logger.warning(
            "Failed to run model with sample inputs before ONNX export: %s", e
        )

    is_lightning = isinstance(model, pl.LightningModule)

    export_torch_to_onnx(
        model=model,
        dest_path=dest_onnx_path,
        sample_inputs=sample_inputs,
        input_names=input_names,
        output_names=output_names,
        model_schemas=[model_schema],
        enable_dynamic_batching=enable_dynamic_batching,
        is_lightning_module=is_lightning,
    )

    is_valid, error = validate_deployable_onnx_file(dest_onnx_path)
    if not is_valid:
        raise RuntimeError(f"ONNX export produced an invalid model: {error}")
