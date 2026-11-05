"""Batch model inference building blocks.

``lib.inference`` scores batches of named numpy columns with a pluggable
:class:`ModelBackend`. The built-in :class:`DeployableModelBackend` scores the
deployable package that serving runs (TorchScript, ONNX, or a Python-backend
PyTorch model), and :class:`RawModelBackend` scores a raw model (a PyTorch
module or a custom ``Model``). Any other model runtime plugs in with a factory
that takes a :class:`BackendContext` and returns an object with the same three
members, selected by dotted path through ``OfflineInferenceConfig.backend``.

Most users score datasets through the ``tabular_inference`` workflow task. To
score a Ray dataset directly:

Example:
    >>> from michelangelo.lib.inference import ModelSource, batch_model_inference
    >>> from michelangelo.workflow.schema.inference import OfflineInferenceConfig
    >>> scored = batch_model_inference(  # doctest: +SKIP
    ...     {"train": ray_dataset},
    ...     [(OfflineInferenceConfig(), ModelSource(model_path="/path/to/package"))],
    ... )["train"]
"""

from michelangelo.lib.inference.backend import (
    BackendContext,
    BackendFactory,
    ModelBackend,
    build_backend,
    default_device,
    resolve_backend,
)
from michelangelo.lib.inference.batch_inference import (
    ModelSource,
    batch_model_inference,
)
from michelangelo.lib.inference.deployable_backend import (
    DeployableModelBackend,
    create_deployable_backend,
)
from michelangelo.lib.inference.predictor import (
    MODEL_NAME_COLUMN,
    MODEL_VERSION_COLUMN,
    Predictor,
)
from michelangelo.lib.inference.raw_backend import RawModelBackend, create_raw_backend

__all__ = [
    "MODEL_NAME_COLUMN",
    "MODEL_VERSION_COLUMN",
    "BackendContext",
    "BackendFactory",
    "DeployableModelBackend",
    "ModelBackend",
    "ModelSource",
    "Predictor",
    "RawModelBackend",
    "batch_model_inference",
    "build_backend",
    "create_deployable_backend",
    "create_raw_backend",
    "default_device",
    "resolve_backend",
]
