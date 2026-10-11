"""Tests for custom dataloader support in the Lightning trainer."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

from michelangelo.lib._internal.errors import UserInputError
from michelangelo.lib.trainer.torch.pytorch_lightning._private import util
from michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer import (
    LightningTrainer,
    LightningTrainerParam,
)
from michelangelo.lib.trainer.torch.pytorch_lightning.schema import (
    CustomDataloaderParam,
    DataLoaderFactory,
)

_TORCH_TRAINER_INIT = (
    "michelangelo.lib.trainer.torch.pytorch_lightning."
    "lightning_trainer.TorchTrainer.__init__"
)
_UTIL = "michelangelo.lib.trainer.torch.pytorch_lightning._private.util"


def _recording_factory(calls: list):
    """Return a factory that records its call and returns a sentinel loader."""

    def factory(stage, *, dataset_path, read_kwargs, **kwargs):
        calls.append(
            {
                "stage": stage,
                "dataset_path": dataset_path,
                "read_kwargs": read_kwargs,
                "kwargs": kwargs,
            }
        )
        return f"loader-{stage}"

    return factory


def _custom(factory=None, **overrides) -> CustomDataloaderParam:
    defaults = {
        "factory": factory or _recording_factory([]),
        "train_dataset_path": "memory://train",
        "validation_dataset_path": "memory://val",
    }
    defaults.update(overrides)
    return CustomDataloaderParam(**defaults)


def _make_param(**overrides) -> LightningTrainerParam:
    defaults = {
        "create_model_fn": MagicMock(name="create_model_fn"),
        "create_model_fn_kwargs": {},
        "train_data": MagicMock(name="train_data"),
        "val_data": MagicMock(name="val_data"),
    }
    defaults.update(overrides)
    return LightningTrainerParam(**defaults)


class TestCustomDataloaderParam:
    """Defaults and protocol conformance."""

    def test_defaults(self):
        """Optional fields default to an empty, Ray-backed configuration."""
        custom = _custom()
        assert custom.factory_kwargs == {}
        assert custom.read_kwargs == {}
        assert custom.file_backed is False

    def test_function_satisfies_protocol(self):
        """A plain function with the documented signature is a DataLoaderFactory."""
        assert isinstance(_recording_factory([]), DataLoaderFactory)


class TestLightningTrainerParamValidation:
    """``train_data`` / ``val_data`` may be omitted only for file-backed loaders."""

    @pytest.mark.parametrize("missing", ["train_data", "val_data"])
    def test_missing_data_without_custom_loader_raises(self, missing):
        """Default mode still requires both datasets."""
        with pytest.raises(UserInputError, match="file-backed"):
            _make_param(**{missing: None})

    @pytest.mark.parametrize("missing", ["train_data", "val_data"])
    def test_missing_data_with_ray_backed_loader_raises(self, missing):
        """Ray-backed custom loaders still need the Ray datasets."""
        with pytest.raises(UserInputError):
            _make_param(custom_dataloader=_custom(), **{missing: None})

    def test_file_backed_allows_no_datasets(self):
        """File-backed mode needs no Ray datasets."""
        param = _make_param(
            train_data=None,
            val_data=None,
            custom_dataloader=_custom(file_backed=True),
        )
        assert param.train_data is None
        assert param.val_data is None


class TestLightningTrainerInitCustomLoader:
    """``LightningTrainer.__init__`` wiring for custom dataloaders."""

    def _init(self, param, **kwargs):
        with patch(_TORCH_TRAINER_INIT, return_value=None) as mock_super:
            trainer = LightningTrainer(trainer_param=param, **kwargs)
        return trainer, mock_super.call_args.kwargs

    def test_default_has_no_custom_dataloader_in_config(self):
        """Without a custom loader the loop config carries no entry for it."""
        _, kwargs = self._init(_make_param())
        assert "custom_dataloader" not in kwargs["train_loop_config"]

    def test_ray_backed_keeps_datasets_and_reinjects_original(self):
        """The factory object is re-injected unchanged and datasets are kept."""
        train_ds, val_ds = MagicMock(), MagicMock()
        custom = _custom()
        _, kwargs = self._init(
            _make_param(train_data=train_ds, val_data=val_ds, custom_dataloader=custom)
        )
        assert kwargs["datasets"] == {"train": train_ds, "val": val_ds}
        assert kwargs["train_loop_config"]["custom_dataloader"] is custom

    def test_file_backed_passes_no_datasets(self):
        """File-backed mode hands Ray Train an empty datasets mapping."""
        param = _make_param(
            train_data=None,
            val_data=None,
            custom_dataloader=_custom(file_backed=True),
        )
        _, kwargs = self._init(param)
        assert kwargs["datasets"] == {}

    def test_limit_batches_do_not_cap_custom_datasets(self):
        """``limit_*_batches`` are left to Lightning and never cap a Ray Dataset."""
        param = _make_param(
            custom_dataloader=_custom(),
            lightning_trainer_kwargs={"limit_train_batches": 3},
        )
        with patch(
            "michelangelo.lib.trainer.torch.pytorch_lightning."
            "lightning_trainer._apply_batch_limit",
            side_effect=lambda ds, limit, *_: (ds, limit),
        ) as apply_limit:
            _, kwargs = self._init(param)
        assert apply_limit.call_args_list[0].args[1] is None
        assert set(kwargs["datasets"]) == {"train", "val"}
        loop_kwargs = kwargs["train_loop_config"]["lightning_trainer_kwargs"]
        assert loop_kwargs["limit_train_batches"] == 3

    def test_file_backed_with_limit_batches_does_not_touch_none_dataset(self):
        """File-backed runs tolerate ``limit_*_batches`` with no dataset to cap."""
        param = _make_param(
            train_data=None,
            val_data=None,
            custom_dataloader=_custom(file_backed=True),
            lightning_trainer_kwargs={"limit_train_batches": 3},
        )
        _, kwargs = self._init(param)
        assert kwargs["datasets"] == {}

    def test_profiler_row_count_skipped_with_custom_loader(self):
        """The profiler row count is not computed for custom loaders."""
        train_ds = MagicMock(name="train")
        param = _make_param(
            train_data=train_ds,
            custom_dataloader=_custom(),
            lightning_trainer_kwargs={"profiler": {"type": "simple"}},
        )
        with patch(
            "michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer._logger"
        ) as logger:
            _, kwargs = self._init(param)
        train_ds.count.assert_not_called()
        assert "train_dataset_num_rows" not in kwargs["train_loop_config"]
        assert any(
            "row-count validation" in call.args[0]
            for call in logger.info.call_args_list
        )

    def test_profiler_row_count_still_computed_by_default(self):
        """Default mode keeps counting rows for the profiler schedule."""
        train_ds = MagicMock(name="train")
        train_ds.count.return_value = 40
        param = _make_param(
            train_data=train_ds,
            lightning_trainer_kwargs={"profiler": {"type": "simple"}},
        )
        _, kwargs = self._init(param)
        assert kwargs["train_loop_config"]["train_dataset_num_rows"] == 40


class TestBuildCustomDataloaders:
    """``_build_custom_dataloaders`` stage dispatch."""

    def test_ray_backed_passes_shards_as_ds(self):
        """Each stage gets its own Ray Data shard, paths and kwargs."""
        calls: list = []
        custom = _custom(
            _recording_factory(calls),
            factory_kwargs={"num_workers": 2},
            read_kwargs={"columns": ["a"]},
        )
        with patch(f"{_UTIL}.ray.train.get_dataset_shard") as get_shard:
            get_shard.side_effect = lambda key: f"shard-{key}"
            train, val = util._build_custom_dataloaders(custom)

        assert (train, val) == ("loader-train", "loader-validation")
        get_shard.assert_any_call("train")
        get_shard.assert_any_call("val")
        assert calls[0] == {
            "stage": "train",
            "dataset_path": "memory://train",
            "read_kwargs": {"columns": ["a"]},
            "kwargs": {"ds": "shard-train", "num_workers": 2},
        }
        assert calls[1]["stage"] == "validation"
        assert calls[1]["dataset_path"] == "memory://val"
        assert calls[1]["kwargs"]["ds"] == "shard-val"

    def test_file_backed_omits_ds_and_skips_shard_lookup(self):
        """File-backed mode never touches the Ray dataset shards."""
        calls: list = []
        custom = _custom(_recording_factory(calls), file_backed=True)
        with patch(f"{_UTIL}.ray.train.get_dataset_shard") as get_shard:
            util._build_custom_dataloaders(custom)

        get_shard.assert_not_called()
        assert all("ds" not in call["kwargs"] for call in calls)
        assert [call["stage"] for call in calls] == ["train", "validation"]

    def test_factory_returning_none_raises(self):
        """A factory that returns ``None`` is reported with its stage."""
        custom = _custom(lambda **_: None, file_backed=True)
        with pytest.raises(UserInputError, match="'train'"):
            util._build_custom_dataloaders(custom)


class TestTrainLoopDispatch:
    """``_train_loop_per_worker`` chooses the loader source."""

    def _run(self, config_overrides):
        config = {
            "batch_size": 4,
            "num_epochs": 1,
            "num_shuffle_batches": 0,
            "create_model_fn": MagicMock(),
            "create_model_fn_kwargs": {},
            "data_collate_fn": None,
        }
        config.update(config_overrides)
        fit = MagicMock()
        with (
            patch(f"{_UTIL}.ray.train.get_context") as ctx,
            patch(f"{_UTIL}.ray.train.get_dataset_shard") as get_shard,
            patch(f"{_UTIL}.ray.train.get_checkpoint", return_value=None),
            patch(f"{_UTIL}.pl.Trainer") as trainer_cls,
            patch(f"{_UTIL}._prepare_trainer_for_ray", side_effect=lambda t: t),
            patch(f"{_UTIL}._resolve_strategy", return_value=None),
            patch(f"{_UTIL}._resolve_plugins", return_value=None),
            patch(f"{_UTIL}._resolve_logger", return_value=None),
            patch(f"{_UTIL}._resolve_callbacks", return_value=([], False)),
            patch(f"{_UTIL}._resolve_profiler", return_value=(None, None, False)),
            patch(f"{_UTIL}._maybe_export_profiler_results"),
        ):
            ctx.return_value.get_world_rank.return_value = 0
            ctx.return_value.get_world_size.return_value = 1
            trainer_cls.return_value.fit = fit
            util._train_loop_per_worker(config)
        return fit, get_shard

    def test_default_uses_dataset_shards(self):
        """Without a custom loader the Ray shards feed ``iter_torch_batches``."""
        fit, get_shard = self._run({})
        assert {c.args[0] for c in get_shard.call_args_list} == {"train", "val"}
        kwargs = fit.call_args.kwargs
        assert kwargs["train_dataloaders"] is (
            get_shard.return_value.iter_torch_batches.return_value
        )

    def test_custom_loader_feeds_trainer_fit(self):
        """Custom loaders are passed to ``Trainer.fit`` in place of the shards."""
        calls: list = []
        fit, _ = self._run(
            {"custom_dataloader": _custom(_recording_factory(calls), file_backed=True)}
        )
        kwargs = fit.call_args.kwargs
        assert kwargs["train_dataloaders"] == "loader-train"
        assert kwargs["val_dataloaders"] == "loader-validation"

    def test_file_backed_does_not_request_shards(self):
        """No dataset shard is requested in file-backed mode."""
        _, get_shard = self._run({"custom_dataloader": _custom(file_backed=True)})
        get_shard.assert_not_called()
