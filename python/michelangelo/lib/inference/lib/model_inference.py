"""Base class for running inference with a model package."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from michelangelo.lib.inference._private.lib.model_inference import (
    _execute_torch,
    _is_custom_python,
    _load_custom_python_model,
    _load_deployable_model,
    _numpy_to_model_input,  # noqa: F401
    _torch_model_accepts_dict_input,  # noqa: F401
)
from michelangelo.lib.inference._private.lib.tensorrt_onnx import _execute_onnx
from michelangelo.lib.model_manager.constants import RawModelType, TritonBackendType

if TYPE_CHECKING:
    import numpy as np

_logger = logging.getLogger(__name__)


class ModelInference:
    """This is a base class for running inference for a model."""

    def __init__(
        self,
        model_path: str | None = None,
        model_type: str | None = None,
        model_name: str | None = None,
    ):
        """Initialize the inference object. Call ``load_model`` before ``execute``.

        Args:
            model_path: The local path to the packaged model.
            model_type: The raw model type of the model.
            model_name: The name of the model, used for logging.

        Raises:
            ValueError: If ``model_path`` is not provided.
        """
        if not model_path:
            raise ValueError("model_path must be provided")

        self.model_name = model_name
        self.model_path = model_path
        self.model_type = model_type
        self.model = None
        self.batch_inference = False
        self.model_schema = None
        self._torch_model_accepts_dict = False
        self.device = None  # Set during load_model() for torch models
        self._onnx_input_dtypes: dict[
            str, np.dtype | None
        ] = {}  # Set during load_model() for ONNX models

    def load_model(self):
        """Load the raw package for custom-python, else the deployable package."""
        # Custom-python loads the raw model; everything else loads the
        # deployable package.
        if _is_custom_python(self):
            self.model_type = RawModelType.CUSTOM_PYTHON
            _load_custom_python_model(self)
        else:
            _load_deployable_model(self)

    def execute(self, data: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Make a prediction from the input data.

        This is the entry point for model inference.

        Args:
            data: Input arrays keyed by column name.

        Returns:
            Output arrays keyed by output name.

        Raises:
            ValueError: If the model is not loaded or its type is not supported.
        """
        if not self.model:
            raise ValueError(
                "Model is not loaded yet. "
                "Please load the model via model_inference.load_model()"
            )

        if self.model_type == RawModelType.CUSTOM_PYTHON:
            return self.model.predict(data)

        if self.model_type in (RawModelType.TORCH, RawModelType.LIGHTNING):
            return self._execute_torch(data)

        if self.model_type == TritonBackendType.ONNX:
            return self._execute_onnx(data)

        raise ValueError(f"Execution for '{self.model_type}' is not supported.")

    # Exposed as class methods so subclasses can call self._is_custom_python() etc.
    _is_custom_python = _is_custom_python
    # Bound as methods rather than called as module-level functions so subclasses can
    # override them. The default `_execute_torch` feeds the model POSITIONALLY in
    # input_schema order; a subclass can override it to feed by KEYWORD. input_schema
    # is sorted alphabetically while a compiled forward() takes all its `List[str]`
    # args first, so a positional call transposes typed arguments and raises
    # "forward() Expected a value of type 'List[str]' ... found Tensor".
    _execute_torch = _execute_torch
    _execute_onnx = _execute_onnx
