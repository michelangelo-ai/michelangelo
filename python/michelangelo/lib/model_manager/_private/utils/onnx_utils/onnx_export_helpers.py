"""Private ONNX export helpers shared by the model manager packager and the model fuser.

Contains the MHA fused-fastpath disable, dynamo-with-legacy-fallback,
IO shape normalization from a model schema, and batch-size expansion fixes
that both the non-fused Triton packager
(:mod:`...packager.torch_triton.onnx_conversion`) and the fused-model
exporter (:mod:`michelangelo.lib.shared.utils.model_fuser.fuse`)
need for equivalent ONNX export quality. This module is a private
implementation detail, not a stable contract for callers outside
``model_manager`` — import the public entry point,
:func:`michelangelo.lib.model_manager.utils.onnx.torch_onnx.export_torch_to_onnx`,
instead.
"""

from __future__ import annotations

import importlib.util
import logging
from contextlib import contextmanager
from typing import TYPE_CHECKING, Any

import onnx
import torch

if TYPE_CHECKING:
    from collections.abc import Iterator

    from michelangelo.lib.model_manager.schema import ModelSchema

try:
    from torch.export import Dim as _TorchExportDim
except ImportError:  # pragma: no cover - depends on installed torch version
    _TorchExportDim = None

_logger = logging.getLogger(__name__)


def onnx_dynamo_exporter_dependencies_available() -> bool:
    """Return whether ``onnxscript`` is installed (required for ``dynamo=True``)."""
    return importlib.util.find_spec("onnxscript") is not None


@contextmanager
def disable_transformer_encoder_fastpath_for_onnx(
    root: torch.nn.Module,
) -> Iterator[None]:
    """Disable the MHA fused fast path during ONNX export.

    The fused fast path in ``nn.MultiheadAttention``/``TransformerEncoderLayer``
    produces graphs that don't export cleanly; disabling it for the duration
    of export (via ``torch.backends.mha.set_fastpath_enabled``) avoids that
    without requiring any change to the model itself.

    Args:
        root: Unused; kept for a stable call signature across callers.
    """
    _ = root
    mha = getattr(torch.backends, "mha", None)
    setter = getattr(mha, "set_fastpath_enabled", None) if mha is not None else None
    if setter is None:
        yield
        return
    getter = getattr(mha, "get_fastpath_enabled", None)
    prev = getter() if callable(getter) else True
    setter(False)
    try:
        yield
    finally:
        setter(prev)


def onnx_dynamo_dynamic_shapes_for_tuple_arg(
    tuple_in: tuple[torch.Tensor, ...],
) -> tuple[tuple[dict[int, Any], ...]] | None:
    """Build ``dynamic_shapes`` for ``export(model, (tuple_in,), dynamo=True)``.

    Args:
        tuple_in: The single tuple-of-tensors argument passed to dynamo
            export.

    Returns:
        A one-element tuple wrapping a per-tensor ``{0: Dim("batch")}`` dict,
        or ``None`` if ``torch.export.Dim`` is unavailable on this torch
        version.
    """
    if _TorchExportDim is None:
        return None
    batch = _TorchExportDim("batch")
    return (tuple({0: batch} for _ in tuple_in),)


def onnx_dynamo_export_error_should_retry_legacy(exc: BaseException) -> bool:
    """Return whether a dynamo export failure should retry with ``dynamo=False``."""
    if isinstance(exc, (ImportError, ModuleNotFoundError)):
        return True
    msg = str(exc).lower()
    return (
        "onnxscript" in msg
        or "convertversionpass" in msg
        or "version conversion pass" in msg
        or "model contains functions" in msg
        or "passerror" in msg
        or "failed to convert 'dynamic_axes'" in msg
        or "treespec.unflatten" in msg
        # torch.export's Dim-based dynamic_shapes validation has changed shape
        # across torch releases (e.g. rejecting a Dim keyed by position for a
        # tuple-of-tensors arg on some versions); treat that whole class of
        # torch.export capture failures as dynamo-not-usable-here rather than
        # a genuine model bug, and fall back to the legacy exporter.
        or "torchexporterror" in msg
        or "unexpected dimension" in msg
    )


def expand_batch_for_onnx_export(
    tensors: tuple[torch.Tensor, ...],
) -> tuple[torch.Tensor, ...]:
    """Expand any size-1 batch dimension to size 2 so it isn't baked into the graph."""
    expanded_batch_size = 2
    out: list[torch.Tensor] = []
    for inp in tensors:
        if inp.size(0) > 1:
            out.append(inp)
        else:
            out.append(inp.repeat(expanded_batch_size, *[1] * (inp.dim() - 1)))
    return tuple(out)


def force_onnx_io_shapes_from_schema(
    onnx_path: str,
    model_schemas: list[ModelSchema | None],
    batch_dim_param: str = "b",
) -> None:
    """Override ONNX graph IO shapes to match the schema's static dims.

    PyTorch's ONNX shape inference can drop a fixed non-batch dimension and
    emit ``[-1, -1]``; Triton's onnxruntime backend then rejects the model
    because its config declares a static shape. For every graph input/output
    whose name matches a schema item with a known shape, this overwrites dim
    0 with ``batch_dim_param`` and the remaining dims with the schema's
    static values. Names absent from every schema are left untouched.

    Args:
        onnx_path: Path to the ``.onnx`` file to rewrite in place.
        model_schemas: Schemas to source static shapes from (``None`` entries
            are skipped).
        batch_dim_param: Symbolic name to use for the batch dimension.
    """
    name_to_shape: dict[str, list[int]] = {}
    for schema in model_schemas:
        if schema is None:
            continue
        for item in list(schema.input_schema) + list(schema.output_schema):
            if item.shape is None:
                continue
            name_to_shape[item.name] = [int(s) for s in item.shape]

    if not name_to_shape:
        return

    # load_external_data=False: this path also runs on models whose weights
    # were split into sidecar files (>2 GiB protobuf limit). Loading with
    # external data inlined then writing back via onnx.save()/save_model()
    # re-invokes the external_data_helper, which fails (IsADirectoryError /
    # data re-packing) for a proto whose initializers already carry EXTERNAL
    # references. Loading without external data and serializing the proto
    # verbatim below preserves the EXTERNAL metadata (location/offset/length)
    # so the sidecar files stay valid, while still writing back the shape
    # override.
    model_proto = onnx.load(onnx_path, load_external_data=False)

    def _override(value_info: Any) -> None:
        schema_shape = name_to_shape.get(value_info.name)
        if schema_shape is None:
            return
        tensor_type = value_info.type.tensor_type
        expected_rank = 1 + len(schema_shape)
        existing_rank = len(tensor_type.shape.dim)
        if existing_rank != expected_rank:
            _logger.warning(
                "Skipping ONNX shape override for '%s': rank %d in graph != "
                "%d expected from schema.",
                value_info.name,
                existing_rank,
                expected_rank,
            )
            return
        tensor_type.shape.dim[0].ClearField("dim_value")
        tensor_type.shape.dim[0].dim_param = batch_dim_param
        for i, dim_size in enumerate(schema_shape, start=1):
            tensor_type.shape.dim[i].ClearField("dim_param")
            tensor_type.shape.dim[i].dim_value = dim_size

    for value_info in list(model_proto.graph.input) + list(model_proto.graph.output):
        _override(value_info)

    with open(onnx_path, "wb") as f:
        f.write(model_proto.SerializeToString())


def onnx_export_input_preserver(
    input_tensors: tuple[torch.Tensor, ...],
    *,
    ref_dtype: torch.dtype,
    ref_device: torch.device,
) -> torch.Tensor:
    """Return a scalar zero that data-depends on every tensor in ``input_tensors``.

    Used to keep every ONNX graph input alive even when the traced module
    doesn't otherwise use all of them (the exporter can otherwise prune an
    unused input from the graph).
    """
    acc = torch.zeros((), dtype=ref_dtype, device=ref_device)
    for tensor in input_tensors:
        acc = acc + (tensor * 0).sum().to(dtype=ref_dtype)
    return acc


def onnx_export_attach_inputs_to_output(
    out: object, input_tensors: tuple[torch.Tensor, ...]
) -> object:
    """Add a zero-valued data dependency from each input into one output branch.

    Args:
        out: The module's forward output — a Tensor, NamedTuple, dict, tuple,
            or list.
        input_tensors: The tensors that must remain live graph inputs.

    Returns:
        ``out`` with one branch adjusted to depend on every input tensor.
        Returned unchanged if ``input_tensors`` is empty or ``out``'s shape
        isn't recognized.
    """
    if not input_tensors:
        return out
    if isinstance(out, torch.Tensor):
        preserver = onnx_export_input_preserver(
            input_tensors, ref_dtype=out.dtype, ref_device=out.device
        )
        return out + preserver
    fields = getattr(out, "_fields", None)
    if fields:
        vals = list(out)
        for i, v in enumerate(vals):
            if isinstance(v, torch.Tensor):
                preserver = onnx_export_input_preserver(
                    input_tensors, ref_dtype=v.dtype, ref_device=v.device
                )
                vals[i] = v + preserver
                return type(out)(*vals)
    if isinstance(out, dict):
        d = dict(out)
        for k, v in d.items():
            if isinstance(v, torch.Tensor):
                preserver = onnx_export_input_preserver(
                    input_tensors, ref_dtype=v.dtype, ref_device=v.device
                )
                d[k] = v + preserver
                return d
        return out
    if isinstance(out, (tuple, list)):
        seq = list(out)
        for i, v in enumerate(seq):
            if isinstance(v, torch.Tensor):
                preserver = onnx_export_input_preserver(
                    input_tensors, ref_dtype=v.dtype, ref_device=v.device
                )
                seq[i] = v + preserver
                return tuple(seq) if isinstance(out, tuple) else seq
    return out


class OnnxTupleWrapper(torch.nn.Module):
    """Adapts a dict-input module to positional tensors, for legacy ONNX export.

    ``torch.onnx.export`` (legacy path) traces with positional args; some
    wrapped modules (e.g. a fused transform+predictor model) take a single
    ``dict[str, Tensor]``. This wrapper converts ``*inputs`` (in
    ``input_key_order``) into that dict, calls the inner module, and
    preserves all inputs in the traced graph.
    """

    def __init__(self, inner: torch.nn.Module, input_key_order: list[str]) -> None:
        """Initialize the wrapper.

        Args:
            inner: The module to wrap.
            input_key_order: Feature name for each positional argument, in
                order.
        """
        super().__init__()
        self.inner = inner
        self._input_key_order = input_key_order

    def forward(self, *inputs: torch.Tensor) -> object:
        """Merge positional ``inputs`` into a dict and call the inner module."""
        merged = dict(zip(self._input_key_order, inputs))
        out = self.inner(merged)
        return onnx_export_attach_inputs_to_output(out, inputs)


class OnnxDynamoTupleWrapper(torch.nn.Module):
    """Same as :class:`OnnxTupleWrapper`, for the dynamo ONNX export path.

    ``torch.onnx.export(..., dynamo=True)`` treats ``args=(tuple_in,)`` as a
    single pytree argument (a tuple of tensors), so this wrapper takes one
    tuple argument instead of ``*args``.
    """

    def __init__(self, inner: torch.nn.Module, input_key_order: list[str]) -> None:
        """Initialize the wrapper.

        Args:
            inner: The module to wrap.
            input_key_order: Feature name for each tuple element, in order.
        """
        super().__init__()
        self.inner = inner
        self._input_key_order = input_key_order

    def forward(self, inputs_tuple: tuple[torch.Tensor, ...]) -> object:
        """Merge the tuple argument into a dict and call the inner module."""
        merged = dict(zip(self._input_key_order, inputs_tuple))
        out = self.inner(merged)
        return onnx_export_attach_inputs_to_output(out, inputs_tuple)


def run_export_with_retry(
    export_args: tuple[Any, Any, str],
    export_kwargs: dict[str, Any],
    legacy_export_kwargs: dict[str, Any],
    use_dynamo: bool,
    use_tuple_wrapper: bool,
    model: torch.nn.Module,
    input_key_order: list[str] | None,
) -> None:
    """Run ``torch.onnx.export``, retrying with ``dynamo=False`` on failure.

    Args:
        export_args: ``(model_or_wrapper, sample_args, dest_path)`` for the
            dynamo attempt.
        export_kwargs: Keyword arguments for the dynamo (or sole) export
            attempt.
        legacy_export_kwargs: Keyword arguments for the legacy fallback
            export.
        use_dynamo: Whether to attempt the dynamo exporter first.
        use_tuple_wrapper: Whether inputs are wrapped via
            :class:`OnnxDynamoTupleWrapper`/:class:`OnnxTupleWrapper`.
        model: The unwrapped module, used for the legacy fallback.
        input_key_order: Feature name per positional/tuple input; required
            when ``use_tuple_wrapper`` is ``True``.
    """
    if use_dynamo and "dynamic_shapes" in export_kwargs and use_tuple_wrapper:
        dynamo_wrapped = OnnxDynamoTupleWrapper(model, input_key_order)
        dynamo_wrapped.eval()
        try:
            with disable_transformer_encoder_fastpath_for_onnx(dynamo_wrapped):
                sample_args = export_args[1]
                tuple_arg = (
                    sample_args[0] if isinstance(sample_args, tuple) else sample_args
                )
                torch.onnx.export(
                    dynamo_wrapped,
                    (tuple_arg,),
                    export_args[2],
                    **export_kwargs,
                )
        except Exception as e:
            if not onnx_dynamo_export_error_should_retry_legacy(e):
                raise
            _logger.warning(
                "ONNX dynamo export failed (%s); retrying with legacy "
                "torch.onnx.export.",
                e,
            )
            with disable_transformer_encoder_fastpath_for_onnx(export_args[0]):
                model, sample_args, dest_path = export_args
                torch.onnx.export(model, sample_args, dest_path, **legacy_export_kwargs)
    else:
        with disable_transformer_encoder_fastpath_for_onnx(export_args[0]):
            model, sample_args, dest_path = export_args
            torch.onnx.export(model, sample_args, dest_path, **export_kwargs)
