"""Tests for custom dataloader support in the tabular trainer task."""

from __future__ import annotations

from unittest import TestCase
from unittest.mock import Mock, patch

import numpy as np
import torch

from michelangelo.lib.trainer.torch.pytorch_lightning.schema import (
    CustomDataloaderParam,
)
from michelangelo.workflow.schema.custom_dataloader import (
    CustomDataloaderConfig,
    CustomDataloaderKind,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.tabular_trainer import (
    ColumnConfig,
    DataloadingConfig,
    TabularTrainerConfig,
)
from michelangelo.workflow.tasks.tabular_trainer._private.dataset import (
    get_sample_data_from_dataloader,
)
from michelangelo.workflow.tasks.tabular_trainer.task import (
    _build_custom_dataloader_param,
    _derive_custom_sample_data,
    train_tabular,
)
from michelangelo.workflow.tasks.tabular_trainer.tests.fixtures import (
    make_lightning_config,
    mock_train_dataset,
    mock_validation_dataset,
)

_TASK = "michelangelo.workflow.tasks.tabular_trainer.task"
_INPUTS = {"x": ColumnConfig("torch.float32", [1])}


def _batches(**columns):
    return [dict(columns)]


class TestGetSampleDataFromDataloader(TestCase):
    """Tests for get_sample_data_from_dataloader."""

    def test_takes_first_row_of_first_batch(self):
        """It builds sample data from row 0 of the first batch."""
        loader = _batches(x=torch.tensor([[1.0], [2.0]]), label=torch.tensor([0, 1]))
        sample = get_sample_data_from_dataloader(loader, _INPUTS)
        self.assertEqual(len(sample), 1)
        np.testing.assert_array_equal(sample[0]["x"], np.array([1.0], dtype="float32"))
        self.assertNotIn("label", sample[0])

    def test_accepts_list_form_input_columns(self):
        """It accepts named ColumnConfig lists."""
        loader = _batches(x=torch.tensor([[1.0]]))
        cols = [ColumnConfig("torch.float32", [1], name="x")]
        self.assertIn("x", get_sample_data_from_dataloader(loader, cols)[0])

    def test_empty_loader_raises(self):
        """It raises when no batches are produced."""
        with self.assertRaisesRegex(ConfigurationError, "no batches"):
            get_sample_data_from_dataloader([], _INPUTS)

    def test_non_dict_batch_raises(self):
        """It raises when a batch is not a dict."""
        with self.assertRaisesRegex(ConfigurationError, "dict"):
            get_sample_data_from_dataloader([torch.zeros(2, 1)], _INPUTS)

    def test_non_tensor_column_raises(self):
        """It raises when an input column is not a tensor."""
        with self.assertRaisesRegex(ConfigurationError, "batched tensor"):
            get_sample_data_from_dataloader(_batches(x=[1.0, 2.0]), _INPUTS)

    def test_zero_dim_tensor_raises(self):
        """It raises when an input column has no batch dimension."""
        with self.assertRaisesRegex(ConfigurationError, "batch dimension"):
            get_sample_data_from_dataloader(_batches(x=torch.tensor(1.0)), _INPUTS)

    def test_no_input_columns_in_batch_raises(self):
        """It raises when the batch holds none of the input columns."""
        with self.assertRaisesRegex(ConfigurationError, "no tensor values"):
            get_sample_data_from_dataloader(_batches(other=torch.ones(2, 1)), _INPUTS)


class TestBuildCustomDataloaderParam(TestCase):
    """Tests for _build_custom_dataloader_param."""

    def _build(self, kind):
        cfg = CustomDataloaderConfig(
            build_dataloader_fn="pkg.build",
            build_dataloader_kwargs={"k": 1},
            kind=kind,
        )
        train, val = Mock(path="s3://b/train"), Mock(path="s3://b/val")
        factory = Mock(name="factory")
        with patch(f"{_TASK}.get_module_attr", return_value=factory) as resolve:
            param = _build_custom_dataloader_param(cfg, train, val, {"columns": ["x"]})
        resolve.assert_called_once_with("pkg.build")
        return param, factory

    def test_ray_backed(self):
        """It maps a Ray-backed config to a non-file-backed param."""
        param, factory = self._build(CustomDataloaderKind.RAY_DATA)
        self.assertIs(param.factory, factory)
        self.assertFalse(param.file_backed)
        self.assertEqual(param.train_dataset_path, "s3://b/train")
        self.assertEqual(param.validation_dataset_path, "s3://b/val")
        self.assertEqual(param.factory_kwargs, {"k": 1})
        self.assertEqual(param.read_kwargs, {"columns": ["x"]})

    def test_file_backed(self):
        """It maps a file-backed config to a file-backed param."""
        param, _ = self._build(CustomDataloaderKind.FILE_BACKED)
        self.assertTrue(param.file_backed)


class TestDeriveCustomSampleData(TestCase):
    """Tests for _derive_custom_sample_data."""

    def _param(self, factory, file_backed=False):
        return CustomDataloaderParam(
            factory=factory,
            train_dataset_path="s3://b/train",
            validation_dataset_path="s3://b/val",
            factory_kwargs={"k": 1},
            read_kwargs={"columns": ["x"]},
            file_backed=file_backed,
        )

    def test_builds_train_loader_when_no_sample_fn(self):
        """It samples from the first batch of the factory's train loader."""
        factory = Mock(return_value=_batches(x=torch.tensor([[3.0]])))
        ds = Mock()
        sample = _derive_custom_sample_data(self._param(factory), None, {}, ds, _INPUTS)
        factory.assert_called_once_with(
            stage="train",
            ds=ds,
            dataset_path="s3://b/train",
            read_kwargs={"columns": ["x"]},
            k=1,
        )
        np.testing.assert_array_equal(sample[0]["x"], np.array([3.0], dtype="float32"))

    def test_file_backed_omits_ds(self):
        """File-backed sampling never passes ``ds``."""
        factory = Mock(return_value=_batches(x=torch.tensor([[3.0]])))
        _derive_custom_sample_data(
            self._param(factory, file_backed=True), None, {}, None, _INPUTS
        )
        self.assertNotIn("ds", factory.call_args.kwargs)

    def test_sample_fn_skips_train_loader(self):
        """A sample function replaces the train loader and gets its own kwargs."""
        factory = Mock()
        sample_fn = Mock(return_value=[{"x": np.zeros(1)}])
        ds = Mock()
        out = _derive_custom_sample_data(
            self._param(factory), sample_fn, {"n": 2}, ds, _INPUTS
        )
        factory.assert_not_called()
        sample_fn.assert_called_once_with(
            ds=ds,
            dataset_path="s3://b/train",
            read_kwargs={"columns": ["x"]},
            n=2,
        )
        self.assertEqual(out, sample_fn.return_value)

    def test_sample_fn_file_backed_omits_ds(self):
        """File-backed sample functions do not receive ``ds``."""
        sample_fn = Mock(return_value=[])
        _derive_custom_sample_data(
            self._param(Mock(), file_backed=True), sample_fn, {}, None, _INPUTS
        )
        self.assertNotIn("ds", sample_fn.call_args.kwargs)


def _custom_config(kind=CustomDataloaderKind.RAY_DATA, **kwargs):
    return make_lightning_config(
        dataloading_config=DataloadingConfig(
            custom_dataloader_config=CustomDataloaderConfig(
                build_dataloader_fn="pkg.build", kind=kind, **kwargs
            )
        )
    )


def _run(lightning_config, train_ds=None, val_ds=None):
    """Run train_tabular with a custom loader and all heavy deps mocked."""
    train_ds = train_ds or mock_train_dataset()
    val_ds = val_ds or mock_validation_dataset()
    train_ds.path, val_ds.path = "s3://b/train", "s3://b/val"
    factory = Mock(return_value=_batches(x=torch.tensor([[1.0]])))
    with (
        patch(f"{_TASK}.get_module_attr", return_value=factory),
        patch(f"{_TASK}.ModelVariable"),
        patch(f"{_TASK}.LightningTrainerParam") as param_cls,
        patch(f"{_TASK}.LightningTrainerWithStateDict"),
    ):
        train_tabular(
            TabularTrainerConfig(lightning=lightning_config), train_ds, val_ds
        )
    return param_cls, train_ds, val_ds, factory


class TestTrainTabularCustomDataloader(TestCase):
    """End-to-end wiring of custom dataloaders through train_tabular."""

    def test_ray_backed_loads_datasets_and_passes_param(self):
        """Ray-backed mode still loads datasets and hands the factory to the trainer."""
        param_cls, train_ds, val_ds, factory = _run(_custom_config())
        train_ds.load_ray_dataset.assert_called_once()
        val_ds.load_ray_dataset.assert_called_once()
        kwargs = param_cls.call_args.kwargs
        self.assertIs(kwargs["train_data"], train_ds.value)
        custom = kwargs["custom_dataloader"]
        self.assertIs(custom.factory, factory)
        self.assertFalse(custom.file_backed)
        self.assertEqual(custom.train_dataset_path, "s3://b/train")
        self.assertEqual(custom.read_kwargs, {"columns": ["label", "x"]})

    def test_file_backed_skips_ray_dataset_registration(self):
        """File-backed mode never loads Ray datasets."""
        param_cls, train_ds, val_ds, factory = _run(
            _custom_config(CustomDataloaderKind.FILE_BACKED)
        )
        train_ds.load_ray_dataset.assert_not_called()
        val_ds.load_ray_dataset.assert_not_called()
        kwargs = param_cls.call_args.kwargs
        self.assertIsNone(kwargs["train_data"])
        self.assertIsNone(kwargs["val_data"])
        self.assertTrue(kwargs["custom_dataloader"].file_backed)
        self.assertNotIn("ds", factory.call_args.kwargs)

    def test_sample_data_comes_from_loader_not_dataset_take(self):
        """The Ray dataset is not sampled with ``take`` when a custom loader is set."""
        _, train_ds, _, _ = _run(_custom_config())
        train_ds.value.take.assert_not_called()

    def test_default_path_has_no_custom_dataloader(self):
        """Without a custom config the trainer param has no custom loader."""
        train_ds, val_ds = mock_train_dataset(), mock_validation_dataset()
        with (
            patch(f"{_TASK}.get_module_attr", return_value=lambda **kw: Mock()),
            patch(f"{_TASK}.ModelVariable"),
            patch(f"{_TASK}.LightningTrainerParam") as param_cls,
            patch(f"{_TASK}.LightningTrainerWithStateDict"),
        ):
            train_tabular(
                TabularTrainerConfig(lightning=make_lightning_config()),
                train_ds,
                val_ds,
            )
        self.assertIsNone(param_cls.call_args.kwargs["custom_dataloader"])
        train_ds.value.take.assert_called_once()
