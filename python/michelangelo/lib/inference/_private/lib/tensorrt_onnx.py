"""ONNX deployable model loading and execution.

``_load_onnx_deployable_model`` is where a TensorRT engine build would branch off
from the plain ONNX Runtime session.
"""

from __future__ import annotations

import logging
import os

import numpy as np

from michelangelo.lib.constants.sentinel import INT32_SENTINEL

_ONNX_DEPLOYABLE_MODEL_FILE = "model.onnx"

_logger = logging.getLogger(__name__)


def _load_onnx_deployable_model(self) -> None:
    from michelangelo.lib.model_manager.serde.model import (
        load_deployable_model,
        load_deployable_model_schema,
    )
    from michelangelo.lib.model_manager.utils.onnx.data_type import (
        onnx_tensor_type_to_np_dtype,
    )

    onnx_file = os.path.join(self.model_path, "0", _ONNX_DEPLOYABLE_MODEL_FILE)
    if not os.path.exists(onnx_file):
        raise FileNotFoundError(
            f"ONNX model file not found at {onnx_file}. Deployable ONNX models "
            f"should be in 0/{_ONNX_DEPLOYABLE_MODEL_FILE}"
        )

    self.model_schema = load_deployable_model_schema(self.model_path)
    self.batch_inference = True  # ONNX runs on batched inputs

    self.model = load_deployable_model(self.model_path)
    self._onnx_input_dtypes = {
        inp.name: onnx_tensor_type_to_np_dtype(inp.type)
        for inp in self.model.get_inputs()
    }


def _prepare_onnx_input(
    self,
    arr: np.ndarray,
    target_dtype: np.dtype | None,
) -> np.ndarray:
    """Cast to the model's dtype and give ONNX Runtime a C-contiguous, writeable buffer.

    Integer casts map NaN (pyarrow's null representation in ``pyarrow_to_numpy``'s
    uniform-nested fast path) to ``INT32_SENTINEL`` instead of numpy's undefined
    NaN->int cast, which lands on INT64_MIN/INT32_MIN. Exported transform layers
    (e.g. ``PadOrCrop1D``) only recognize ``INT32_SENTINEL`` as the padding marker,
    so any other value at padded slots corrupts downstream transform outputs.
    See ``sentinel_for_numpy_dtype`` in
    ``michelangelo.lib._internal.utils.numpy_utils.sentinel``.

    Args:
        self: The ``ModelInference`` the model belongs to.
        arr: The input array.
        target_dtype: The dtype the model graph declares, or None to keep
            the array's dtype.

    Returns:
        The array to feed to the session.
    """
    if (
        target_dtype is not None
        and arr.dtype != target_dtype
        and np.issubdtype(target_dtype, np.integer)
        and np.issubdtype(arr.dtype, np.floating)
    ):
        arr = np.where(np.isnan(arr), INT32_SENTINEL, arr).astype(arr.dtype, copy=False)
    if target_dtype is not None and arr.dtype != target_dtype:
        arr = arr.astype(target_dtype, copy=False)
    if not arr.flags["C_CONTIGUOUS"]:
        arr = np.ascontiguousarray(arr)
    if not arr.flags.writeable:
        arr = arr.copy()
    return arr


def _execute_onnx(self, data: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
    """Execute inference on an ONNX model via onnxruntime.InferenceSession."""
    feeds: dict[str, np.ndarray] = {}
    for schema_item in self.model_schema.input_schema:
        target_dtype = self._onnx_input_dtypes.get(schema_item.name)
        feeds[schema_item.name] = _prepare_onnx_input(
            self, data[schema_item.name], target_dtype
        )

    output_names = [item.name for item in self.model_schema.output_schema]
    outputs = self.model.run(output_names, feeds)
    return {name: outputs[i] for i, name in enumerate(output_names)}
