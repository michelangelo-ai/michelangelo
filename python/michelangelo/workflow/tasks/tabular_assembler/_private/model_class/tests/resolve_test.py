"""Unit tests for ``...tabular_assembler._private.model_class.resolve``."""

from __future__ import annotations

import unittest
from typing import TYPE_CHECKING

import pytorch_lightning as pl
import torch.nn as nn

from michelangelo.lib.model_manager.interface.custom_model import Model
from michelangelo.workflow.tasks.tabular_assembler._private.model_class.resolve import (
    resolve_model_class,
    resolve_training_framework,
    try_load_class,
)
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_CUSTOM,
    TRAINING_FRAMEWORK_LIGHTNING,
    TRAINING_FRAMEWORK_PYTORCH,
)

if TYPE_CHECKING:
    from numpy import ndarray


class _CustomModelFixture(Model):
    """Minimal concrete ``Model`` used only to exercise framework resolution."""

    def save(self, path: str) -> None:
        pass

    @classmethod
    def load(cls, path: str) -> _CustomModelFixture:
        return cls()

    def predict(self, inputs: dict[str, ndarray]) -> dict[str, ndarray]:
        return inputs


CUSTOM_MODEL_CLASS_PATH = (
    "michelangelo.workflow.tasks.tabular_assembler._private.model_class.tests."
    "resolve_test._CustomModelFixture"
)


class _StructuralModelFixture:
    """Implements the ``save``/``load``/``predict`` shape without inheriting ``Model``.

    Used to prove ``resolve_training_framework`` requires literal ``Model``
    subclassing (nominal, matching the internal interface's own behavior) --
    shape alone is not enough.
    """

    def save(self, path: str) -> None:
        pass

    @classmethod
    def load(cls, path: str) -> _StructuralModelFixture:
        return cls()

    def predict(self, inputs: dict[str, ndarray]) -> dict[str, ndarray]:
        return inputs


_STRUCTURAL_MODEL_CLASS_PATH = (
    "michelangelo.workflow.tasks.tabular_assembler._private.model_class.tests."
    "resolve_test._StructuralModelFixture"
)


class _MinimalTorch(nn.Module):
    def forward(self, x):  # type: ignore[override]
        return x


_TORCH_MODEL_PATH = (
    "michelangelo.workflow.tasks.tabular_assembler._private.model_class.tests."
    "resolve_test._MinimalTorch"
)


class _MinimalLightning(pl.LightningModule):
    def forward(self, x):  # type: ignore[override]
        return x


_LIGHTNING_MODEL_PATH = (
    "michelangelo.workflow.tasks.tabular_assembler._private.model_class.tests."
    "resolve_test._MinimalLightning"
)


class ResolveTest(unittest.TestCase):
    """Tests for ``try_load_class``, ``resolve_training_framework``, ``resolve_model_class``."""  # noqa: E501

    def test_try_load_class_none_and_empty_string(self):
        """``None`` and empty-string input both return ``None``."""
        for value in (None, ""):
            with self.subTest(value=value):
                self.assertIsNone(try_load_class(value))

    def test_try_load_class_invalid_module(self):
        """An unimportable module path returns ``None``."""
        self.assertIsNone(try_load_class("nonexistent_module_xyz_abc.SomeClass"))

    def test_try_load_class_builtin_type(self):
        """A builtin type resolves to that type."""
        self.assertIs(try_load_class("builtins.str"), str)

    def test_try_load_class_non_type_attribute(self):
        """A non-class attribute (e.g. a function) returns ``None``."""
        self.assertIsNone(try_load_class("math.sqrt"))

    def test_try_load_class_propagates_exceptions_outside_the_narrow_catch(self):
        """An exception from the target module's own top-level code is not swallowed.

        ``try_load_class`` only degrades to ``None`` for
        ``ModuleNotFoundError``/``ImportError``/``AttributeError``/``ValueError``
        (an import-resolution failure). A module that imports successfully as
        far as Python is concerned but raises something else (e.g. a plain
        ``RuntimeError``) during its own top-level execution is a different
        failure mode -- a broken module, not a missing one -- and should
        propagate uncaught rather than silently resolve to ``None``.
        """
        with self.assertRaisesRegex(RuntimeError, "boom"):
            try_load_class(
                "michelangelo.workflow.tasks.tabular_assembler._private.model_class."
                "tests.fixtures.broken_module.SomeClass"
            )

    def test_resolve_training_framework_custom_model(self):
        """A ``Model`` subclass resolves to the custom framework."""
        self.assertEqual(
            resolve_training_framework(CUSTOM_MODEL_CLASS_PATH),
            TRAINING_FRAMEWORK_CUSTOM,
        )

    def test_resolve_training_framework_structurally_conformant_model(self):
        """A structurally-conformant, non-inheriting class does not resolve to custom.

        ``resolve_training_framework`` requires literal ``Model`` inheritance
        (nominal) -- a class that only implements the save/load/predict
        shape without subclassing ``Model`` is not routed to the custom
        packager.
        """
        self.assertIsNone(resolve_training_framework(_STRUCTURAL_MODEL_CLASS_PATH))

    def test_resolve_training_framework_lightning_before_torch(self):
        """A ``LightningModule`` subclass resolves to lightning, not plain torch."""
        self.assertEqual(
            resolve_training_framework(_LIGHTNING_MODEL_PATH),
            TRAINING_FRAMEWORK_LIGHTNING,
        )

    def test_resolve_training_framework_plain_torch_module(self):
        """A plain ``nn.Module`` subclass resolves to the pytorch framework."""
        self.assertEqual(
            resolve_training_framework(_TORCH_MODEL_PATH), TRAINING_FRAMEWORK_PYTORCH
        )

    def test_resolve_training_framework_none_and_invalid(self):
        """Unresolvable paths and non-framework classes return ``None``."""
        self.assertIsNone(
            resolve_training_framework("nonexistent_module_xyz_abc.Model")
        )
        # str is not a Model / LightningModule / nn.Module in the resolver sense.
        self.assertIsNone(resolve_training_framework("builtins.str"))

    def test_resolve_model_class_uses_config_when_importable(self):
        """The config-supplied class wins when it imports successfully."""
        self.assertEqual(
            resolve_model_class(CUSTOM_MODEL_CLASS_PATH, "metadata.fallback.Model"),
            CUSTOM_MODEL_CLASS_PATH,
        )

    def test_resolve_model_class_falls_back_when_config_not_importable(self):
        """Falls back to the metadata class when the config class is unimportable."""
        self.assertEqual(
            resolve_model_class(
                "nonexistent_module_xyz_abc.Model", "metadata.fallback.Model"
            ),
            "metadata.fallback.Model",
        )

    def test_resolve_model_class_without_config_uses_metadata(self):
        """With no config class at all, the metadata class is used."""
        self.assertEqual(resolve_model_class(None, "meta.Model"), "meta.Model")
