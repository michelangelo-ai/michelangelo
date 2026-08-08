"""Shared PyTorch-to-ONNX export API.

Provides a single :func:`export_torch_to_onnx` entry point that consolidates
all the ONNX export fixes (MHA fastpath disable, dynamo/legacy fallback,
shape normalization, batch expansion, external-data support for models over
the 2 GiB protobuf limit) so that both the model manager packager (non-fused
models) and the model fuser (fused models) produce identical ONNX output
quality.

Private helpers live in
:mod:`michelangelo.lib.model_manager._private.utils.onnx_utils`.
"""

from __future__ import annotations

import inspect
import logging
import os
from typing import TYPE_CHECKING, Any

import numpy as np
import pytorch_lightning as pl
import torch

from michelangelo.lib.model_manager._private.utils.onnx_utils import (
    OnnxTupleWrapper,
    expand_batch_for_onnx_export,
    force_onnx_io_shapes_from_schema,
    onnx_dynamo_dynamic_shapes_for_tuple_arg,
    onnx_dynamo_exporter_dependencies_available,
    run_export_with_retry,
)
from michelangelo.lib.model_manager._private.utils.torch_utils import (
    torch_export_supports_external_data,
)

if TYPE_CHECKING:
    from michelangelo.lib.model_manager.schema import ModelSchema

_logger = logging.getLogger(__name__)

OPSET_VERSION = 14
_DYNAMO_OPSET_VERSION = 18


def export_torch_to_onnx(
    model: torch.nn.Module,
    dest_path: str,
    sample_inputs: tuple[torch.Tensor, ...],
    input_names: list[str],
    output_names: list[str],
    model_schemas: list[ModelSchema | None] | None = None,
    enable_dynamic_batching: bool = True,
    is_lightning_module: bool = False,
    use_tuple_wrapper: bool = False,
    input_key_order: list[str] | None = None,
    external_data: bool = True,
) -> str:
    """Export a PyTorch ``nn.Module`` to ONNX at ``dest_path`` with all critical fixes.

    This is the single entry point for ONNX export shared by:
    - The model manager packager (non-fused models, positional tensor inputs).
    - The model fuser (fused models, dict-input models wrapped in a tuple adapter).

    Fixes applied:
    - MHA transformer fused fastpath disabled during export.
    - Batch dim expanded to >=2 so batch size is not baked into the graph.
    - Dynamo export attempted first when available, with legacy fallback on failure.
    - ONNX IO shapes normalized to match schema static dims (Triton config validation).
    - ``pl.core.module._jit_is_scripting()`` context for LightningModule export.
    - ``external_data=True`` splits weights into sibling files when the graph exceeds
      the 2 GiB protobuf serialization limit (no-op for small models).

    Args:
        model: The PyTorch module to export (already in eval mode).
        dest_path: Local path where the ``.onnx`` file will be saved.
        sample_inputs: Trace tensors in ``input_names`` order (batch dim
            will be expanded).
        input_names: ONNX graph input names (matches Triton config input order).
        output_names: ONNX graph output names (matches Triton config output order).
        model_schemas: Schemas used to force ONNX IO shapes; pass ``None``
            or ``[None]`` to skip.
        enable_dynamic_batching: If True, add dynamic batch axis (dim 0 -> "b").
        is_lightning_module: If True, wrap export in
            ``pl.core.module._jit_is_scripting()``.
        use_tuple_wrapper: If True, wrap ``model`` in ``OnnxTupleWrapper``/
            ``OnnxDynamoTupleWrapper`` so dict-input models receive positional
            (or single-tuple, for dynamo) tensor args for ``torch.onnx.export``.
            ``input_key_order`` must be provided when True.
        input_key_order: Key order for the tuple wrapper (required when
            ``use_tuple_wrapper`` is True).
        external_data: If True (default), pass ``external_data=True`` to
            ``torch.onnx.export`` so weights are split into sibling files
            when the model exceeds the 2 GiB protobuf limit. Small models
            are unaffected (no sidecar files are produced).

    Returns:
        ``dest_path`` (the path where the ONNX model was saved).
    """
    os.makedirs(os.path.dirname(dest_path) or ".", exist_ok=True)

    sample_inputs = expand_batch_for_onnx_export(sample_inputs)

    dynamic_axes: dict[str, dict[int, str]] = {}
    if enable_dynamic_batching:
        for name in list(input_names) + list(output_names):
            dynamic_axes[name] = {0: "b"}

    export_sig = inspect.signature(torch.onnx.export)
    supports_dynamo = "dynamo" in export_sig.parameters
    supports_external_data = torch_export_supports_external_data()
    use_dynamo = supports_dynamo and onnx_dynamo_exporter_dependencies_available()

    wrapped: torch.nn.Module = model
    if use_tuple_wrapper:
        if input_key_order is None:
            raise ValueError("input_key_order is required when use_tuple_wrapper=True")
        wrapped = OnnxTupleWrapper(model, input_key_order)
        wrapped.eval()

    export_kwargs: dict[str, Any] = {
        "input_names": list(input_names),
        "output_names": list(output_names) if output_names else None,
        "do_constant_folding": False,
    }
    if external_data and supports_external_data:
        export_kwargs["external_data"] = True
    if use_dynamo:
        export_kwargs["dynamo"] = True
        export_kwargs["opset_version"] = _DYNAMO_OPSET_VERSION
        if use_tuple_wrapper:
            dynamic_shapes = onnx_dynamo_dynamic_shapes_for_tuple_arg(sample_inputs)
            if dynamic_shapes is not None:
                export_kwargs["dynamic_shapes"] = dynamic_shapes
            else:
                export_kwargs["dynamic_axes"] = dynamic_axes or None
        else:
            export_kwargs["dynamic_axes"] = dynamic_axes or None
    else:
        export_kwargs["dynamic_axes"] = dynamic_axes or None
        export_kwargs["opset_version"] = OPSET_VERSION

    legacy_export_kwargs: dict[str, Any] = {
        "input_names": list(input_names),
        "output_names": list(output_names) if output_names else None,
        "do_constant_folding": False,
        "dynamic_axes": dynamic_axes or None,
        "opset_version": OPSET_VERSION,
    }
    if external_data and supports_external_data:
        legacy_export_kwargs["external_data"] = True
    if supports_dynamo:
        legacy_export_kwargs["dynamo"] = False

    export_args = (
        (wrapped, (sample_inputs,), dest_path)
        if use_tuple_wrapper and use_dynamo
        else (wrapped, sample_inputs, dest_path)
    )

    def _do_export() -> None:
        if is_lightning_module:
            with pl.core.module._jit_is_scripting(), torch.no_grad():
                run_export_with_retry(
                    export_args,
                    export_kwargs,
                    legacy_export_kwargs,
                    use_dynamo,
                    use_tuple_wrapper,
                    model,
                    input_key_order,
                )
        else:
            with torch.no_grad():
                run_export_with_retry(
                    export_args,
                    export_kwargs,
                    legacy_export_kwargs,
                    use_dynamo,
                    use_tuple_wrapper,
                    model,
                    input_key_order,
                )

    _do_export()

    if model_schemas is not None:
        try:
            force_onnx_io_shapes_from_schema(dest_path, model_schemas)
        except Exception as e:
            _logger.warning("Could not normalize ONNX IO shapes from schema: %s", e)

    return dest_path


def prepare_sample_inputs(
    input_names: list[str],
    sample_data: dict[str, Any],
) -> tuple[torch.Tensor, ...]:
    """Build trace tensors from ``sample_data``, ordered by ``input_names``.

    Then expand any size-1 batch dim, matching :func:`export_torch_to_onnx`'s
    own batch expansion.

    Args:
        input_names: Input tensor names in schema order.
        sample_data: Mapping of input name to a ``torch.Tensor`` or ``numpy.ndarray``.

    Returns:
        Trace tensors with batch dimensions expanded for ONNX export.

    Raises:
        ValueError: If ``sample_data`` is missing a required input.
        TypeError: If a sample value is not a tensor or ndarray.
    """
    sample_list: list[torch.Tensor] = []
    for name in input_names:
        if name not in sample_data:
            raise ValueError(f"sample_data missing required input '{name}'")
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
    return expand_batch_for_onnx_export(tuple(sample_list))
