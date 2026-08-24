"""Tests for the XGBoost trainer module.

Cover the public surface of ``michelangelo.lib.trainer.xgboost.xgboost_trainer``:
dataclass validation, trainer construction and dataset wiring, the ``train()``
result contract, and the per-worker training loop. Ray Train's parent
``__init__`` and ``fit()`` are patched throughout, so nothing here needs a Ray
cluster.
"""

from __future__ import annotations

import sys
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

import pytest

pytest.importorskip("xgboost")
pytest.importorskip("ray.train.xgboost")

from michelangelo.lib.trainer.xgboost.xgboost_trainer import (
    CHECKPOINT_PATH_KEY,
    TRAIN_DATASET_KEY,
    VALIDATION_DATASET_KEY,
    XGBoostTrainer,
    XGBoostTrainerParam,
    _train_loop_per_worker,
)

_PARENT_INIT = "ray.train.xgboost.XGBoostTrainer.__init__"


def _make_param(**overrides) -> XGBoostTrainerParam:
    """Build a minimally-valid ``XGBoostTrainerParam`` for tests."""
    defaults = {
        "label_column": "target",
        "train_data": MagicMock(name="train_data"),
    }
    defaults.update(overrides)
    return XGBoostTrainerParam(**defaults)


# -----------------------------------------------------------------------------
# XGBoostTrainerParam
# -----------------------------------------------------------------------------


class TestXGBoostTrainerParam:
    """``XGBoostTrainerParam`` dataclass behavior."""

    def test_defaults(self):
        """It applies the documented default for every optional field."""
        param = _make_param()
        assert param.val_data is None
        assert param.params == {}
        assert param.num_boost_round == 10
        assert param.feature_columns is None
        assert param.xgboost_train_kwargs == {}

    def test_mutable_defaults_are_not_shared(self):
        """It gives each instance its own params dict."""
        first = _make_param()
        second = _make_param()
        first.params["eta"] = 0.1
        assert second.params == {}

    def test_empty_label_column_rejected(self):
        """It rejects an empty label_column."""
        with pytest.raises(ValueError, match="label_column"):
            _make_param(label_column="")

    @pytest.mark.parametrize("rounds", [0, -1])
    def test_non_positive_boost_rounds_rejected(self, rounds):
        """It rejects a num_boost_round below one."""
        with pytest.raises(ValueError, match="num_boost_round"):
            _make_param(num_boost_round=rounds)

    def test_user_callbacks_rejected(self):
        """It rejects user callbacks, which would displace checkpointing."""
        with pytest.raises(ValueError, match="callbacks"):
            _make_param(xgboost_train_kwargs={"callbacks": [object()]})

    def test_other_train_kwargs_allowed(self):
        """It keeps xgboost train kwargs other than callbacks."""
        param = _make_param(xgboost_train_kwargs={"verbose_eval": False})
        assert param.xgboost_train_kwargs == {"verbose_eval": False}


# -----------------------------------------------------------------------------
# XGBoostTrainer construction
# -----------------------------------------------------------------------------


class TestXGBoostTrainerInit:
    """Argument wiring handed to Ray's ``XGBoostTrainer``."""

    def test_train_only_dataset_wiring(self):
        """It passes only the train dataset when no validation set is given."""
        param = _make_param()
        with patch(_PARENT_INIT, return_value=None) as parent:
            XGBoostTrainer(param)

        datasets = parent.call_args.kwargs["datasets"]
        assert datasets == {TRAIN_DATASET_KEY: param.train_data}
        assert parent.call_args.kwargs["train_loop_config"]["has_validation"] is False

    def test_validation_dataset_wiring(self):
        """It passes the validation dataset and flags it in the config."""
        val = MagicMock(name="val_data")
        param = _make_param(val_data=val)
        with patch(_PARENT_INIT, return_value=None) as parent:
            XGBoostTrainer(param)

        datasets = parent.call_args.kwargs["datasets"]
        assert datasets[VALIDATION_DATASET_KEY] is val
        assert parent.call_args.kwargs["train_loop_config"]["has_validation"] is True

    def test_train_loop_config_carries_booster_settings(self):
        """It forwards the booster settings through train_loop_config."""
        param = _make_param(
            params={"objective": "reg:squarederror"},
            num_boost_round=42,
            feature_columns=["a", "b"],
            xgboost_train_kwargs={"verbose_eval": False},
        )
        with patch(_PARENT_INIT, return_value=None) as parent:
            XGBoostTrainer(param)

        config = parent.call_args.kwargs["train_loop_config"]
        assert config["label_column"] == "target"
        assert config["params"] == {"objective": "reg:squarederror"}
        assert config["num_boost_round"] == 42
        assert config["feature_columns"] == ["a", "b"]
        assert config["xgboost_train_kwargs"] == {"verbose_eval": False}

    def test_train_loop_function_is_passed_positionally(self):
        """It passes the worker loop as the first positional argument."""
        with patch(_PARENT_INIT, return_value=None) as parent:
            XGBoostTrainer(_make_param())

        assert parent.call_args.args[0] is _train_loop_per_worker

    def test_configs_forwarded(self):
        """It forwards run_config and scaling_config unchanged."""
        run_config = object()
        scaling_config = object()
        with patch(_PARENT_INIT, return_value=None) as parent:
            XGBoostTrainer(
                _make_param(), run_config=run_config, scaling_config=scaling_config
            )

        assert parent.call_args.kwargs["run_config"] is run_config
        assert parent.call_args.kwargs["scaling_config"] is scaling_config

    def test_trainer_param_retained(self):
        """It keeps the trainer param on the instance."""
        param = _make_param()
        with patch(_PARENT_INIT, return_value=None):
            trainer = XGBoostTrainer(param)
        assert trainer.trainer_param is param


# -----------------------------------------------------------------------------
# XGBoostTrainer.train
# -----------------------------------------------------------------------------


def _build_trainer() -> XGBoostTrainer:
    """Construct a trainer with Ray's ``__init__`` patched out."""
    with patch(_PARENT_INIT, return_value=None):
        return XGBoostTrainer(_make_param())


class TestXGBoostTrainerTrain:
    """``train()`` result contract and override handling."""

    def test_returns_result_dict(self):
        """It returns the checkpoint path, the run path and the metrics."""
        trainer = _build_trainer()
        result = SimpleNamespace(
            error=None,
            checkpoint=SimpleNamespace(path="/ckpt/latest"),
            path="/run/path",
            metrics={"validation-rmse": 0.5},
        )
        with patch.object(XGBoostTrainer, "fit", return_value=result):
            out = trainer.train()

        assert out == {
            CHECKPOINT_PATH_KEY: "/ckpt/latest",
            "path": "/run/path",
            "metrics": {"validation-rmse": 0.5},
        }

    def test_missing_checkpoint_yields_none(self):
        """It returns None for the checkpoint path when there is no checkpoint."""
        trainer = _build_trainer()
        result = SimpleNamespace(
            error=None, checkpoint=None, path="/run/path", metrics={}
        )
        with patch.object(XGBoostTrainer, "fit", return_value=result):
            out = trainer.train()

        assert out[CHECKPOINT_PATH_KEY] is None

    def test_error_is_raised(self):
        """It raises the error Ray reports on the result."""
        trainer = _build_trainer()
        boom = RuntimeError("training failed")
        result = SimpleNamespace(
            error=boom, checkpoint=None, path="/run/path", metrics={}
        )
        with (
            patch.object(XGBoostTrainer, "fit", return_value=result),
            pytest.raises(RuntimeError, match="training failed"),
        ):
            trainer.train()

    def test_overrides_applied_before_fit(self):
        """It applies run_config and scaling_config overrides before fitting."""
        trainer = _build_trainer()
        run_config = object()
        scaling_config = object()
        result = SimpleNamespace(
            error=None,
            checkpoint=SimpleNamespace(path="/ckpt"),
            path="/run",
            metrics={},
        )
        with patch.object(XGBoostTrainer, "fit", return_value=result):
            trainer.train(run_config=run_config, scaling_config=scaling_config)

        assert trainer.run_config is run_config
        assert trainer.scaling_config is scaling_config

    def test_checkpoint_retained_on_instance(self):
        """It keeps the returned checkpoint on the instance."""
        trainer = _build_trainer()
        checkpoint = SimpleNamespace(path="/ckpt")
        result = SimpleNamespace(
            error=None, checkpoint=checkpoint, path="/run", metrics={}
        )
        with patch.object(XGBoostTrainer, "fit", return_value=result):
            trainer.train()

        assert trainer.checkpoint is checkpoint


# -----------------------------------------------------------------------------
# _train_loop_per_worker
# -----------------------------------------------------------------------------


def _base_config(**overrides) -> dict:
    """Build a ``train_loop_config`` for the worker loop."""
    config = {
        "label_column": "target",
        "feature_columns": None,
        "params": {"objective": "reg:squarederror"},
        "num_boost_round": 7,
        "xgboost_train_kwargs": {},
        "has_validation": False,
    }
    config.update(overrides)
    return config


class _FakeFrame:
    """Minimal pandas-like stand-in exposing ``columns`` and column selection."""

    def __init__(self, columns):
        self.columns = list(columns)
        self.selected = None

    def __getitem__(self, key):
        self.selected = key
        return f"frame[{key}]"


def _shard_returning(frame):
    """Build a dataset-shard mock whose ``materialize().to_pandas()`` is ``frame``."""
    shard = MagicMock()
    shard.materialize.return_value.to_pandas.return_value = frame
    return shard


@pytest.fixture
def fake_xgboost(monkeypatch):
    """Install a fake ``xgboost`` module and report callback for the worker loop."""
    fake = MagicMock(name="xgboost")
    monkeypatch.setitem(sys.modules, "xgboost", fake)
    with patch("ray.train.xgboost.RayTrainReportCallback", MagicMock()):
        yield fake


class TestTrainLoopPerWorker:
    """Worker-side training loop behavior."""

    def test_infers_feature_columns_excluding_label(self, fake_xgboost):
        """It treats every column except the label as a feature."""
        frame = _FakeFrame(["a", "b", "target"])
        with patch("ray.train.get_dataset_shard", return_value=_shard_returning(frame)):
            _train_loop_per_worker(_base_config())

        assert frame.selected == "target"
        first_dmatrix_call = fake_xgboost.DMatrix.call_args_list[0]
        assert first_dmatrix_call.args[0] == "frame[['a', 'b']]"

    def test_explicit_feature_columns_respected(self, fake_xgboost):
        """It uses the configured feature columns when they are given."""
        frame = _FakeFrame(["a", "b", "c", "target"])
        with patch("ray.train.get_dataset_shard", return_value=_shard_returning(frame)):
            _train_loop_per_worker(_base_config(feature_columns=["a", "c"]))

        first_dmatrix_call = fake_xgboost.DMatrix.call_args_list[0]
        assert first_dmatrix_call.args[0] == "frame[['a', 'c']]"

    def test_missing_label_column_raises(self, fake_xgboost):
        """It raises KeyError when the label column is absent from the shard."""
        frame = _FakeFrame(["a", "b"])
        with (
            patch("ray.train.get_dataset_shard", return_value=_shard_returning(frame)),
            pytest.raises(KeyError, match="target"),
        ):
            _train_loop_per_worker(_base_config(feature_columns=["a"]))

    def test_booster_params_forwarded(self, fake_xgboost):
        """It forwards params, rounds and train kwargs to ``xgboost.train``."""
        frame = _FakeFrame(["a", "target"])
        with patch("ray.train.get_dataset_shard", return_value=_shard_returning(frame)):
            _train_loop_per_worker(
                _base_config(xgboost_train_kwargs={"verbose_eval": False})
            )

        call = fake_xgboost.train.call_args
        assert call.args[0] == {"objective": "reg:squarederror"}
        assert call.kwargs["num_boost_round"] == 7
        assert call.kwargs["verbose_eval"] is False
        assert len(call.kwargs["callbacks"]) == 1

    def test_validation_shard_only_read_when_declared(self, fake_xgboost):
        """It reads the validation shard only when one was declared."""
        frame = _FakeFrame(["a", "target"])
        with patch(
            "ray.train.get_dataset_shard", return_value=_shard_returning(frame)
        ) as shard:
            _train_loop_per_worker(_base_config(has_validation=False))
        assert shard.call_count == 1

        with patch(
            "ray.train.get_dataset_shard", return_value=_shard_returning(frame)
        ) as shard:
            _train_loop_per_worker(_base_config(has_validation=True))
        assert shard.call_count == 2
        assert shard.call_args_list[1].args[0] == VALIDATION_DATASET_KEY

    def test_eval_sets_include_validation(self, fake_xgboost):
        """It evaluates on both train and validation when both exist."""
        frame = _FakeFrame(["a", "target"])
        with patch("ray.train.get_dataset_shard", return_value=_shard_returning(frame)):
            _train_loop_per_worker(_base_config(has_validation=True))

        evals = fake_xgboost.train.call_args.kwargs["evals"]
        assert [name for _, name in evals] == [
            TRAIN_DATASET_KEY,
            VALIDATION_DATASET_KEY,
        ]
