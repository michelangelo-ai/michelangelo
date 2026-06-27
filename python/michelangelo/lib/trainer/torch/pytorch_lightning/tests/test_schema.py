"""Tests for the Lightning trainer schema dataclasses and enums.

Covers ``michelangelo.lib.trainer.torch.pytorch_lightning.schema``:
the ``TrainingType`` / ``LearningMode`` enums and the ``ModelSpec`` /
``IncrementalTrainingMetadata`` / ``IncrementalTrainingSpec`` /
``TransferLearningMetadata`` / ``TransferLearningSpec`` dataclasses.

These types are pure-Python dataclasses with no heavy dependencies, so the
tests exercise construction, defaults, and field wiring directly.
"""

from __future__ import annotations

import dataclasses

import pytest

# Importing the schema module pulls in the package ``__init__`` which eagerly
# imports the Ray/Lightning-backed trainer. Skip cleanly if those optional
# heavy dependencies are not installed (e.g. a lightweight dev environment).
pytest.importorskip("ray")
pytest.importorskip("torch")
pytest.importorskip("pytorch_lightning")

from michelangelo.lib.trainer.torch.pytorch_lightning.schema import (
    IncrementalTrainingMetadata,
    IncrementalTrainingSpec,
    LearningMode,
    ModelSpec,
    TrainingObserver,
    TrainingType,
    TransferLearningMetadata,
    TransferLearningSpec,
)

# -----------------------------------------------------------------------------
# TrainingObserver Protocol
# -----------------------------------------------------------------------------


class TestTrainingObserver:
    """``TrainingObserver`` is a runtime-checkable protocol."""

    def test_runtime_checkable_with_matching_class(self):
        """A plain class implementing both methods satisfies the protocol."""

        class _Obs:
            def on_result(self, metrics, checkpoint_path):
                pass

            def on_checkpoint_saved(self, epoch, step, metrics, checkpoint_path):
                pass

        assert isinstance(_Obs(), TrainingObserver)

    def test_runtime_checkable_rejects_incomplete_class(self):
        """A class missing a method does not satisfy the protocol."""

        class _Partial:
            def on_result(self, metrics, checkpoint_path):
                pass

        assert not isinstance(_Partial(), TrainingObserver)


# -----------------------------------------------------------------------------
# Enums
# -----------------------------------------------------------------------------


class TestTrainingType:
    """``TrainingType`` enum values and membership."""

    def test_member_values(self):
        """The two members map to their fixed integer values."""
        assert TrainingType.BASE_MODEL_TRAINING.value == 0
        assert TrainingType.INCREMENTAL_TRAINING.value == 1

    def test_lookup_by_value(self):
        """Members can be recovered from their integer value."""
        assert TrainingType(0) is TrainingType.BASE_MODEL_TRAINING
        assert TrainingType(1) is TrainingType.INCREMENTAL_TRAINING

    def test_lookup_by_name(self):
        """Members can be recovered from their name."""
        assert TrainingType["INCREMENTAL_TRAINING"] is TrainingType.INCREMENTAL_TRAINING

    def test_member_count(self):
        """Exactly two training types are defined."""
        assert len(list(TrainingType)) == 2

    def test_unknown_value_raises(self):
        """An out-of-range value is not a valid member."""
        with pytest.raises(ValueError):
            TrainingType(99)


class TestLearningMode:
    """``LearningMode`` enum values and membership."""

    def test_member_values(self):
        """The two members map to their fixed integer values."""
        assert LearningMode.DISABLED.value == 0
        assert LearningMode.TRANSFER_LEARNING.value == 1

    def test_lookup_by_value(self):
        """Members can be recovered from their integer value."""
        assert LearningMode(0) is LearningMode.DISABLED
        assert LearningMode(1) is LearningMode.TRANSFER_LEARNING

    def test_member_count(self):
        """Exactly two learning modes are defined."""
        assert len(list(LearningMode)) == 2

    def test_unknown_value_raises(self):
        """An out-of-range value is not a valid member."""
        with pytest.raises(ValueError):
            LearningMode(7)


# -----------------------------------------------------------------------------
# ModelSpec
# -----------------------------------------------------------------------------


class TestModelSpec:
    """``ModelSpec`` reference dataclass."""

    def test_required_fields(self):
        """``project_name`` and ``model_name`` are stored verbatim."""
        spec = ModelSpec(project_name="proj", model_name="mdl")
        assert spec.project_name == "proj"
        assert spec.model_name == "mdl"

    def test_revision_id_defaults_to_none(self):
        """``revision_id`` defaults to ``None`` when omitted."""
        spec = ModelSpec(project_name="proj", model_name="mdl")
        assert spec.revision_id is None

    def test_revision_id_can_be_set(self):
        """``revision_id`` is stored when provided."""
        spec = ModelSpec(project_name="proj", model_name="mdl", revision_id="rev-1")
        assert spec.revision_id == "rev-1"

    def test_equality(self):
        """Dataclass equality compares by field values."""
        a = ModelSpec(project_name="p", model_name="m", revision_id="r")
        b = ModelSpec(project_name="p", model_name="m", revision_id="r")
        assert a == b

    def test_is_dataclass(self):
        """``ModelSpec`` is a dataclass."""
        assert dataclasses.is_dataclass(ModelSpec)


# -----------------------------------------------------------------------------
# IncrementalTrainingMetadata
# -----------------------------------------------------------------------------


class TestIncrementalTrainingMetadata:
    """``IncrementalTrainingMetadata`` field wiring and defaults."""

    def _baseline(self):
        """Build a minimal baseline ``ModelSpec``."""
        return ModelSpec(project_name="proj", model_name="mdl")

    def test_required_fields(self):
        """``training_type`` and ``baseline_model`` are stored verbatim."""
        baseline = self._baseline()
        meta = IncrementalTrainingMetadata(
            training_type=TrainingType.INCREMENTAL_TRAINING,
            baseline_model=baseline,
        )
        assert meta.training_type is TrainingType.INCREMENTAL_TRAINING
        assert meta.baseline_model is baseline

    def test_optional_field_defaults(self):
        """Optional fields fall back to their declared defaults."""
        meta = IncrementalTrainingMetadata(
            training_type=TrainingType.BASE_MODEL_TRAINING,
            baseline_model=self._baseline(),
        )
        assert meta.deployment_name is None
        assert meta.skip_training is False
        assert meta.log_layer_weights is False

    def test_optional_fields_overridden(self):
        """Optional fields are honored when provided."""
        meta = IncrementalTrainingMetadata(
            training_type=TrainingType.INCREMENTAL_TRAINING,
            baseline_model=self._baseline(),
            deployment_name="deploy-a",
            skip_training=True,
            log_layer_weights=True,
        )
        assert meta.deployment_name == "deploy-a"
        assert meta.skip_training is True
        assert meta.log_layer_weights is True


# -----------------------------------------------------------------------------
# IncrementalTrainingSpec
# -----------------------------------------------------------------------------


class TestIncrementalTrainingSpec:
    """``IncrementalTrainingSpec`` field wiring and defaults."""

    def _metadata(self):
        """Build a minimal ``IncrementalTrainingMetadata``."""
        return IncrementalTrainingMetadata(
            training_type=TrainingType.INCREMENTAL_TRAINING,
            baseline_model=ModelSpec(project_name="proj", model_name="mdl"),
        )

    def test_required_metadata_field(self):
        """``metadata`` is stored verbatim."""
        meta = self._metadata()
        spec = IncrementalTrainingSpec(metadata=meta)
        assert spec.metadata is meta

    def test_defaults(self):
        """Optional fields fall back to their declared defaults."""
        spec = IncrementalTrainingSpec(metadata=self._metadata())
        assert spec.load_optimizer_weights is False
        assert spec.override_incremental_training_epoch is None

    def test_overrides(self):
        """Optional fields are honored when provided."""
        spec = IncrementalTrainingSpec(
            metadata=self._metadata(),
            load_optimizer_weights=True,
            override_incremental_training_epoch=5,
        )
        assert spec.load_optimizer_weights is True
        assert spec.override_incremental_training_epoch == 5


# -----------------------------------------------------------------------------
# TransferLearningMetadata
# -----------------------------------------------------------------------------


class TestTransferLearningMetadata:
    """``TransferLearningMetadata`` field wiring."""

    def test_fields_stored(self):
        """Both fields are stored verbatim."""
        baseline = ModelSpec(project_name="proj", model_name="mdl")
        meta = TransferLearningMetadata(
            learning_mode=LearningMode.TRANSFER_LEARNING,
            baseline_model=baseline,
        )
        assert meta.learning_mode is LearningMode.TRANSFER_LEARNING
        assert meta.baseline_model is baseline

    def test_baseline_model_may_be_none(self):
        """``baseline_model`` accepts ``None`` (no required default)."""
        meta = TransferLearningMetadata(
            learning_mode=LearningMode.DISABLED,
            baseline_model=None,
        )
        assert meta.baseline_model is None


# -----------------------------------------------------------------------------
# TransferLearningSpec
# -----------------------------------------------------------------------------


class TestTransferLearningSpec:
    """``TransferLearningSpec`` field wiring, defaults, and list factories."""

    def _metadata(self):
        """Build a minimal ``TransferLearningMetadata``."""
        return TransferLearningMetadata(
            learning_mode=LearningMode.TRANSFER_LEARNING,
            baseline_model=ModelSpec(project_name="proj", model_name="mdl"),
        )

    def test_required_metadata_field(self):
        """``metadata`` is stored verbatim."""
        meta = self._metadata()
        spec = TransferLearningSpec(metadata=meta)
        assert spec.metadata is meta

    def test_default_scalar_fields(self):
        """``model_loader_function`` defaults to ``None``."""
        spec = TransferLearningSpec(metadata=self._metadata())
        assert spec.model_loader_function is None

    def test_default_list_fields_are_empty(self):
        """All four layer-name list fields default to empty lists."""
        spec = TransferLearningSpec(metadata=self._metadata())
        assert spec.layer_names_to_inherit == []
        assert spec.layer_names_to_inherit_regex == []
        assert spec.layer_names_to_freeze == []
        assert spec.layer_names_to_freeze_regex == []

    def test_list_factories_are_independent_per_instance(self):
        """Each instance gets its own list (default_factory, not shared mutable)."""
        a = TransferLearningSpec(metadata=self._metadata())
        b = TransferLearningSpec(metadata=self._metadata())
        a.layer_names_to_freeze.append("layer.0")
        assert a.layer_names_to_freeze == ["layer.0"]
        assert b.layer_names_to_freeze == []

    def test_overrides(self):
        """Provided list / scalar values are honored."""
        spec = TransferLearningSpec(
            metadata=self._metadata(),
            model_loader_function="pkg.module.load",
            layer_names_to_inherit=["enc.0"],
            layer_names_to_inherit_regex=[r"enc\..*"],
            layer_names_to_freeze=["enc.0"],
            layer_names_to_freeze_regex=[r"frozen\..*"],
        )
        assert spec.model_loader_function == "pkg.module.load"
        assert spec.layer_names_to_inherit == ["enc.0"]
        assert spec.layer_names_to_inherit_regex == [r"enc\..*"]
        assert spec.layer_names_to_freeze == ["enc.0"]
        assert spec.layer_names_to_freeze_regex == [r"frozen\..*"]
