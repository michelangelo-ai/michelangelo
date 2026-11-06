"""Backend that scores a deployable (serving) model package.

A deployable package is what serving runs: a Triton model directory with a
``config.pbtxt`` and a ``0/`` version directory holding a TorchScript module,
an ONNX graph, or a Python-backend PyTorch model. Scoring it offline means
batch predictions come from the same artifact as online ones, including a
native transform fused into the graph.

Custom models are not scored from a deployable package: their package does not
record whether ``predict`` takes a whole batch or one record, so they are
scored from the raw package with :class:`~michelangelo.lib.inference.RawModelBackend`.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

import numpy as np

from michelangelo.lib.constants.sentinel import INT32_SENTINEL
from michelangelo.lib.inference._invoke import (
    call_module,
    call_torchscript,
    name_outputs,
    to_torch_inputs,
)

if TYPE_CHECKING:
    from michelangelo.lib.inference.backend import BackendContext
    from michelangelo.lib.model_manager.schema import ModelSchema

__all__ = ["DeployableModelBackend", "create_deployable_backend"]

_ONNX_DTYPES = {
    "tensor(bool)": np.bool_,
    "tensor(int8)": np.int8,
    "tensor(int16)": np.int16,
    "tensor(int32)": np.int32,
    "tensor(int64)": np.int64,
    "tensor(uint8)": np.uint8,
    "tensor(float16)": np.float16,
    "tensor(float)": np.float32,
    "tensor(double)": np.float64,
    "tensor(string)": np.object_,
}


class DeployableModelBackend:
    """Scores batches with a model loaded from a deployable package.

    Args:
        model: The result of ``load_deployable_model``: a TorchScript module,
            an ONNX Runtime session, or a PyTorch module.
        schema: The model schema. Provides the input and output names, and the
            positional order of TorchScript arguments.
        device: Device a PyTorch model and its inputs are moved to. The loader
            loads PyTorch models on the CPU.

    Raises:
        ValueError: If ``model`` is a custom ``Model``, which is scored from
            its raw package instead.
        TypeError: If ``model`` is none of the supported kinds.
    """

    def __init__(
        self,
        model: Any,
        schema: ModelSchema | None = None,
        device: str = "cpu",
    ) -> None:
        """Wrap ``model``, moving a PyTorch model to ``device``."""
        self._kind = _kind_of(model)
        if self._kind in ("torchscript", "module"):
            model = model.to(device)
            if self._kind == "torchscript" and device.startswith("cuda"):
                import torch

                # The TorchScript fuser can generate invalid CUDA code for
                # large clamp constants, so use the precompiled kernels.
                torch.jit.set_fusion_strategy([("STATIC", 0), ("DYNAMIC", 0)])
        self._model = model
        self._schema = schema
        self._device = device

    @property
    def input_names(self) -> list[str] | None:
        """Input column names from the schema, or ``None`` without one."""
        if self._schema is None or not self._schema.input_schema:
            return None
        return [item.name for item in self._schema.input_schema]

    @property
    def output_names(self) -> list[str] | None:
        """Output column names from the schema, or ``None`` without one."""
        if self._schema is None or not self._schema.output_schema:
            return None
        return [item.name for item in self._schema.output_schema]

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Score one batch.

        Args:
            inputs: Column name to array, each with the batch on axis 0.

        Returns:
            Output column name to array.
        """
        if self._kind == "onnx":
            return self._predict_onnx(inputs)

        import torch

        tensors = to_torch_inputs(inputs, self._device)
        with torch.inference_mode():
            if self._kind == "torchscript":
                output = call_torchscript(
                    self._model, tensors, self.input_names or list(inputs)
                )
            else:
                output = call_module(self._model, tensors)
        return name_outputs(output, self.output_names)

    def _predict_onnx(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Run the ONNX Runtime session on the inputs the graph declares."""
        feeds = {
            meta.name: _onnx_input(inputs[meta.name], _ONNX_DTYPES.get(meta.type))
            for meta in self._model.get_inputs()
        }
        names = [meta.name for meta in self._model.get_outputs()]
        return dict(zip(names, self._model.run(names, feeds)))


def _kind_of(model: Any) -> str:
    """Classify a loaded model by how it is called."""
    from michelangelo.lib.model_manager.interface.custom_model import Model

    if isinstance(model, Model):
        raise ValueError(
            "Custom models are not scored from a deployable package, which does "
            "not record whether predict takes a batch or one record. Score the "
            "model's raw package instead."
        )
    if hasattr(model, "get_inputs") and hasattr(model, "run"):
        return "onnx"

    import torch

    if isinstance(model, torch.jit.ScriptModule):
        return "torchscript"
    if isinstance(model, torch.nn.Module):
        return "module"
    raise TypeError(
        f"DeployableModelBackend cannot score a {type(model).__name__}. It "
        "takes a TorchScript module, an ONNX Runtime session or a torch.nn.Module."
    )


def _onnx_input(arr: np.ndarray, dtype: type | None) -> np.ndarray:
    """Cast a column to the dtype the ONNX graph declares.

    Integer inputs get ``INT32_SENTINEL`` where the column holds NaN, which is
    how a null in a numeric list column arrives, instead of numpy's undefined
    NaN-to-integer cast. Exported transform layers only recognize that sentinel
    as padding. String inputs are decoded to ``str``.
    """
    if dtype is np.object_:
        flat = [x.decode("utf-8") if isinstance(x, bytes) else x for x in arr.ravel()]
        return np.array(flat, dtype=object).reshape(arr.shape)
    if dtype is None or arr.dtype == dtype:
        return np.ascontiguousarray(arr)
    if np.issubdtype(dtype, np.integer) and np.issubdtype(arr.dtype, np.floating):
        arr = np.where(np.isnan(arr), INT32_SENTINEL, arr)
    return np.ascontiguousarray(arr.astype(dtype))


def create_deployable_backend(context: BackendContext) -> DeployableModelBackend:
    """Load a deployable package and wrap it in a :class:`DeployableModelBackend`.

    Args:
        context: The package path, schema and device. When ``context.schema`` is
            ``None``, the schema stored in the package is used.

    Returns:
        The backend.

    Raises:
        ValueError: If the package holds a custom model, which is scored from
            its raw package instead.
    """
    from michelangelo.lib.model_manager.serde.model import (
        load_deployable_model,
        load_model_schema,
    )

    schema = context.schema or load_model_schema(context.model_path)
    model = load_deployable_model(context.model_path)
    return DeployableModelBackend(model, schema=schema, device=context.device)
