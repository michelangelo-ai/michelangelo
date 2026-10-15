"""Batch model inference building blocks.

``lib.inference`` scores batches of named numpy columns with a pluggable
:class:`ModelBackend`. The built-in :class:`RawModelBackend` scores raw model
packages (PyTorch, Lightning, fused, or a custom ``Model``); any other model
runtime plugs in by providing a factory that returns an object with the same
three members, selected by dotted path.

Example:
    >>> from michelangelo.lib.inference import BackendContext, build_backend
    >>> from michelangelo.lib.inference import resolve_backend
    >>> factory = resolve_backend("/path/to/raw_package")  # doctest: +SKIP
    >>> backend = build_backend(factory, BackendContext("/path/to/raw_package"))
    ... # doctest: +SKIP
    >>> backend.predict({"x": [[1.0]]})  # doctest: +SKIP
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
from michelangelo.lib.inference.predictor import Predictor
from michelangelo.lib.inference.raw_backend import RawModelBackend, create_raw_backend

__all__ = [
    "BackendContext",
    "BackendFactory",
    "ModelBackend",
    "ModelSource",
    "Predictor",
    "RawModelBackend",
    "batch_model_inference",
    "build_backend",
    "create_raw_backend",
    "default_device",
    "resolve_backend",
]
