"""Fuse a PyTorch predictor with a preceding native-transform model.

Both the predictor and the native-transform model are expected to be saved
as ``state_dict`` (or full-module) ``.pt``/``.pth`` files. They are loaded,
wrapped in a :class:`~.fused_model.FusedModel` (transform -> predictor) with
schema-driven input/output merge, and exported as TorchScript, ONNX, or a
combined state dict for Python-backend serving.

Private helpers live in
:mod:`michelangelo.lib.shared.utils.model_fuser._private.fuse` — module
loading and forward-signature helpers used to build the fused model for
tracing. ONNX export itself delegates to the shared
:func:`michelangelo.lib.model_manager.utils.onnx.torch_onnx.export_torch_to_onnx`,
which the non-fused ``model_manager`` Triton packager also uses, so fused and
non-fused models get equivalent ONNX output quality.

Building the Hydra reconstruction spec for a fused *native-transform* model's
Python-backend package (:func:`~._private.fuse._build_tx_hydra_spec`)
requires the native transform package, which has not yet been migrated to
OSS. Until it lands, that one function raises ``NotImplementedError``; every
other function in this module (TorchScript export, ONNX export, field-order
recovery, sample-data merge) works standalone.
"""

from __future__ import annotations

import inspect
import logging
import os
from typing import TYPE_CHECKING, Any

import pytorch_lightning as pl
import torch

from michelangelo.lib.model_manager.utils.onnx.torch_onnx import export_torch_to_onnx

from ._private.fuse import (
    _align_predictor_input_keys,
    _build_fused_model_and_sample,
    _build_fused_sample_input,
    _build_tx_hydra_spec,
    _forward_accepts_dict,
    _forward_param_order,
    _load_module_from_path,
    _schema_input_keys,
)
from .fused_model import FusedModel

if TYPE_CHECKING:
    from michelangelo.lib.model_manager.schema import ModelSchema

__all__ = [
    "build_fused_sample_data",
    "compute_python_fuse_metadata",
    "fuse_models_to_onnx",
    "fuse_models_to_python",
    "fuse_models_to_torchscript",
    "get_predictor_output_field_order",
]

_logger = logging.getLogger(__name__)


def get_predictor_output_field_order(
    model_path: str,
    model_class: str,
    hyperparameters: dict[str, Any],
    model_schema: ModelSchema | None,
) -> list[str] | None:
    """Recover a NamedTuple predictor's output ``_fields`` order.

    Tries two strategies in order:

    1. Inspect ``forward()``'s return-type annotation for a ``_fields``
       attribute. Works without running the model, so it needs no sample
       input.
    2. Run a forward pass with a synthetic sample input and read ``_fields``
       from the live output. Used only when the annotation strategy fails.

    Args:
        model_path: Local path to the predictor model.
        model_class: Dotted class name for the predictor.
        hyperparameters: Constructor kwargs for the predictor.
        model_schema: Predictor model schema, used to build a synthetic
            sample input for strategy 2.

    Returns:
        The output field names in order, or ``None`` if the predictor's
        output isn't a NamedTuple or loading/inference fails. Callers should
        fall back to the original schema order when ``None`` is returned.
    """
    try:
        module = _load_module_from_path(model_path, model_class, hyperparameters)

        try:
            sig = inspect.signature(module.forward)
            return_annotation = sig.return_annotation
            if return_annotation is not inspect.Parameter.empty and hasattr(
                return_annotation, "_fields"
            ):
                _logger.info(
                    "Predictor output field order recovered from forward() "
                    "return annotation."
                )
                return list(return_annotation._fields)
        except Exception:
            pass

        sample_input = _build_fused_sample_input(None, model_schema)
        with torch.no_grad():
            if _forward_accepts_dict(module):
                output = module(sample_input)
            else:
                forward_params = _forward_param_order(module)
                args = [sample_input[p] for p in forward_params if p in sample_input]
                output = module(*args)
        if hasattr(output, "_fields"):
            return list(output._fields)
        _logger.info("Predictor output has no _fields; output_schema order unchanged.")
        return None
    except Exception as e:
        _logger.warning(
            "Could not determine predictor output field order: %s. Output "
            "schema unchanged.",
            e,
        )
        return None


def fuse_models_to_torchscript(
    torch_model_path: str,
    tx_model_path: str,
    model_class: str,
    hyperparameters: dict[str, Any],
    tx_model_class: str,
    tx_hyperparameters: dict[str, Any],
    dest_path: str,
    tx_model_schema: ModelSchema | None = None,
    model_schema: ModelSchema | None = None,
) -> str:
    """Fuse the predictor and transform models and save as TorchScript.

    Loads both models from local paths, composes them as a ``FusedModel``
    with schema-driven merge (transform input/output, predictor input), and
    exports to ``dest_path`` as TorchScript.

    Args:
        torch_model_path: Local path to the predictor model.
        tx_model_path: Local path to the native-transform model.
        model_class: Dotted class name for the predictor.
        hyperparameters: Constructor kwargs for the predictor.
        tx_model_class: Dotted class name for the transform.
        tx_hyperparameters: Constructor kwargs for the transform.
        dest_path: Local path where the fused TorchScript model is saved.
        tx_model_schema: Native-transform model schema.
        model_schema: Predictor model schema.

    Returns:
        ``dest_path``.
    """
    fused, sample_input, _ = _build_fused_model_and_sample(
        torch_model_path,
        tx_model_path,
        model_class,
        hyperparameters,
        tx_model_class,
        tx_hyperparameters,
        tx_model_schema=tx_model_schema,
        model_schema=model_schema,
    )

    os.makedirs(os.path.dirname(dest_path) or ".", exist_ok=True)

    trace_device = next(iter(sample_input.values())).device
    _logger.info("Tracing fused model on device: %s", trace_device)
    # torch.no_grad ensures a consistent eval-mode fast path in
    # nn.TransformerEncoderLayer during trace; without it, trace vs.
    # verification runs can diverge.
    with pl.core.module._jit_is_scripting(), torch.no_grad():
        traced = torch.jit.trace(fused, (sample_input,))
        torch.jit.save(traced, dest_path)
    return dest_path


def fuse_models_to_onnx(
    torch_model_path: str,
    tx_model_path: str,
    model_class: str,
    hyperparameters: dict[str, Any],
    tx_model_class: str,
    tx_hyperparameters: dict[str, Any],
    dest_path: str,
    tx_model_schema: ModelSchema | None = None,
    model_schema: ModelSchema | None = None,
) -> str:
    """Fuse the predictor and transform models and save as ONNX.

    Uses the same composition rules as :func:`fuse_models_to_torchscript`,
    then delegates ONNX export to the shared ``export_torch_to_onnx`` (see
    the module docstring) so fused and non-fused models get equivalent
    export quality.

    Args:
        torch_model_path: Local path to the predictor model.
        tx_model_path: Local path to the native-transform model.
        model_class: Dotted class name for the predictor.
        hyperparameters: Constructor kwargs for the predictor.
        tx_model_class: Dotted class name for the transform.
        tx_hyperparameters: Constructor kwargs for the transform.
        dest_path: Local path where the fused ``.onnx`` model is saved.
        tx_model_schema: Native-transform model schema.
        model_schema: Predictor model schema.

    Returns:
        ``dest_path``.
    """
    fused, sample_input, input_key_order = _build_fused_model_and_sample(
        torch_model_path,
        tx_model_path,
        model_class,
        hyperparameters,
        tx_model_class,
        tx_hyperparameters,
        tx_model_schema=tx_model_schema,
        model_schema=model_schema,
    )

    tuple_in = tuple(sample_input[k] for k in input_key_order)
    input_names = list(input_key_order)
    output_names = [
        item.name for item in (model_schema.output_schema if model_schema else [])
    ]

    try:
        with torch.no_grad():
            out = fused(dict(zip(input_key_order, tuple_in)))
        if hasattr(out, "_fields"):
            output_names = list(out._fields)
    except Exception as e:
        _logger.warning(
            "Could not infer ONNX output names from forward sample run: %s", e
        )

    return export_torch_to_onnx(
        model=fused,
        dest_path=dest_path,
        sample_inputs=tuple_in,
        input_names=input_names,
        output_names=output_names,
        model_schemas=[tx_model_schema, model_schema],
        enable_dynamic_batching=True,
        is_lightning_module=False,
        use_tuple_wrapper=True,
        input_key_order=input_key_order,
    )


def compute_python_fuse_metadata(
    torch_model_path: str,
    model_class: str,
    hyperparameters: dict[str, Any],
    tx_model_schema: ModelSchema | None,
    model_schema: ModelSchema | None,
) -> tuple[list[str], list[str], bool]:
    """Compute fuse routing metadata for Python-backend packaging.

    Loads the predictor to inspect its ``forward()`` signature and derives
    the key lists and predictor call style needed to reconstruct the fused
    model at serve time, using the same key-ordering logic as
    :func:`fuse_models_to_torchscript` but without tracing.

    Args:
        torch_model_path: Local path to the predictor model.
        model_class: Dotted class name for the predictor.
        hyperparameters: Constructor kwargs for the predictor.
        tx_model_schema: Native-transform model schema.
        model_schema: Predictor model schema.

    Returns:
        A tuple ``(transform_input_keys, predictor_input_keys,
        predictor_takes_dict)``: the feature names routed to the transform,
        the feature names routed to the predictor (after merge), and whether
        the predictor's ``forward()`` takes a single dict argument.
    """
    hyperparameters = hyperparameters or {}
    pred_module = _load_module_from_path(torch_model_path, model_class, hyperparameters)
    predictor_takes_dict = _forward_accepts_dict(pred_module)

    transform_input_keys = _schema_input_keys(tx_model_schema)
    predictor_input_keys = _schema_input_keys(model_schema)
    predictor_input_keys = _align_predictor_input_keys(
        pred_module, predictor_input_keys, predictor_takes_dict
    )

    return transform_input_keys, predictor_input_keys, predictor_takes_dict


def fuse_models_to_python(
    torch_model_path: str,
    tx_model_path: str,
    model_class: str,
    hyperparameters: dict[str, Any],
    tx_hyperparameters: dict[str, Any],
    dest_path: str,
    tx_model_schema: ModelSchema | None = None,
    model_schema: ModelSchema | None = None,
) -> tuple[str, str, dict[str, Any]]:
    """Fuse predictor and transform into a combined state dict for serving.

    Unlike :func:`fuse_models_to_torchscript`/:func:`fuse_models_to_onnx`,
    which trace a single ``nn.Module``, this path keeps weights separate and
    reconstructs the ``FusedModel`` at serve time from the returned
    hyperparameters spec. The predictor and transform state dicts are
    combined with submodule prefixes (``predictor_module.*``,
    ``transform_module.*``) matching ``FusedModel``'s attribute names.

    Args:
        torch_model_path: Local path to the predictor model (state_dict or
            full module).
        tx_model_path: Local path to the native-transform model (full
            ``nn.Module``).
        model_class: Dotted class name for the predictor.
        hyperparameters: Constructor kwargs for the predictor.
        tx_hyperparameters: The transform's ``to_dict()`` output.
        dest_path: Local path where the combined state dict is saved.
        tx_model_schema: Native-transform model schema.
        model_schema: Predictor model schema.

    Returns:
        A tuple ``(dest_path, fused_model_class, fused_hyperparameters)``:
        the saved state dict path, ``FusedModel``'s dotted class name, and
        the serve-time reconstruction spec.

    Raises:
        NotImplementedError: Building ``fused_hyperparameters["transform_module"]``
            requires the native-transform package (see
            :func:`~._private.fuse._build_tx_hydra_spec`), which is not yet
            available in OSS.
    """
    hyperparameters = hyperparameters or {}
    tx_hyperparameters = tx_hyperparameters or {}

    transform_input_keys, predictor_input_keys, predictor_takes_dict = (
        compute_python_fuse_metadata(
            torch_model_path,
            model_class,
            hyperparameters,
            tx_model_schema,
            model_schema,
        )
    )

    predictor_sd = torch.load(torch_model_path, map_location="cpu", weights_only=True)
    if "state_dict" in predictor_sd and isinstance(predictor_sd["state_dict"], dict):
        predictor_sd = predictor_sd["state_dict"]

    # weights_only=False: this file holds a full nn.Module (need its
    # architecture to call .state_dict() below, not just tensors), from the
    # same trusted-pipeline storage backend as _load_module_from_path above.
    tx_obj = torch.load(tx_model_path, map_location="cpu", weights_only=False)
    tx_sd = tx_obj.state_dict()
    del tx_obj

    combined_sd = {f"predictor_module.{k}": v for k, v in predictor_sd.items()}
    combined_sd.update({f"transform_module.{k}": v for k, v in tx_sd.items()})

    os.makedirs(os.path.dirname(dest_path) or ".", exist_ok=True)
    torch.save(combined_sd, dest_path)

    tx_spec = _build_tx_hydra_spec(tx_hyperparameters)

    fused_model_class = f"{FusedModel.__module__}.{FusedModel.__qualname__}"
    fused_hyperparameters = {
        "predictor_module": {"_target_": model_class, **hyperparameters},
        "transform_module": tx_spec,
        "transform_input_keys": transform_input_keys,
        "predictor_input_keys": predictor_input_keys,
        "predictor_takes_dict": predictor_takes_dict,
    }

    return dest_path, fused_model_class, fused_hyperparameters


def build_fused_sample_data(
    tx_sample_data: list[dict[str, Any]] | None,
    predictor_sample_data: list[dict[str, Any]] | None,
    fused_input_cols: set[str],
) -> list[dict[str, Any]]:
    """Merge transform + predictor sample data and filter to the fused input schema.

    The predictor is trained on post-transform data, so its sample data
    includes transform output columns. The fused model's input schema only
    has pre-transform columns, so this merges both sample sets and filters
    to the fused input columns.

    Args:
        tx_sample_data: Native-transform model's sample data.
        predictor_sample_data: Predictor model's sample data.
        fused_input_cols: Column names in the fused model's input schema.

    Returns:
        A single-element list containing the merged, filtered sample dict.
        When a key appears in both inputs, ``tx_sample_data`` wins — it holds
        the raw pre-transform values the fused model's input schema expects.
    """
    merged: dict[str, Any] = {}
    for sample in predictor_sample_data or []:
        merged.update(sample)
    for sample in tx_sample_data or []:
        merged.update(sample)
    return [{k: v for k, v in merged.items() if k in fused_input_cols}]
