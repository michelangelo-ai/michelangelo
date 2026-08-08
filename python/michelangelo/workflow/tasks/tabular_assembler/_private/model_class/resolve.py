"""Model-class and training-framework resolution for the tabular assembler."""

from __future__ import annotations

import pytorch_lightning as pl
import torch.nn as nn

from michelangelo.lib.model_manager.interface.custom_model import Model
from michelangelo.uniflow.core.utils import import_attribute
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_CUSTOM,
    TRAINING_FRAMEWORK_LIGHTNING,
    TRAINING_FRAMEWORK_PYTORCH,
)


def try_load_class(model_class_path: str | None) -> type | None:
    """Return the class for a dotted import path, or ``None`` on failure.

    ``model_class_path`` is expected to come from a trusted pipeline
    definition (assembler config or a previously-recorded model's own
    metadata), not from unauthenticated end-user input: resolving it imports
    the target module, which executes that module's top-level code.

    Args:
        model_class_path: Fully-qualified dotted path to a class (e.g.
            ``"mypkg.models.MyModel"``), or ``None``/empty.

    Returns:
        The resolved class, or ``None`` if ``model_class_path`` is falsy, the
        module cannot be imported, the attribute does not exist, or the
        attribute is not a class.
    """
    if not model_class_path:
        return None
    try:
        attr = import_attribute(model_class_path)
    except (ModuleNotFoundError, ImportError, AttributeError, ValueError):
        return None
    return attr if isinstance(attr, type) else None


def resolve_training_framework(model_class_path: str | None) -> str | None:
    """Infer the training framework identifier for a dotted class path.

    Checks, in order: ``Model`` subclass, ``LightningModule`` subclass, plain
    ``nn.Module`` subclass. Lightning is checked before plain
    ``torch.nn.Module`` since ``LightningModule`` is itself an ``nn.Module``.

    Args:
        model_class_path: Fully-qualified dotted path to a model class, or
            ``None``.

    Returns:
        One of ``TRAINING_FRAMEWORK_CUSTOM``, ``TRAINING_FRAMEWORK_LIGHTNING``,
        ``TRAINING_FRAMEWORK_PYTORCH``, or ``None`` if the class cannot be
        resolved or does not match any known framework base class.
    """
    model_class = try_load_class(model_class_path)
    if model_class is None:
        return None
    if issubclass(model_class, Model):
        return TRAINING_FRAMEWORK_CUSTOM
    if issubclass(model_class, pl.LightningModule):
        return TRAINING_FRAMEWORK_LIGHTNING
    if issubclass(model_class, nn.Module):
        return TRAINING_FRAMEWORK_PYTORCH
    return None


def resolve_model_class(
    config_model_class: str | None,
    metadata_model_class: str | None,
) -> str | None:
    """Resolve the model class to package.

    Prefers the assembler config's ``model_class`` when it imports
    successfully; otherwise falls back to the model class recorded on the
    trained model's metadata.

    Args:
        config_model_class: ``TabularAssemblerConfig.model_class``, or
            ``None``.
        metadata_model_class: ``ModelMetadata.model_class`` from the raw
            trained model, or ``None``.

    Returns:
        ``config_model_class`` if it resolves to a class, otherwise
        ``metadata_model_class`` (which may itself be ``None``).
    """
    if config_model_class and try_load_class(config_model_class) is not None:
        return config_model_class
    return metadata_model_class
