"""ModelVariable — trained-model variable for ML workflow tasks.

Subclasses ``Variable``, wraps a storage path and a transient in-memory value,
and dispatches save/load to format-specific handlers based on
``metadata.training_framework``.

Three frameworks are supported as first-class citizens:

- ``"custom"`` — user-defined ``Model`` subclasses (``CustomModel.save`` /
  ``CustomModel.load``) from
  ``michelangelo.lib.model_manager.interface.custom_model``.
- ``"pytorch"`` — generic ``torch.nn.Module`` via ``torch.save`` /
  ``torch.load``. The storage path is auto-suffixed with ``.pt`` for
  compatibility with the Triton torch packager.
- ``"lightning"`` — ``pytorch_lightning.LightningModule`` via
  ``state_dict`` round-trip; loading re-instantiates ``metadata.model_class``
  with ``metadata.hyperparameters`` and applies ``load_state_dict``.

``_private/`` convention:
    This file lives in ``_private/`` — do not import directly from this path.
    Import ``ModelVariable`` from ``michelangelo.workflow.variables`` instead.
"""

from __future__ import annotations

import logging
import os
import tempfile
from dataclasses import dataclass
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


@dataclass
class ModelVariable(Variable):
    """A model variable flowing between workflow tasks.

    Subclasses ``Variable``. Underlying it could be a PyTorch model, a Lightning
    module, or a user-defined ``CustomModel``. Persistence is delegated to
    framework-specific handlers; dispatch is keyed on
    ``metadata.training_framework``.
    """

    @classmethod
    def create(cls, value: Any) -> ModelVariable:
        """Create a ``ModelVariable`` and auto-detect the training framework.

        The framework is set to ``"custom"`` when ``value`` implements the
        ``CustomModel`` ABC, ``"pytorch"`` when it's a ``torch.nn.Module``,
        and otherwise left unset (callers must populate ``metadata`` manually
        before calling ``save()``).

        Args:
            value: The in-memory model. ``CustomModel`` and ``torch.nn.Module``
                instances are recognised automatically; other framework types
                require manual ``metadata.training_framework`` setup.

        Returns:
            A new ``ModelVariable`` with ``value`` ready in memory and
            ``metadata`` populated with framework + ``model_class``.

        Example:
            >>> import torch  # doctest: +SKIP
            >>> var = ModelVariable.create(torch.nn.Linear(2, 1))  # doctest: +SKIP
            >>> var.metadata.training_framework  # doctest: +SKIP
            'pytorch'
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
            raise ValueError(
                f"Unrecognized training framework: {self.metadata.training_framework}"
            )

    def save(self):
        """Save value to variable path, dispatched on training_framework.

        Raises:
            ValueError: If no value has been set on this variable, or if
                ``metadata.training_framework`` is unset or unrecognised. Call
                the framework-specific ``save_*`` method directly when
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
            raise ValueError(
                f"Unrecognized training framework: {self.metadata.training_framework}"
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
                "model_class must be set in metadata to load a custom model."
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
        """Save a ``torch.nn.Module`` to ``self.path`` with a ``.pt`` suffix.

        The ``.pt`` extension is appended when missing — required for
        compatibility with the Triton torch packager. The full model object
        is pickled via ``torch.save(self._value, ...)``; loading therefore
        requires ``weights_only=False`` (see ``load_torch_model``).
        """
        import torch

        _logger.info("Saving PyTorch model for %s", self.path)

        if self._saved:
            _logger.info(
                "PyTorch model value already saved for %s. Skipping saving.", self.path
            )
            return

        if not self.path.endswith(".pt"):
            self.path = f"{self.path}.pt"

        fs, path = fsspec.core.url_to_fs(self.path)
        with tempfile.TemporaryDirectory() as temp_dir:
            model_file = os.path.join(temp_dir, "model.pt")
            torch.save(self._value, model_file)
            fs.put(model_file, path)

        self._saved = True

    def load_torch_model(self, weights_only: bool = True):
        """Load a ``torch.nn.Module`` via ``torch.load`` (CPU map-location).

        Args:
            weights_only: Forwarded to ``torch.load``. Defaults to ``True`` —
                the safer mode that refuses to execute arbitrary pickle
                opcodes. Pass ``weights_only=False`` when loading artifacts
                produced by ``save_torch_model``, which pickle the full
                ``nn.Module`` object (state_dict-only artifacts work with
                the default).

        Security:
            ``weights_only=False`` permits arbitrary code execution from the
            pickle stream. Only set it when the artifact source is trusted.
        """
        import torch

        _logger.info(
            "Loading PyTorch model from %s (weights_only=%s)",
            self.path,
            weights_only,
        )

        fs, path = fsspec.core.url_to_fs(self.path)
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            fs.get(path, model_path, recursive=True)
            self._value = torch.load(
                model_path, map_location="cpu", weights_only=weights_only
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

        fs, path = fsspec.core.url_to_fs(self.path)
        with tempfile.TemporaryDirectory() as temp_dir:
            model_file = os.path.join(temp_dir, "model.pt")
            torch.save(self._value.state_dict(), model_file)
            fs.put(model_file, path, recursive=True)

        self._saved = True

    def load_lightning_model(self):
        """Load a Lightning module by re-instantiating ``model_class``.

        Steps:

        1. Import the class from ``metadata.model_class``.
        2. Construct an instance via ``model_class(**metadata.hyperparameters)``
           (empty dict when ``hyperparameters`` is ``None``).
        3. Download the ``state_dict`` from ``self.path`` and apply
           ``load_state_dict`` (``weights_only=True`` — only tensors are
           unpickled, so the call is safe against malicious artifacts).
        4. Call ``model.eval()`` and store as ``self._value``.

        Raises:
            ValueError: If ``metadata.model_class`` is not set.
        """
        if not self.metadata.model_class:
            raise ValueError(
                "model_class must be set in metadata to load Lightning model"
            )

        import torch

        _logger.info("Loading Lightning model from %s", self.path)

        model_class = import_attribute(self.metadata.model_class)
        hyperparameters = self.metadata.hyperparameters or {}
        model = model_class(**hyperparameters)

        fs, path = fsspec.core.url_to_fs(self.path)
        with tempfile.TemporaryDirectory() as temp_dir:
            model_file = os.path.join(temp_dir, "model.pt")
            fs.get(path, model_file, recursive=True)
            state_dict = torch.load(model_file, map_location="cpu", weights_only=True)
            model.load_state_dict(state_dict)

        model.eval()
        self._value = model
        self._saved = True
