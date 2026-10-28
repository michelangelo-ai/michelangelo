"""The ``ModelBackend`` contract and backend resolution.

A backend scores one batch of named numpy columns. It is a
:class:`typing.Protocol` instead of an abstract base class, unlike
``StorageBackend``: a storage backend shares behaviour (retries, path handling)
that subclasses inherit, whereas a model backend has no behaviour to share, so
an implementation never needs to import this package to satisfy the contract.
This mirrors how ``collate_fn`` and similar hooks are configured, by a dotted
path to a callable.

Loading happens in a *factory* that receives a :class:`BackendContext` and runs
inside the worker that scores batches, so heavy model objects never have to be
serialized from the driver.
"""

from __future__ import annotations

import importlib
import os
from dataclasses import dataclass
from typing import TYPE_CHECKING, Callable, Protocol, runtime_checkable

if TYPE_CHECKING:
    import numpy as np

    from michelangelo.lib.model_manager.schema import ModelSchema

__all__ = [
    "BackendContext",
    "BackendFactory",
    "ModelBackend",
    "build_backend",
    "default_device",
    "resolve_backend",
]

_PROTOCOL_MEMBERS = ("input_names", "output_names", "predict")


@runtime_checkable
class ModelBackend(Protocol):
    """Scores a batch of named numpy columns.

    ``isinstance`` checks against this protocol only verify that the members
    exist, not their signatures.

    Example:
        >>> class Doubler:
        ...     input_names = ["x"]
        ...     output_names = ["y"]
        ...
        ...     def predict(self, inputs):
        ...         return {"y": inputs["x"] * 2}
        >>> isinstance(Doubler(), ModelBackend)
        True
    """

    @property
    def input_names(self) -> list[str] | None:
        """Names of the columns the model reads, or ``None`` if unknown."""
        ...

    @property
    def output_names(self) -> list[str] | None:
        """Names of the columns ``predict`` returns, or ``None`` if unknown."""
        ...

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Score one batch.

        Args:
            inputs: Column name to array, each with the batch on axis 0. The
                arrays may be read-only views of Arrow memory, so copy before
                modifying one in place.

        Returns:
            Output column name to array, each with the batch on axis 0.
        """
        ...


@dataclass(frozen=True)
class BackendContext:
    """Everything a backend factory needs to load a model.

    Attributes:
        model_path: Local directory of the model package.
        schema: The model schema, when known.
        device: Device the model should run on, such as ``"cpu"`` or
            ``"cuda"``.
    """

    model_path: str
    schema: ModelSchema | None = None
    device: str = "cpu"


#: Builds a backend from a :class:`BackendContext`. Runs inside the worker.
BackendFactory = Callable[[BackendContext], ModelBackend]


def default_device() -> str:
    """Pick the device for the current process.

    Returns:
        ``"cuda"`` if PyTorch is installed and sees a GPU, otherwise ``"cpu"``.
    """
    try:
        import torch
    except ImportError:
        return "cpu"
    return "cuda" if torch.cuda.is_available() else "cpu"


def _import_factory(dotted_path: str) -> BackendFactory:
    """Import a backend factory from ``"package.module.attribute"``."""
    module_path, _, attr = dotted_path.rpartition(".")
    if not module_path:
        raise ValueError(
            f"Backend {dotted_path!r} must be a dotted path such as "
            "'package.module.factory'."
        )
    try:
        factory = getattr(importlib.import_module(module_path), attr)
    except (ImportError, AttributeError) as e:
        raise ValueError(f"Could not import backend {dotted_path!r}: {e}") from e
    if not callable(factory):
        raise ValueError(f"Backend {dotted_path!r} is not callable.")
    return factory


def resolve_backend(model_path: str, backend: str | None = None) -> BackendFactory:
    """Choose the backend factory for a model package.

    Args:
        model_path: Local directory of the model package.
        backend: Dotted path of a :data:`BackendFactory`. When ``None``, the
            backend is chosen from the package layout.

    Returns:
        A callable that builds the backend from a :class:`BackendContext`.

    Raises:
        ValueError: If ``backend`` cannot be imported or is not callable, or
            if the package is a deployable (serving) package, which has no
            backend yet.
        FileNotFoundError: If ``backend`` is ``None`` and ``model_path`` is not
            a model package.
    """
    if backend is not None:
        return _import_factory(backend)

    if os.path.exists(os.path.join(model_path, "metadata", "type.yaml")):
        from michelangelo.lib.inference.raw_backend import create_raw_backend

        return create_raw_backend

    if any(
        os.path.exists(os.path.join(model_path, "0", name))
        for name in ("model.onnx", "model.pt")
    ):
        raise ValueError(
            f"{model_path!r} is a deployable model package, which the built-in "
            "backends cannot score. Pass the raw model package, or set "
            "'backend' to a factory that can load it."
        )

    raise FileNotFoundError(
        f"{model_path!r} is not a model package: metadata/type.yaml not found."
    )


def build_backend(factory: BackendFactory, context: BackendContext) -> ModelBackend:
    """Build a backend and check that it satisfies the protocol.

    Args:
        factory: The factory returned by :func:`resolve_backend`.
        context: Where and how to load the model.

    Returns:
        The backend.

    Raises:
        ValueError: If the factory's result is missing protocol members.
    """
    instance = factory(context)
    if not isinstance(instance, ModelBackend):
        missing = [m for m in _PROTOCOL_MEMBERS if not hasattr(instance, m)]
        raise ValueError(
            f"Backend factory returned {type(instance).__name__}, which is "
            f"missing: {', '.join(missing)}. A ModelBackend needs "
            f"{', '.join(_PROTOCOL_MEMBERS)}."
        )
    return instance
