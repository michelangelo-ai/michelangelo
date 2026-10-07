"""Tests for applying ``limit_{train,val}_batches`` to the Ray Datasets.

Covers ``_apply_batch_limit`` (the row math and the fractional-to-int
resolution) against real, tiny Ray Datasets, and the ``LightningTrainer``
wiring that feeds the limited datasets and resolved limits to Ray Train.
"""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

pytest.importorskip("ray")
pytest.importorskip("torch")
pytest.importorskip("pytorch_lightning")

import ray.data

from michelangelo.lib._internal.errors import UserInputError
from michelangelo.lib.trainer.torch.pytorch_lightning._private.util import (
    _apply_batch_limit,
)
from michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer import (
    LightningTrainer,
    LightningTrainerParam,
)

_TORCH_TRAINER_INIT = (
    "michelangelo.lib.trainer.torch.pytorch_lightning."
    "lightning_trainer.TorchTrainer.__init__"
)


@pytest.fixture(scope="module")
def dataset() -> ray.data.Dataset:
    """A 1000-row Ray Dataset, larger than every cap used below."""
    return ray.data.range(1000)


class TestApplyBatchLimitInt:
    """Integer limits cap the dataset at ``N * batch_size * num_workers`` rows."""

    def test_unset_leaves_dataset_untouched(self, dataset):
        """``None`` returns the same dataset and no limit."""
        out, limit = _apply_batch_limit(dataset, None, 4, 2, "train")
        assert out is dataset
        assert limit is None

    def test_caps_rows_at_batches_times_batch_size_times_workers(self, dataset):
        """Three batches of four rows across two workers read 24 rows."""
        out, limit = _apply_batch_limit(dataset, 3, 4, 2, "train")
        assert out.count() == 24
        assert limit == 3

    def test_single_worker(self, dataset):
        """One worker needs only ``N * batch_size`` rows."""
        out, _ = _apply_batch_limit(dataset, 5, 8, 1, "val")
        assert out.count() == 40

    def test_limit_larger_than_dataset_reads_everything(self):
        """A cap above the dataset size does not truncate it."""
        small = ray.data.range(10)
        out, limit = _apply_batch_limit(small, 100, 4, 1, "train")
        assert out.count() == 10
        assert limit == 100

    def test_zero_leaves_dataset_untouched(self, dataset):
        """``0`` is passed through for Lightning to skip the split."""
        out, limit = _apply_batch_limit(dataset, 0, 4, 2, "val")
        assert out is dataset
        assert limit == 0

    def test_limited_iteration_terminates_and_stays_within_cap(self, dataset):
        """Iterating a limited dataset yields exactly the capped batches."""
        out, _ = _apply_batch_limit(dataset, 3, 4, 1, "train")
        batches = list(out.iter_batches(batch_size=4))
        assert len(batches) == 3
        assert sum(len(b["id"]) for b in batches) == 12


class TestApplyBatchLimitFraction:
    """Fractions in ``(0, 1)`` resolve to an integer batch count first."""

    def test_fraction_resolves_to_int_and_caps_dataset(self):
        """0.5 of 10 batches per worker is 5 batches, so 40 rows over 2 workers."""
        ds = ray.data.range(80)
        out, limit = _apply_batch_limit(ds, 0.5, 4, 2, "train")
        assert limit == 5
        assert isinstance(limit, int)
        assert out.count() == 40

    def test_fraction_rounds_partial_batches_up_before_scaling(self):
        """A trailing partial batch counts as a batch before the fraction applies."""
        ds = ray.data.range(10)
        out, limit = _apply_batch_limit(ds, 0.5, 4, 1, "val")
        # ceil(10 / 4) = 3 batches, int(3 * 0.5) = 1
        assert limit == 1
        assert out.count() == 4

    @pytest.mark.parametrize("fraction", [0.0, 1.0])
    def test_bounds_are_passed_through(self, dataset, fraction):
        """``0.0`` and ``1.0`` need no resolution and leave the dataset alone."""
        out, limit = _apply_batch_limit(dataset, fraction, 4, 2, "train")
        assert out is dataset
        assert limit == fraction

    def test_fraction_resolving_to_zero_batches_raises(self):
        """A fraction that cannot yield one batch is an explicit error."""
        ds = ray.data.range(8)
        with pytest.raises(UserInputError, match="resolves to 0 batches"):
            _apply_batch_limit(ds, 0.1, 4, 1, "train")


class TestApplyBatchLimitValidation:
    """Invalid limits fail with an actionable message naming the split."""

    @pytest.mark.parametrize("bad", [1.5, -0.1, float("nan")])
    def test_float_out_of_range_raises(self, dataset, bad):
        """Floats must lie in ``[0.0, 1.0]``."""
        with pytest.raises(UserInputError, match=r"limit_train_batches.*float"):
            _apply_batch_limit(dataset, bad, 4, 1, "train")

    def test_negative_int_raises(self, dataset):
        """Negative batch counts are rejected."""
        with pytest.raises(UserInputError, match=r"limit_val_batches.*non-negative"):
            _apply_batch_limit(dataset, -1, 4, 1, "val")

    @pytest.mark.parametrize("bad", ["3", True, [1]])
    def test_wrong_type_raises(self, dataset, bad):
        """Only ints and floats are accepted; ``bool`` is not a count."""
        with pytest.raises(UserInputError, match="must be an int or a float"):
            _apply_batch_limit(dataset, bad, 4, 1, "train")


class TestLightningTrainerAppliesLimits:
    """``LightningTrainer`` hands limited datasets and int limits to Ray Train."""

    @staticmethod
    def _param(**kwargs) -> LightningTrainerParam:
        defaults = {
            "create_model_fn": MagicMock(name="create_model_fn"),
            "create_model_fn_kwargs": {},
            "train_data": ray.data.range(1000),
            "val_data": ray.data.range(1000),
            "batch_size": 4,
        }
        defaults.update(kwargs)
        return LightningTrainerParam(**defaults)

    def _init(self, param, scaling_config=None):
        with patch(_TORCH_TRAINER_INIT, return_value=None) as mock_super:
            LightningTrainer(trainer_param=param, scaling_config=scaling_config)
        return mock_super.call_args.kwargs

    def test_no_limits_leaves_datasets_and_kwargs_alone(self):
        """Without any limit nothing is capped and no limit key is added."""
        kwargs = self._init(self._param(lightning_trainer_kwargs={"max_epochs": 1}))
        assert kwargs["datasets"]["train"].count() == 1000
        assert kwargs["datasets"]["val"].count() == 1000
        cfg = kwargs["train_loop_config"]["lightning_trainer_kwargs"]
        assert "limit_train_batches" not in cfg
        assert "limit_val_batches" not in cfg

    def test_int_limits_cap_both_splits_using_scaling_workers(self):
        """Each split is capped by its own limit across the configured workers."""
        scaling = MagicMock(name="scaling", num_workers=2)
        kwargs = self._init(
            self._param(
                lightning_trainer_kwargs={
                    "limit_train_batches": 3,
                    "limit_val_batches": 2,
                }
            ),
            scaling_config=scaling,
        )
        assert kwargs["datasets"]["train"].count() == 3 * 4 * 2
        assert kwargs["datasets"]["val"].count() == 2 * 4 * 2
        cfg = kwargs["train_loop_config"]["lightning_trainer_kwargs"]
        assert cfg["limit_train_batches"] == 3
        assert cfg["limit_val_batches"] == 2

    def test_defaults_to_one_worker_without_scaling_config(self):
        """No ``scaling_config`` means a single worker."""
        kwargs = self._init(
            self._param(lightning_trainer_kwargs={"limit_train_batches": 5})
        )
        assert kwargs["datasets"]["train"].count() == 5 * 4

    def test_fractional_limit_is_resolved_to_int_for_lightning(self):
        """A fraction reaches Lightning as an int and the dataset is capped to match.

        Loaders from ``iter_torch_batches`` have no ``__len__``, so Lightning only
        accepts an int limit for them.
        """
        kwargs = self._init(
            self._param(
                train_data=ray.data.range(80),
                lightning_trainer_kwargs={"limit_train_batches": 0.5},
            ),
            scaling_config=MagicMock(num_workers=2),
        )
        cfg = kwargs["train_loop_config"]["lightning_trainer_kwargs"]
        assert cfg["limit_train_batches"] == 5
        assert isinstance(cfg["limit_train_batches"], int)
        assert kwargs["datasets"]["train"].count() == 40

    def test_invalid_limit_fails_at_construction(self):
        """A bad limit raises before any Ray Train work starts."""
        with pytest.raises(UserInputError, match="limit_train_batches"):
            self._init(
                self._param(lightning_trainer_kwargs={"limit_train_batches": -2})
            )
