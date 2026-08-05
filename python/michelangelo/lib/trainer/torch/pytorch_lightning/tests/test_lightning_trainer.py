"""Tests for the snapshot Lightning trainer module.

These tests cover the public surface of
``michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer`` against
its snapshot API: dataclass construction, ``LightningTrainer`` initialization
and result wiring, the DeepSpeed-aware ``LightningTrainerWithStateDict``, and
the ``_torch_weights_only_disabled`` env-var context manager.
"""

from __future__ import annotations

import os
import warnings
from unittest.mock import MagicMock, patch

import pytest

from michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer import (
    CHECKPOINT_PATH_KEY,
    LightningTrainer,
    LightningTrainerParam,
    LightningTrainerWithStateDict,
    _torch_weights_only_disabled,
)

# -----------------------------------------------------------------------------
# LightningTrainerParam
# -----------------------------------------------------------------------------


def _make_param(**overrides) -> LightningTrainerParam:
    """Build a minimally-valid ``LightningTrainerParam`` for tests."""
    defaults = {
        "create_model_fn": MagicMock(name="create_model_fn"),
        "create_model_fn_kwargs": {"hidden": 4},
        "train_data": MagicMock(name="train_data"),
        "val_data": MagicMock(name="val_data"),
    }
    defaults.update(overrides)
    return LightningTrainerParam(**defaults)


class TestLightningTrainerParam:
    """``LightningTrainerParam`` dataclass behavior."""

    def test_defaults(self):
        """Required-field-only construction populates the expected defaults."""
        param = _make_param()
        assert param.batch_size == 8
        assert param.num_shuffle_batches == 10
        assert param.num_epochs == 1  # _UNSET → 1 in __post_init__
        assert param.data_collate_fn is None
        assert param.lightning_trainer_kwargs == {}
        assert param.transfer_learning_spec is None
        assert param.incremental_training_spec is None
        assert param.initial_weights_path is None

    def test_training_observer_defaults_to_none(self):
        """``training_observer`` defaults to ``None`` when omitted."""
        param = _make_param()
        assert param.training_observer is None

    def test_experiment_store_defaults_to_none(self):
        """``experiment_store`` defaults to ``None`` (opt-in auto-resume)."""
        param = _make_param()
        assert param.experiment_store is None

    def test_experiment_store_stored(self):
        """``experiment_store`` is stored when provided."""
        store = MagicMock(name="store")
        param = _make_param(experiment_store=store)
        assert param.experiment_store is store

    def test_training_observer_stored(self):
        """``training_observer`` is stored when provided."""
        observer = MagicMock(name="observer")
        param = _make_param(training_observer=observer)
        assert param.training_observer is observer

    def test_lightning_kwargs_factory_isolates_instances(self):
        """Default-factory dict must not be shared across instances."""
        a = _make_param()
        b = _make_param()
        a.lightning_trainer_kwargs["max_epochs"] = 2
        assert "max_epochs" not in b.lightning_trainer_kwargs

    def test_num_epochs_unset_does_not_warn(self):
        """When ``num_epochs`` is omitted, no deprecation warning is raised."""
        with warnings.catch_warnings():
            warnings.simplefilter("error")  # convert any warning into an error
            param = _make_param()
        assert param.num_epochs == 1

    def test_num_epochs_explicit_warns(self):
        """Explicitly setting ``num_epochs`` logs a deprecation warning."""
        with patch(
            "michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer._logger"
        ) as mock_logger:
            param = _make_param(num_epochs=5)
        assert param.num_epochs == 5
        assert mock_logger.warning.called
        msg = mock_logger.warning.call_args[0][0]
        assert "num_epochs" in msg and "deprecated" in msg

    def test_passes_through_extra_fields(self):
        """Optional fields are accepted and stored as-is."""
        collate = MagicMock(name="collate")
        lightning_kwargs = {"strategy": "deepspeed"}

        param = _make_param(
            batch_size=64,
            num_shuffle_batches=0,
            data_collate_fn=collate,
            lightning_trainer_kwargs=lightning_kwargs,
            initial_weights_path="s3://bucket/weights.pt",
        )

        assert param.batch_size == 64
        assert param.num_shuffle_batches == 0
        assert param.data_collate_fn is collate
        assert param.lightning_trainer_kwargs is lightning_kwargs
        assert param.initial_weights_path == "s3://bucket/weights.pt"


# -----------------------------------------------------------------------------
# LightningTrainer
# -----------------------------------------------------------------------------


class TestLightningTrainerInit:
    """``LightningTrainer.__init__`` wiring into ``TorchTrainer``."""

    def test_init_forwards_datasets_and_pops_data_from_config(self):
        """``train_data`` / ``val_data`` go to ``datasets``, not the loop config."""
        train_ds = MagicMock(name="train")
        val_ds = MagicMock(name="val")
        param = _make_param(train_data=train_ds, val_data=val_ds, batch_size=12)

        with patch(
            "michelangelo.lib.trainer.torch.pytorch_lightning."
            "lightning_trainer.TorchTrainer.__init__",
            return_value=None,
        ) as mock_super:
            LightningTrainer(trainer_param=param)

        assert mock_super.called
        kwargs = mock_super.call_args.kwargs
        assert kwargs["datasets"] == {"train": train_ds, "val": val_ds}

        loop_cfg = kwargs["train_loop_config"]
        assert "train_data" not in loop_cfg
        assert "val_data" not in loop_cfg
        assert loop_cfg["batch_size"] == 12
        # __post_init__ removes the _UNSET sentinel, so num_epochs is a real int.
        assert loop_cfg["num_epochs"] == 1
        # A fresh run_id (UUID string) is injected for tracking.
        assert isinstance(loop_cfg["run_id"], str) and len(loop_cfg["run_id"]) > 0

    def test_init_forwards_scaling_and_run_config(self):
        """Optional ``run_config`` and ``scaling_config`` are forwarded verbatim."""
        run_cfg = MagicMock(name="run_cfg")
        scaling_cfg = MagicMock(name="scaling_cfg")
        param = _make_param()

        with patch(
            "michelangelo.lib.trainer.torch.pytorch_lightning."
            "lightning_trainer.TorchTrainer.__init__",
            return_value=None,
        ) as mock_super:
            LightningTrainer(
                trainer_param=param,
                run_config=run_cfg,
                scaling_config=scaling_cfg,
            )

        kwargs = mock_super.call_args.kwargs
        assert kwargs["run_config"] is run_cfg
        assert kwargs["scaling_config"] is scaling_cfg

    def test_init_each_run_id_is_unique(self):
        """Each ``LightningTrainer`` instance gets a distinct ``run_id``."""
        param = _make_param()
        with patch(
            "michelangelo.lib.trainer.torch.pytorch_lightning."
            "lightning_trainer.TorchTrainer.__init__",
            return_value=None,
        ) as mock_super:
            LightningTrainer(trainer_param=param)
            first_id = mock_super.call_args.kwargs["train_loop_config"]["run_id"]
            LightningTrainer(trainer_param=param)
            second_id = mock_super.call_args.kwargs["train_loop_config"]["run_id"]
        assert first_id != second_id

    def test_init_observer_popped_from_asdict_and_reinjected(self):
        """Observer is popped from asdict output and re-injected as the original object."""
        observer = MagicMock(name="observer")
        param = _make_param(training_observer=observer)

        with patch(
            "michelangelo.lib.trainer.torch.pytorch_lightning."
            "lightning_trainer.TorchTrainer.__init__",
            return_value=None,
        ) as mock_super:
            trainer = LightningTrainer(trainer_param=param)

        assert trainer._training_observer is observer
        loop_cfg = mock_super.call_args.kwargs["train_loop_config"]
        assert loop_cfg["training_observer"] is observer

    def test_init_no_observer_leaves_config_clean(self):
        """Without an observer, training_observer is not in train_loop_config."""
        param = _make_param()
        with patch(
            "michelangelo.lib.trainer.torch.pytorch_lightning."
            "lightning_trainer.TorchTrainer.__init__",
            return_value=None,
        ) as mock_super:
            trainer = LightningTrainer(trainer_param=param)

        assert trainer._training_observer is None
        loop_cfg = mock_super.call_args.kwargs["train_loop_config"]
        assert "training_observer" not in loop_cfg


class TestLightningTrainerTrain:
    """``LightningTrainer.train()`` result-shaping and error handling."""

    def _build(self, **kwargs):
        param = _make_param()
        with patch(
            "michelangelo.lib.trainer.torch.pytorch_lightning."
            "lightning_trainer.TorchTrainer.__init__",
            return_value=None,
        ):
            return LightningTrainer(trainer_param=param, **kwargs)

    def test_train_success_returns_summary_dict(self):
        """A successful ``fit()`` is reshaped into a small dict."""
        trainer = self._build()

        result = MagicMock()
        result.error = None
        result.checkpoint.path = "/some/ckpt"
        result.path = "/some/run"
        result.metrics = {"loss": 0.1, "config": "drop me"}

        with patch.object(trainer, "fit", return_value=result) as mock_fit:
            summary = trainer.train()

        mock_fit.assert_called_once()
        assert summary[CHECKPOINT_PATH_KEY] == "/some/ckpt"
        assert summary["path"] == "/some/run"
        # ``config`` must be stripped — it is typically not serializable.
        assert "config" not in summary["metrics"]
        assert summary["metrics"]["loss"] == 0.1
        # Checkpoint is cached on the instance for subclasses that need it.
        assert trainer.checkpoint is result.checkpoint

    def test_train_propagates_error(self):
        """If Ray Train reports an error, ``train()`` raises it."""
        trainer = self._build()
        boom = RuntimeError("training blew up")
        result = MagicMock()
        result.error = boom
        with (
            patch.object(trainer, "fit", return_value=result),
            pytest.raises(RuntimeError, match="training blew up"),
        ):
            trainer.train()

    def test_train_applies_overrides(self):
        """``run_config`` / ``scaling_config`` overrides are assigned before fit."""
        trainer = self._build()

        result = MagicMock()
        result.error = None
        result.checkpoint.path = "/c"
        result.path = "/r"
        result.metrics = {}

        new_run = MagicMock(name="new_run")
        new_scaling = MagicMock(name="new_scaling")
        with patch.object(trainer, "fit", return_value=result):
            trainer.train(run_config=new_run, scaling_config=new_scaling)

        assert trainer.run_config is new_run
        assert trainer.scaling_config is new_scaling

    def test_train_calls_observer_on_result(self):
        """Observer's ``on_result`` is called after successful training."""
        observer = MagicMock(name="observer")
        param = _make_param(training_observer=observer)
        with patch(
            "michelangelo.lib.trainer.torch.pytorch_lightning."
            "lightning_trainer.TorchTrainer.__init__",
            return_value=None,
        ):
            trainer = LightningTrainer(trainer_param=param)

        result = MagicMock()
        result.error = None
        result.checkpoint.path = "/ckpt"
        result.path = "/run"
        result.metrics = {"loss": 0.1}

        with patch.object(trainer, "fit", return_value=result):
            trainer.train()

        observer.on_result.assert_called_once_with(
            metrics=result.metrics, checkpoint_path="/ckpt"
        )

    def test_train_no_observer_does_not_raise(self):
        """Training without observer works without errors."""
        trainer = self._build()

        result = MagicMock()
        result.error = None
        result.checkpoint.path = "/c"
        result.path = "/r"
        result.metrics = {}

        with patch.object(trainer, "fit", return_value=result):
            summary = trainer.train()

        assert summary[CHECKPOINT_PATH_KEY] == "/c"


# -----------------------------------------------------------------------------
# LightningTrainerWithStateDict
# -----------------------------------------------------------------------------


class TestLightningTrainerWithStateDict:
    """DeepSpeed-vs-DDP routing and pre-train guards."""

    def _build(self, lightning_trainer_kwargs=None):
        param = _make_param(lightning_trainer_kwargs=lightning_trainer_kwargs or {})
        with patch(
            "michelangelo.lib.trainer.torch.pytorch_lightning."
            "lightning_trainer.TorchTrainer.__init__",
            return_value=None,
        ):
            return LightningTrainerWithStateDict(trainer_param=param)

    def test_update_state_dict_requires_train(self):
        """Calling ``update_model_state_dict`` before ``train`` is a user error."""
        trainer = self._build()
        with pytest.raises(ValueError, match="No checkpoint available"):
            trainer.update_model_state_dict(MagicMock())

    def test_is_deepspeed_strategy_none(self):
        """No strategy → not DeepSpeed."""
        trainer = self._build()
        assert trainer._is_deepspeed_strategy() is False

    def test_is_deepspeed_strategy_string(self):
        """A string strategy of ``deepspeed`` is recognized (case-insensitive)."""
        for name in ("deepspeed", "DeepSpeed", "DEEPSPEED"):
            trainer = self._build(lightning_trainer_kwargs={"strategy": name})
            assert trainer._is_deepspeed_strategy() is True

    def test_is_deepspeed_strategy_other_string(self):
        """A string strategy other than ``deepspeed`` is not DeepSpeed."""
        trainer = self._build(lightning_trainer_kwargs={"strategy": "ddp"})
        assert trainer._is_deepspeed_strategy() is False

    def test_is_deepspeed_strategy_class(self):
        """A ``RayDeepSpeedStrategy`` instance is recognized."""
        try:
            from ray.train.lightning import RayDeepSpeedStrategy
        except ImportError:
            pytest.skip("ray.train.lightning.RayDeepSpeedStrategy not available")

        strategy = MagicMock(spec=RayDeepSpeedStrategy)
        trainer = self._build(lightning_trainer_kwargs={"strategy": strategy})
        assert trainer._is_deepspeed_strategy() is True

    def test_is_deepspeed_strategy_other_class(self):
        """An arbitrary non-DeepSpeed strategy instance is not DeepSpeed."""
        trainer = self._build(lightning_trainer_kwargs={"strategy": MagicMock()})
        assert trainer._is_deepspeed_strategy() is False

    def test_update_state_dict_ddp_path(self, tmp_path):
        """DDP path loads the state_dict from a torch checkpoint file."""
        trainer = self._build()  # no strategy → DDP path

        # Build a fake checkpoint directory containing a CHECKPOINT_NAME file.
        from michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer import (
            CHECKPOINT_NAME,
        )

        ckpt_dir = tmp_path
        (ckpt_dir / CHECKPOINT_NAME).write_bytes(b"fake")

        fake_state = {"layer.weight": "tensor"}
        torch_model = MagicMock(name="torch_model")

        ray_ckpt = MagicMock()
        ray_ckpt.as_directory.return_value.__enter__ = lambda _: str(ckpt_dir)
        ray_ckpt.as_directory.return_value.__exit__ = lambda *_: None
        trainer.checkpoint = ray_ckpt

        with patch(
            "michelangelo.lib.trainer.torch.pytorch_lightning."
            "lightning_trainer.torch.load",
            return_value={"state_dict": fake_state},
        ) as mock_load:
            trainer.update_model_state_dict(torch_model)

        mock_load.assert_called_once()
        torch_model.load_state_dict.assert_called_once_with(fake_state, strict=False)

    def test_update_state_dict_deepspeed_path(self, tmp_path):
        """DeepSpeed path: ZeRO helper is called inside the env-var context."""
        trainer = self._build(lightning_trainer_kwargs={"strategy": "deepspeed"})

        from michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer import (
            CHECKPOINT_NAME,
        )

        ckpt_dir = tmp_path
        ds_dir = ckpt_dir / CHECKPOINT_NAME
        ds_dir.mkdir()  # DeepSpeed ckpt path is a directory, not a file.

        fake_state = {"layer.weight": "tensor"}
        torch_model = MagicMock(name="torch_model")

        ray_ckpt = MagicMock()
        ray_ckpt.as_directory.return_value.__enter__ = lambda _: str(ckpt_dir)
        ray_ckpt.as_directory.return_value.__exit__ = lambda *_: None
        trainer.checkpoint = ray_ckpt

        with patch(
            "michelangelo.lib.trainer.torch.pytorch_lightning."
            "lightning_trainer.convert_zero_checkpoint_to_fp32_state_dict",
            return_value=fake_state,
        ) as mock_convert:
            trainer.update_model_state_dict(torch_model)

        mock_convert.assert_called_once()
        torch_model.load_state_dict.assert_called_once_with(fake_state, strict=False)
        # The env var the context manager sets must be torn down on exit.
        assert "TORCH_FORCE_NO_WEIGHTS_ONLY_LOAD" not in os.environ


# -----------------------------------------------------------------------------
# _torch_weights_only_disabled
# -----------------------------------------------------------------------------


KEY = "TORCH_FORCE_NO_WEIGHTS_ONLY_LOAD"


class TestWeightsOnlyDisabledContext:
    """``_torch_weights_only_disabled`` sets and restores the env var correctly."""

    def setup_method(self):
        """Snapshot the env var so we can restore it after each test."""
        self._saved = os.environ.pop(KEY, None)

    def teardown_method(self):
        """Restore the original env-var state."""
        os.environ.pop(KEY, None)
        if self._saved is not None:
            os.environ[KEY] = self._saved

    def test_sets_and_unsets_when_unset_before(self):
        """If the var was unset, the context sets it to ``"1"`` and removes it after."""
        assert KEY not in os.environ
        with _torch_weights_only_disabled():
            assert os.environ[KEY] == "1"
        assert KEY not in os.environ

    def test_restores_previous_value(self):
        """If the var was already set, the context restores the old value on exit."""
        os.environ[KEY] = "previous"
        with _torch_weights_only_disabled():
            assert os.environ[KEY] == "1"
        assert os.environ[KEY] == "previous"

    def test_restores_on_exception(self):
        """The env var is restored even if the body raises."""
        os.environ[KEY] = "previous"
        with pytest.raises(RuntimeError), _torch_weights_only_disabled():
            assert os.environ[KEY] == "1"
            raise RuntimeError("boom")
        assert os.environ[KEY] == "previous"


# -----------------------------------------------------------------------------
# ExperimentStore wiring: __init__
# -----------------------------------------------------------------------------

_LT_MODULE = "michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer"


def _run_config(storage_path="/root/runs", name="run1"):
    """Build a stand-in ``RunConfig`` exposing the fields the trainer reads."""
    cfg = MagicMock(name="run_config")
    cfg.storage_path = storage_path
    cfg.name = name
    cfg.storage_filesystem = MagicMock(name="storage_filesystem")
    return cfg


class TestExperimentStoreInit:
    """``experiment_store`` pop/re-inject and identity injection in ``__init__``."""

    def test_store_popped_from_asdict_and_reinjected_with_identity(self):
        """Store is stored on ``self`` and re-injected with the run identity."""
        store = MagicMock(name="store")
        run_cfg = _run_config(storage_path="/root/runs", name="run1")
        param = _make_param(experiment_store=store)

        with patch(f"{_LT_MODULE}.TorchTrainer.__init__", return_value=None) as sup:
            trainer = LightningTrainer(trainer_param=param, run_config=run_cfg)

        assert trainer._experiment_store is store
        loop_cfg = sup.call_args.kwargs["train_loop_config"]
        assert loop_cfg["experiment_store"] is store
        assert loop_cfg["storage_path"] == "/root/runs"
        assert loop_cfg["run_name"] == "run1"

    def test_no_store_leaves_config_clean(self):
        """Without a store, no store/identity keys leak into the loop config."""
        param = _make_param()
        with patch(f"{_LT_MODULE}.TorchTrainer.__init__", return_value=None) as sup:
            trainer = LightningTrainer(trainer_param=param, run_config=_run_config())

        assert trainer._experiment_store is None
        loop_cfg = sup.call_args.kwargs["train_loop_config"]
        assert "experiment_store" not in loop_cfg
        assert "storage_path" not in loop_cfg
        assert "run_name" not in loop_cfg

    def test_store_without_run_config_omits_identity(self):
        """A store but no ``run_config`` re-injects the store but no identity."""
        store = MagicMock(name="store")
        param = _make_param(experiment_store=store)
        with patch(f"{_LT_MODULE}.TorchTrainer.__init__", return_value=None) as sup:
            LightningTrainer(trainer_param=param)  # no run_config

        loop_cfg = sup.call_args.kwargs["train_loop_config"]
        assert loop_cfg["experiment_store"] is store
        assert "storage_path" not in loop_cfg
        assert "run_name" not in loop_cfg


# -----------------------------------------------------------------------------
# ExperimentStore wiring: auto-resume resolution in _resolve_resume_checkpoint
# -----------------------------------------------------------------------------


class TestResolveResumeCheckpoint:
    """``_resolve_resume_checkpoint`` locates a checkpoint to seed resume from."""

    def _build(self, store, run_config):
        """Build a trainer with mocked base init and an assigned ``run_config``."""
        param = _make_param(experiment_store=store)
        with (
            patch(f"{_LT_MODULE}.TorchTrainer.__init__", return_value=None),
            patch.object(
                LightningTrainer, "_resolve_resume_checkpoint", return_value=None
            ),
        ):
            trainer = LightningTrainer(trainer_param=param, run_config=run_config)
        trainer.run_config = run_config
        return trainer

    def test_returns_checkpoint_when_candidate_has_one(self):
        """A candidate dir with a restorable checkpoint yields its path."""
        store = MagicMock(name="store")
        store.locate_resumable.return_value = "/candidate/exp"
        run_cfg = _run_config()
        trainer = self._build(store, run_cfg)

        with patch.object(
            LightningTrainer,
            "_latest_checkpoint_in",
            return_value="/candidate/exp/checkpoint_0",
        ) as latest:
            result = trainer._resolve_resume_checkpoint(run_cfg)

        store.locate_resumable.assert_called_once_with(
            storage_path=run_cfg.storage_path, run_name=run_cfg.name
        )
        latest.assert_called_once_with("/candidate/exp")
        assert result == "/candidate/exp/checkpoint_0"

    def test_returns_none_when_candidate_has_no_checkpoint(self):
        """A candidate dir without a restorable checkpoint yields ``None``."""
        store = MagicMock(name="store")
        store.locate_resumable.return_value = "/candidate/exp"
        run_cfg = _run_config()
        trainer = self._build(store, run_cfg)

        with patch.object(LightningTrainer, "_latest_checkpoint_in", return_value=None):
            assert trainer._resolve_resume_checkpoint(run_cfg) is None

    def test_returns_none_when_no_candidate(self):
        """A ``None`` candidate skips checkpoint resolution entirely."""
        store = MagicMock(name="store")
        store.locate_resumable.return_value = None
        run_cfg = _run_config()
        trainer = self._build(store, run_cfg)

        with patch.object(LightningTrainer, "_latest_checkpoint_in") as latest:
            assert trainer._resolve_resume_checkpoint(run_cfg) is None
        latest.assert_not_called()

    def test_locate_raising_is_swallowed(self):
        """A store that raises in ``locate_resumable`` falls through to ``None``."""
        store = MagicMock(name="store")
        store.locate_resumable.side_effect = RuntimeError("boom")
        run_cfg = _run_config()
        trainer = self._build(store, run_cfg)

        assert trainer._resolve_resume_checkpoint(run_cfg) is None  # must not raise

    def test_none_without_store(self):
        """No store → ``None``, and no crash reading run_config."""
        trainer = self._build(None, _run_config())
        assert trainer._resolve_resume_checkpoint(_run_config()) is None

    def test_none_without_run_config(self):
        """A store but ``run_config is None`` → no locate, ``None``."""
        store = MagicMock(name="store")
        trainer = self._build(store, None)
        assert trainer._resolve_resume_checkpoint(None) is None
        store.locate_resumable.assert_not_called()

    def test_incomplete_identity_logs_and_skips(self, caplog):
        """A missing storage_path logs once and skips locate."""
        import logging

        store = MagicMock(name="store")
        run_cfg = _run_config(storage_path=None)
        trainer = self._build(store, run_cfg)

        with caplog.at_level(logging.INFO, logger=_LT_MODULE):
            assert trainer._resolve_resume_checkpoint(run_cfg) is None

        store.locate_resumable.assert_not_called()
        assert any("auto-resume is disabled" in r.message for r in caplog.records)

    def test_missing_run_name_skips(self):
        """A missing run name → no locate, ``None``."""
        store = MagicMock(name="store")
        run_cfg = _run_config(name=None)
        trainer = self._build(store, run_cfg)
        assert trainer._resolve_resume_checkpoint(run_cfg) is None
        store.locate_resumable.assert_not_called()


class TestLatestCheckpointIn:
    """``_latest_checkpoint_in`` reads Ray's snapshot to find the latest checkpoint."""

    def _write_snapshot(self, exp_dir, payload):
        """Write a ``checkpoint_manager_snapshot.json`` into ``exp_dir``."""
        import json

        (exp_dir / "checkpoint_manager_snapshot.json").write_text(json.dumps(payload))

    def test_returns_joined_checkpoint_path(self, tmp_path):
        """The latest checkpoint dir name is joined onto the experiment path."""
        exp = tmp_path / "run1"
        exp.mkdir()
        self._write_snapshot(
            exp,
            {"latest_checkpoint_result": {"checkpoint_dir_name": "checkpoint_000042"}},
        )
        result = LightningTrainer._latest_checkpoint_in(str(exp))
        assert result == f"{exp}/checkpoint_000042"

    def test_returns_none_when_snapshot_missing(self, tmp_path):
        """No snapshot file → ``None`` (nothing to resume)."""
        exp = tmp_path / "run1"
        exp.mkdir()
        assert LightningTrainer._latest_checkpoint_in(str(exp)) is None

    def test_returns_none_when_no_latest_result(self, tmp_path):
        """A snapshot without ``latest_checkpoint_result`` → ``None``."""
        exp = tmp_path / "run1"
        exp.mkdir()
        self._write_snapshot(exp, {"checkpoint_results": []})
        assert LightningTrainer._latest_checkpoint_in(str(exp)) is None

    def test_returns_none_when_dir_name_empty(self, tmp_path):
        """A latest result with no ``checkpoint_dir_name`` → ``None``."""
        exp = tmp_path / "run1"
        exp.mkdir()
        self._write_snapshot(exp, {"latest_checkpoint_result": {}})
        assert LightningTrainer._latest_checkpoint_in(str(exp)) is None

    def test_returns_none_on_corrupt_snapshot(self, tmp_path):
        """A corrupt (non-JSON) snapshot → ``None`` (swallowed)."""
        exp = tmp_path / "run1"
        exp.mkdir()
        (exp / "checkpoint_manager_snapshot.json").write_text("{ not json")
        assert LightningTrainer._latest_checkpoint_in(str(exp)) is None


class TestResumeCheckpointInjection:
    """``__init__`` threads the resolved checkpoint into ``train_loop_config``."""

    def test_seed_injected_when_resolved(self):
        """A resolved checkpoint path is placed in the worker config."""
        store = MagicMock(name="store")
        param = _make_param(experiment_store=store)
        with (
            patch(f"{_LT_MODULE}.TorchTrainer.__init__", return_value=None) as sup,
            patch.object(
                LightningTrainer,
                "_resolve_resume_checkpoint",
                return_value="/exp/checkpoint_0",
            ),
        ):
            LightningTrainer(trainer_param=param, run_config=_run_config())

        loop_cfg = sup.call_args.kwargs["train_loop_config"]
        assert loop_cfg["resume_checkpoint_path"] == "/exp/checkpoint_0"

    def test_no_seed_key_when_unresolved(self):
        """No key is injected when nothing is resolved (fresh run)."""
        store = MagicMock(name="store")
        param = _make_param(experiment_store=store)
        with (
            patch(f"{_LT_MODULE}.TorchTrainer.__init__", return_value=None) as sup,
            patch.object(
                LightningTrainer, "_resolve_resume_checkpoint", return_value=None
            ),
        ):
            LightningTrainer(trainer_param=param, run_config=_run_config())

        loop_cfg = sup.call_args.kwargs["train_loop_config"]
        assert "resume_checkpoint_path" not in loop_cfg


class TestTrainRunConfigOverride:
    """``train(run_config=...)`` override interplay with auto-resume (issue #2)."""

    def _trainer_with_result(self, store):
        """Build a trainer whose ``fit`` returns a benign result."""
        param = _make_param(experiment_store=store)
        with (
            patch(f"{_LT_MODULE}.TorchTrainer.__init__", return_value=None),
            patch.object(
                LightningTrainer, "_resolve_resume_checkpoint", return_value=None
            ),
        ):
            trainer = LightningTrainer(trainer_param=param, run_config=_run_config())
        result = MagicMock()
        result.error = None
        result.checkpoint.path = "/c"
        result.path = "/r"
        result.metrics = {}
        trainer.fit = MagicMock(return_value=result)
        return trainer

    def test_override_warns_when_store_set(self, caplog):
        """Overriding run_config in train() warns that resume won't re-trigger."""
        import logging

        trainer = self._trainer_with_result(MagicMock(name="store"))
        with caplog.at_level(logging.WARNING, logger=_LT_MODULE):
            trainer.train(run_config=_run_config(name="different"))

        assert any("will not re-trigger" in r.message for r in caplog.records)

    def test_no_warning_without_store(self, caplog):
        """Without a store, a run_config override is not warned about."""
        import logging

        trainer = self._trainer_with_result(None)
        with caplog.at_level(logging.WARNING, logger=_LT_MODULE):
            trainer.train(run_config=_run_config(name="different"))

        assert not any("re-trigger" in r.message for r in caplog.records)


# -----------------------------------------------------------------------------
# Profiler wiring (driver side)
# -----------------------------------------------------------------------------


class TestProfilerRowCounting:
    """Driver-side training-row counting that feeds the profiler schedule."""

    def _loop_config(self, param):
        """Construct a trainer with ``TorchTrainer.__init__`` stubbed out."""
        with patch(
            "michelangelo.lib.trainer.torch.pytorch_lightning."
            "lightning_trainer.TorchTrainer.__init__",
            return_value=None,
        ) as mock_super:
            LightningTrainer(trainer_param=param)
        return mock_super.call_args.kwargs["train_loop_config"]

    def test_rows_not_counted_without_a_profiler(self):
        """No profiler config means the (potentially expensive) count is skipped."""
        train_ds = MagicMock(name="train")
        loop_cfg = self._loop_config(_make_param(train_data=train_ds))

        train_ds.count.assert_not_called()
        assert "train_dataset_num_rows" not in loop_cfg

    def test_rows_counted_when_a_profiler_is_configured(self):
        """A profiler config triggers the count and threads it to the worker."""
        train_ds = MagicMock(name="train")
        train_ds.count.return_value = 4096
        param = _make_param(
            train_data=train_ds, lightning_trainer_kwargs={"profiler": "simple"}
        )

        loop_cfg = self._loop_config(param)

        train_ds.count.assert_called_once_with()
        assert loop_cfg["train_dataset_num_rows"] == 4096

    def test_count_failure_falls_back_to_none(self):
        """A dataset that cannot be counted degrades to an unknown row count."""
        train_ds = MagicMock(name="train")
        train_ds.count.side_effect = RuntimeError("cannot count")
        param = _make_param(
            train_data=train_ds, lightning_trainer_kwargs={"profiler": {"pytorch": {}}}
        )

        loop_cfg = self._loop_config(param)

        assert loop_cfg["train_dataset_num_rows"] is None

    def test_profiler_sink_defaults_to_none(self):
        """``profiler_sink`` is optional."""
        assert _make_param().profiler_sink is None

    def test_profiler_sink_reaches_the_worker_config(self):
        """A plain callable survives ``asdict()`` and is threaded to workers."""

        def my_sink(profiler, profiler_logs_path, logger):
            """Test sink."""

        loop_cfg = self._loop_config(_make_param(profiler_sink=my_sink))

        assert loop_cfg["profiler_sink"] is my_sink
