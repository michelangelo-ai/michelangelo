"""Model loading and execution helpers bound onto ``ModelInference``."""

from __future__ import annotations

import logging
import os
from typing import TYPE_CHECKING

import numpy as np
import yaml

from michelangelo.lib.inference._private.lib.tensorrt_onnx import (
    _load_onnx_deployable_model,
)
from michelangelo.lib.inference._private.raw_model import inject_model_defs
from michelangelo.lib.model_manager._private.utils.model_utils import (
    infer_deployable_model_package_type,
)
from michelangelo.lib.model_manager.constants import RawModelType, TritonBackendType
from michelangelo.lib.model_manager.schema.data_type import DataType

if TYPE_CHECKING:
    import torch

_logger = logging.getLogger(__name__)


def _numpy_to_model_input(arr: np.ndarray, device: torch.device | None = None):
    """Convert a numpy array to the appropriate model input format.

    Numeric arrays are converted to torch tensors and moved to the specified device.
    String/object arrays are converted to Python lists.

    Args:
        arr: The numpy array to convert.
        device: The torch device to place the tensor on. If None, tensor stays on CPU.

    Returns:
        A list for string and object arrays, otherwise a tensor.
    """
    import torch

    if arr.dtype.kind in ("U", "S", "O"):  # Unicode, byte string, or object
        return arr.tolist()
    tensor = torch.from_numpy(np.ascontiguousarray(arr))
    if device is not None:
        tensor = tensor.to(device)
    return tensor


def _torch_model_accepts_dict_input(model: torch.nn.Module, model_schema) -> bool:
    """Detect whether the model's forward accepts a single ``Dict(str, Tensor)``.

    First tries schema inspection on the TorchScript graph, which is reliable even
    when the model's forward cannot be probe-called (e.g. it requires label columns
    absent from input_schema). Falls back to a probe call for non-TorchScript or
    older models where the TorchScript schema is not accessible.

    Args:
        model: The loaded model.
        model_schema: The model schema, used to build the probe input.

    Returns:
        True if the model's forward takes one dict of named tensors.
    """
    import torch

    from michelangelo.lib.model_manager.utils.torch.data_type import (
        data_type_to_torch_dtype,
    )

    try:
        schema = str(model._c._get_method("forward").schema)
    except Exception:
        pass
    else:
        return "Dict(str, Tensor)" in schema

    # Fallback: probe call
    if not model_schema or not model_schema.input_schema:
        return False
    probe = {}
    for item in model_schema.input_schema:
        shape = list(item.shape) if getattr(item, "shape", None) else [1]
        batch_shape = [1] + [max(1, int(s)) for s in shape]
        if item.data_type == DataType.STRING:
            probe[item.name] = [""] * batch_shape[0]
        else:
            dtype = (
                data_type_to_torch_dtype(item.data_type)
                if item.data_type
                else torch.float32
            )
            probe[item.name] = torch.zeros(batch_shape, dtype=dtype)
    try:
        with torch.no_grad():
            model(probe)
    except (TypeError, RuntimeError):
        # TypeError: Python signature mismatch; RuntimeError: TorchScript
        # "expected Tensor, got dict"
        return False
    else:
        return True


def _load_deployable_model(self) -> None:
    # The deployable package is the source of truth for the backend (torch vs
    # onnx), not the raw training framework, which can differ (e.g. a
    # torch/lightning model compiled to an ONNX deployable).
    self.model_type = infer_deployable_model_package_type(self.model_path)

    if self.model_type == TritonBackendType.ONNX:
        _load_onnx_deployable_model(self)
    elif self.model_type in (RawModelType.TORCH, RawModelType.LIGHTNING):
        _load_torch_deployable_model(self)
    else:
        raise ValueError(
            f"Unsupported deployable backend '{self.model_type}' at {self.model_path}."
        )


def _load_custom_python_model(self) -> None:
    from michelangelo.lib.model_manager.serde.model import (
        load_model_schema,
        load_raw_model,
    )

    inject_model_defs(self.model_path)
    self.model = load_raw_model(self.model_path)
    self.model_schema = load_model_schema(self.model_path)

    # Check if the model expects a batch instead of single record.
    type_yaml = os.path.join(self.model_path, "metadata", "type.yaml")
    if os.path.exists(type_yaml):
        with open(type_yaml) as f:
            content = yaml.safe_load(f)
            self.batch_inference = content.get("batch_inference", False)


def _load_torch_deployable_model(self) -> None:
    import torch

    from michelangelo.lib.model_manager.serde.model import (
        load_deployable_model,
        load_deployable_model_schema,
    )

    self.model = load_deployable_model(self.model_path)
    self.model_schema = load_deployable_model_schema(self.model_path)

    self.batch_inference = True  # PyTorch always uses batch inference
    self._torch_model_accepts_dict = _torch_model_accepts_dict_input(
        self.model, self.model_schema
    )

    # Move model to GPU if available
    self.device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    if self.device.type == "cuda":
        # The fuser emits invalid float literals (e.g. `1.0e+20.f`) for large
        # clamp constants, causing NVRTC to fail. Fall back to pre-compiled
        # ATen kernels.
        torch.jit.set_fusion_strategy([("STATIC", 0), ("DYNAMIC", 0)])
    self.model = self.model.to(self.device)


def _is_custom_python(self) -> bool:
    """Decide whether to load the raw custom-python model, not a deployable package.

    Custom-python is the only inference path that uses the raw model. Everything
    else runs on the deployable package, whose backend is inferred from the
    package. This relies on the explicit model type or the local package layout.

    Returns:
        True if the raw custom-python model should be loaded.
    """
    from michelangelo.lib.model_manager.serde.model import get_raw_model_type

    # An explicit type from the caller takes precedence.
    # Any explicit non-custom-python type means "not raw" (the package decides the
    # backend).
    if self.model_type:
        return self.model_type == RawModelType.CUSTOM_PYTHON

    # A deployable package (config.pbtxt) is never a raw custom-python model.
    if os.path.exists(os.path.join(self.model_path, "config.pbtxt")):
        return False
    # Raw model (has metadata/type.yaml).
    if os.path.exists(os.path.join(self.model_path, "metadata", "type.yaml")):
        return get_raw_model_type(self.model_path) == RawModelType.CUSTOM_PYTHON
    return False


def _execute_torch(self, data: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
    """Execute inference on TorchScript model with numpy-tensor conversion."""
    import torch

    if self._torch_model_accepts_dict:
        # Model expects a single dict argument
        model_inputs = {
            schema_item.name: _numpy_to_model_input(data[schema_item.name], self.device)
            for schema_item in self.model_schema.input_schema
        }
        with torch.no_grad():
            outputs = self.model(model_inputs)
    else:
        # Model expects positional arguments in schema order
        model_inputs = [
            _numpy_to_model_input(data[schema_item.name], self.device)
            for schema_item in self.model_schema.input_schema
        ]
        with torch.no_grad():
            if len(model_inputs) == 1:
                outputs = self.model(model_inputs[0])
            else:
                outputs = self.model(*model_inputs)

    # Convert tensors to numpy
    result = {}
    if isinstance(outputs, torch.Tensor):
        outputs = (outputs,)
    if hasattr(outputs, "_fields"):
        # NamedTuple output (scripted model): map by field name so
        # output_schema order doesn't matter
        for schema_item in self.model_schema.output_schema:
            result[schema_item.name] = (
                getattr(outputs, schema_item.name).detach().cpu().numpy()
            )
    else:
        # Plain tuple output (traced model): output_schema order is guaranteed to match
        # NamedTuple field order by the packager.
        for i, schema_item in enumerate(self.model_schema.output_schema):
            result[schema_item.name] = outputs[i].detach().cpu().numpy()
    return result
