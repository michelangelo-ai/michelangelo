"""ModelVariable — trained-model variable for ML workflow tasks.

Subclasses ``Variable``, wraps a storage path and a transient in-memory value,
and dispatches save/load to format-specific handlers based on
``metadata.training_framework``.

Three frameworks are supported as first-class citizens:

- ``"custom"`` — user-defined ``Model`` subclasses (``CustomModel.save`` /
  ``CustomModel.load``) from
  ``michelangelo.lib.model_manager.interface.custom_model``. Artifact layout
  is a directory tree controlled by the user's ``save()`` implementation.
- ``"pytorch"`` — generic ``torch.nn.Module`` via ``torch.save`` /
  ``torch.load``. The full ``nn.Module`` object is pickled so fused models
  with nested submodules (and any non-trivial constructor signature) round
  trip without requiring callers to re-supply constructor arguments.
  ``ModelVariable`` is for intra-workflow scratch storage of task
  intermediates within the same trusted pipeline run — for long-term
  registry artifacts, prefer the ``state_dict`` + ``ModelMetadata``
  pattern (see ``save_lightning_model``).
- ``"lightning"`` — ``pytorch_lightning.LightningModule`` via the
  ``state_dict`` round-trip; ``metadata.model_class`` and
  ``metadata.hyperparameters`` drive reconstruction.

``_private/`` convention:
    This file lives in ``_private/`` — do not import directly from this path.
    Import ``ModelVariable`` from ``michelangelo.workflow.variables`` instead.
"""

from __future__ import annotations

import logging
import os
import tempfile
from typing import Any

import fsspec

from michelangelo.uniflow.core.utils import dot_path, import_attribute
from michelangelo.workflow.variables._private.base import Variable
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_CUSTOM,
    TRAINING_FRAMEWORK_LIGHTNING,
    TRAINING_FRAMEWORK_PYTORCH,
    ModelMetadata,
)

_logger = logging.getLogger(__name__)


class ModelVariable(Variable):
    """A model variable flowing between workflow tasks.

    Subclasses ``Variable``. Underlying it could be a PyTorch model, a
    Lightning module, or a user-defined ``CustomModel``. Persistence is
    delegated to framework-specific handlers; dispatch is keyed on
    ``metadata.training_framework``.

    Example:
        >>> import torch  # doctest: +SKIP
        >>> model = torch.nn.Linear(2, 1)  # doctest: +SKIP
        >>> var = ModelVariable.create(model)  # doctest: +SKIP
        >>> var.metadata.training_framework  # doctest: +SKIP
        'pytorch'
        >>> var.save()  # doctest: +SKIP
        >>> restored = ModelVariable(path=var.path, metadata=var.metadata)
        >>> isinstance(restored.value, torch.nn.Linear)  # doctest: +SKIP
        True
    """

    def __init__(
        self,
        value: Any = None,
        path: str | None = None,
        metadata: Any = None,
        _io_metadata: Any = None,
    ) -> None:
        """Initialise with an optional in-memory value and/or storage path.

        Args:
            value: The in-memory model. When provided, ``save()`` persists it
                to ``path``. When ``None``, accessing ``value`` triggers a
                lazy load.
            path: Storage path (local or fsspec URL). Auto-generated from the
                ``UF_STORAGE_URL`` env var (default ``memory://storage``)
                when not provided.
            metadata: Optional ``ModelMetadata`` instance. Defaults to an
                empty ``ModelMetadata()`` so that ``save()`` / ``_load()``
                can raise a clear ``ValueError`` instead of an
                ``AttributeError`` when ``training_framework`` is unset.
            _io_metadata: Internal metadata written by the IO layer after a
                save. Passed through by the UniFlow codec when reconstructing
                a ``ModelVariable`` from a task result.
        """
        import uuid

        if path is None:
            path = (
                f"{os.environ.get('UF_STORAGE_URL', 'memory://storage')}/"
                f"{uuid.uuid4().hex}"
            )
        if metadata is None:
            metadata = ModelMetadata()
        super().__init__(path=path, metadata=metadata, _io_metadata=_io_metadata)
        self._value = value

    @classmethod
    def create(cls, value: Any) -> ModelVariable:
        """Create a ``ModelVariable`` and auto-detect the training framework.

        Detection order — first match wins:

        1. ``CustomModel`` (the ``Model`` ABC from
           ``michelangelo.lib.model_manager.interface.custom_model``)
           → ``training_framework="custom"``.
        2. ``pytorch_lightning.LightningModule`` →
           ``training_framework="lightning"``. Checked before plain PyTorch
           because ``LightningModule`` subclasses ``torch.nn.Module``.
        3. ``torch.nn.Module`` → ``training_framework="pytorch"``.

        Any other type leaves ``training_framework`` unset; callers must
        populate ``metadata`` manually before calling ``save()``.

        Args:
            value: The in-memory model. ``CustomModel``, ``LightningModule``,
                and ``torch.nn.Module`` instances are recognised
                automatically; other framework types require manual
                ``metadata.training_framework`` setup.

        Returns:
            A new ``ModelVariable`` with ``value`` ready in memory and
            ``metadata`` populated with the detected framework +
            ``model_class``.
        """
        res = super().create(value)
        res.metadata = ModelMetadata()

        try:
            from michelangelo.lib.model_manager.interface.custom_model import (
                Model as CustomModel,
            )

            if isinstance(value, CustomModel):
                res.metadata.training_framework = TRAINING_FRAMEWORK_CUSTOM
                res.metadata.model_class = dot_path(type(value))
                return res
        except ImportError:
            pass

        try:
            import pytorch_lightning as pl

            if isinstance(value, pl.LightningModule):
                res.metadata.training_framework = TRAINING_FRAMEWORK_LIGHTNING
                res.metadata.model_class = dot_path(type(value))
                return res
        except ImportError:
            pass

        try:
            import torch

            if isinstance(value, torch.nn.Module):
                res.metadata.training_framework = TRAINING_FRAMEWORK_PYTORCH
                res.metadata.model_class = dot_path(type(value))
                return res
        except ImportError:
            pass

        return res

    def _load(self):
        """Load value from variable path, dispatched on training_framework.

        Raises:
            ValueError: If ``metadata.training_framework`` is unset or
                unrecognised. Call the framework-specific ``load_*`` method
                directly when auto-dispatch is not possible.
        """
        if self.metadata.training_framework == TRAINING_FRAMEWORK_CUSTOM:
            self.load_custom_model()
        elif self.metadata.training_framework == TRAINING_FRAMEWORK_PYTORCH:
            self.load_torch_model()
        elif self.metadata.training_framework == TRAINING_FRAMEWORK_LIGHTNING:
            self.load_lightning_model()
        else:
            raise ValueError(self._unknown_framework_message())

    def save(self):
        """Save value to variable path, dispatched on training_framework.

        Raises:
            ValueError: If no value has been set on this variable, or if
                ``metadata.training_framework`` is unset or unrecognised.
                Call the framework-specific ``save_*`` method directly when
                auto-dispatch is not possible.
        """
        if self._value is None:
            raise ValueError("Cannot save: no value has been set on this variable.")
        if self.metadata.training_framework == TRAINING_FRAMEWORK_CUSTOM:
            self.save_custom_model()
        elif self.metadata.training_framework == TRAINING_FRAMEWORK_PYTORCH:
            self.save_torch_model()
        elif self.metadata.training_framework == TRAINING_FRAMEWORK_LIGHTNING:
            self.save_lightning_model()
        else:
            raise ValueError(self._unknown_framework_message())

    def _unknown_framework_message(self) -> str:
        """Build an actionable error message for an unset/unknown framework."""
        return (
            f"Unrecognized training framework: "
            f"{self.metadata.training_framework!r}. "
            f"Set metadata.training_framework to one of "
            f"{TRAINING_FRAMEWORK_CUSTOM!r}, {TRAINING_FRAMEWORK_PYTORCH!r}, "
            f"or {TRAINING_FRAMEWORK_LIGHTNING!r} "
            f"(see michelangelo.workflow.variables.metadata), or construct "
            f"the variable via ModelVariable.create(value) which auto-detects."
        )

    # ------------------------------------------------------------------
    # Custom model
    # ------------------------------------------------------------------

    def save_custom_model(self):
        """Save a ``CustomModel`` instance via its ``.save(path)`` method.

        Materialises the model into a temporary directory and uploads the
        directory tree to ``self.path`` via fsspec. No-ops when the value has
        already been saved.
        """
        _logger.info("Saving custom model for %s", self.path)

        if self._saved:
            _logger.info(
                "Custom model value already saved for %s. Skipping saving.", self.path
            )
            return

        fs, path = fsspec.core.url_to_fs(self.path)
        with tempfile.TemporaryDirectory() as temp_dir:
            self._value.save(temp_dir)
            fs.put(temp_dir, path, recursive=True)

        self._saved = True

    def load_custom_model(self):
        """Load a ``CustomModel`` via its ``.load(path)`` classmethod.

        Imports the model class from ``metadata.model_class``, downloads the
        artifact tree from ``self.path`` to a temporary directory, and calls
        ``model_class.load(temp_path)``.

        Raises:
            ValueError: If ``metadata.model_class`` is not set.
        """
        _logger.info("Loading custom model from %s", self.path)

        if not self.metadata.model_class:
            raise ValueError(
                "metadata.model_class must be set to load a custom model "
                "(e.g. var.metadata.model_class = 'mypackage.models.MyModel'). "
                "ModelVariable.create(value) sets it automatically."
            )

        model_class = import_attribute(self.metadata.model_class)

        fs, path = fsspec.core.url_to_fs(self.path)
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            fs.get(path, model_path, recursive=True)
            self._value = model_class.load(model_path)
        self._saved = True

    # ------------------------------------------------------------------
    # PyTorch model
    # ------------------------------------------------------------------

    def save_torch_model(self):
        """Save a ``torch.nn.Module`` to ``self.path`` via ``torch.save``.

        Persists the full ``nn.Module`` object — including any nested
        submodules and bound state that a state_dict alone could not
        capture. ``ModelVariable`` is intended for intra-workflow scratch
        storage of task intermediates (not for long-term registry
        artifacts), so the simpler full-pickle pattern is preferred over a
        state_dict + class-reconstruction step that cannot represent fused
        models with non-trivial constructors. Lightning modules, which have
        a canonical ``state_dict`` + class re-instantiation path, go
        through ``save_lightning_model`` / ``load_lightning_model``.
        """
        import torch

        _logger.info("Saving PyTorch model for %s", self.path)

        if self._saved:
            _logger.info(
                "PyTorch model value already saved for %s. Skipping saving.", self.path
            )
            return

        fs, path = fsspec.core.url_to_fs(self.path)
        with tempfile.TemporaryDirectory() as temp_dir:
            model_file = os.path.join(temp_dir, "model.pt")
            torch.save(self._value, model_file)
            fs.put(model_file, path)

        self._saved = True

    def load_torch_model(
        self,
        weights_only: bool = False,
        map_location: Any = "cpu",
    ):
        """Load a ``torch.nn.Module`` from ``self.path`` via ``torch.load``.

        Args:
            weights_only: Forwarded to ``torch.load``. Defaults to ``False``
                because ``save_torch_model`` pickles the full
                ``nn.Module``; loading the artifact this class wrote
                requires the unsafe mode. Pass ``weights_only=True`` only
                when you know the artifact at ``self.path`` is a
                state_dict-only file written by an external producer.
            map_location: Forwarded to ``torch.load``. Defaults to ``"cpu"``
                so that an artifact pickled from a CUDA device on the
                producer host always loads on consumers without matching
                GPU topology (otherwise ``torch.load`` raises
                ``RuntimeError`` if the recorded CUDA device is not
                available). Pass ``None`` to use ``torch.load``'s default
                (restore tensors to their original device), a device
                string such as ``"cuda:0"`` to pin to a specific device,
                a ``torch.device``, a ``{src: dst}`` device-map dict, or
                a callable — anything ``torch.load`` accepts.

        Security:
            ``weights_only=False`` permits arbitrary code execution from
            the pickle stream. ``ModelVariable`` is designed for
            intra-workflow scratch storage where the producer and consumer
            are part of the same trusted pipeline run; do not load
            artifacts from untrusted sources without setting
            ``weights_only=True``.
        """
        import torch

        _logger.info(
            "Loading PyTorch model from %s (weights_only=%s, map_location=%s)",
            self.path,
            weights_only,
            map_location,
        )

        fs, path = fsspec.core.url_to_fs(self.path)
        with tempfile.TemporaryDirectory() as temp_dir:
            model_file = os.path.join(temp_dir, "model.pt")
            fs.get(path, model_file)
            self._value = torch.load(
                model_file,
                map_location=map_location,
                weights_only=weights_only,
            )
        self._saved = True

    # ------------------------------------------------------------------
    # PyTorch Lightning model
    # ------------------------------------------------------------------

    def save_lightning_model(self):
        """Save a Lightning module's ``state_dict`` via ``torch.save``.

        Stores only the ``state_dict``; the model class itself is not
        serialised. Loading requires ``metadata.model_class`` to be set so
        the class can be re-instantiated.
        """
        import torch

        _logger.info("Saving Lightning model for %s", self.path)

        if self._saved:
            _logger.info(
                "Lightning model value already saved for %s. Skipping saving.",
                self.path,
            )
            return

        if not self.metadata.model_class:
            self.metadata.model_class = dot_path(type(self._value))

        fs, path = fsspec.core.url_to_fs(self.path)
        with tempfile.TemporaryDirectory() as temp_dir:
            model_file = os.path.join(temp_dir, "model.pt")
            torch.save(self._value.state_dict(), model_file)
            fs.put(model_file, path)

        self._saved = True

    def load_lightning_model(self, map_location: Any = "cpu"):
        """Load a Lightning module by re-instantiating ``model_class``.

        Steps:

        1. Import the class from ``metadata.model_class``.
        2. Construct an instance via
           ``model_class(**metadata.hyperparameters)`` (empty dict when
           ``hyperparameters`` is ``None``).
        3. Download the ``state_dict`` from ``self.path`` and apply
           ``load_state_dict`` (``weights_only=True`` — only tensors are
           unpickled, so the call is safe against malicious artifacts).
        4. Call ``model.eval()`` and store as ``self._value``.

        Args:
            map_location: Forwarded to ``torch.load``. Defaults to ``"cpu"``
                so artifacts saved on a CUDA device load on consumers
                without matching GPU topology. Accepts the same value
                shapes as ``torch.load`` (device string, ``torch.device``,
                ``{src: dst}`` device-map dict, callable, or ``None``).

        Raises:
            ValueError: If ``metadata.model_class`` is not set, or if the
                class constructor requires arguments not supplied via
                ``metadata.hyperparameters``.
        """
        if not self.metadata.model_class:
            raise ValueError(
                "metadata.model_class must be set to load a Lightning model "
                "(ModelVariable.create(value) sets it automatically for "
                "pytorch_lightning.LightningModule instances)."
            )

        import torch

        _logger.info(
            "Loading Lightning model from %s (map_location=%s)",
            self.path,
            map_location,
        )

        model_class = import_attribute(self.metadata.model_class)
        hyperparameters = self.metadata.hyperparameters or {}
        try:
            model = model_class(**hyperparameters)
        except TypeError as e:
            raise ValueError(
                f"Cannot reconstruct {self.metadata.model_class}: {e}. "
                "Set metadata.hyperparameters before save() with the kwargs "
                "required by the model constructor."
            ) from e

        fs, path = fsspec.core.url_to_fs(self.path)
        with tempfile.TemporaryDirectory() as temp_dir:
            model_file = os.path.join(temp_dir, "model.pt")
            fs.get(path, model_file)
            state_dict = torch.load(
                model_file, map_location=map_location, weights_only=True
            )
            model.load_state_dict(state_dict)

        model.eval()
        self._value = model
        self._saved = True
