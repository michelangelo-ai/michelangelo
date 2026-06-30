"""Tests for michelangelo.workflow.schema.tabular_trainer config dataclasses."""

from __future__ import annotations

from unittest import TestCase

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.tabular_trainer import (
    BatchIterConfig,
    CheckpointConfig,
    CheckpointScoreOrder,
    ColumnConfig,
    CometConfig,
    CustomTrainerConfig,
    DataloadingConfig,
    IncrementalTrainingModeConfig,
    LightningTrainerConfig,
    LightningTrainerKwargs,
    ParquetReadConfig,
    ScalingConfig,
    TabularTrainerConfig,
)

# ---------------------------------------------------------------------------
# Enums
# ---------------------------------------------------------------------------


class TestCheckpointScoreOrder(TestCase):
    """Tests for CheckpointScoreOrder enum."""

    def test_values(self):
        """It exposes 'max' and 'min' string values."""
        self.assertEqual(CheckpointScoreOrder.MAX.value, "max")
        self.assertEqual(CheckpointScoreOrder.MIN.value, "min")

    def test_is_str_subclass(self):
        """It is a str subclass and compares equal to its value."""
        self.assertEqual(CheckpointScoreOrder.MAX, "max")
        self.assertIsInstance(CheckpointScoreOrder.MIN, str)


class TestIncrementalTrainingModeConfig(TestCase):
    """Tests for IncrementalTrainingModeConfig enum."""

    def test_values(self):
        """It exposes 'NONE' and 'BASELINE' string values."""
        self.assertEqual(IncrementalTrainingModeConfig.NONE.value, "NONE")
        self.assertEqual(IncrementalTrainingModeConfig.BASELINE.value, "BASELINE")

    def test_is_str_subclass(self):
        """It is a str subclass."""
        self.assertIsInstance(IncrementalTrainingModeConfig.NONE, str)


# ---------------------------------------------------------------------------
# ColumnConfig
# ---------------------------------------------------------------------------


class TestColumnConfig(TestCase):
    """Tests for ColumnConfig dataclass."""

    def test_required_data_type(self):
        """It stores data_type and defaults shape to []."""
        cfg = ColumnConfig(data_type="torch.float32")
        self.assertEqual(cfg.data_type, "torch.float32")
        self.assertEqual(cfg.shape, [])

    def test_shape_stored(self):
        """It stores an explicit shape."""
        cfg = ColumnConfig(data_type="torch.long", shape=[128])
        self.assertEqual(cfg.shape, [128])

    def test_shape_instances_are_independent(self):
        """Default shape lists are not shared between instances."""
        a = ColumnConfig(data_type="torch.float32")
        b = ColumnConfig(data_type="torch.float32")
        a.shape.append(1)
        self.assertEqual(b.shape, [])


# ---------------------------------------------------------------------------
# CometConfig
# ---------------------------------------------------------------------------


class TestCometConfig(TestCase):
    """Tests for CometConfig dataclass."""

    def test_all_fields_stored(self):
        """It stores all four required fields."""
        cfg = CometConfig(
            api_key="k", workspace="ws", project_name="proj", experiment_name="exp"
        )
        self.assertEqual(cfg.api_key, "k")
        self.assertEqual(cfg.workspace, "ws")
        self.assertEqual(cfg.project_name, "proj")
        self.assertEqual(cfg.experiment_name, "exp")


# ---------------------------------------------------------------------------
# ScalingConfig
# ---------------------------------------------------------------------------


class TestScalingConfig(TestCase):
    """Tests for ScalingConfig dataclass."""

    def test_default_cpu_per_worker(self):
        """cpu_per_worker defaults to 1."""
        self.assertEqual(ScalingConfig().cpu_per_worker, 1)

    def test_explicit_cpu_per_worker(self):
        """It stores an explicit value."""
        self.assertEqual(ScalingConfig(cpu_per_worker=4).cpu_per_worker, 4)


# ---------------------------------------------------------------------------
# CheckpointConfig
# ---------------------------------------------------------------------------


class TestCheckpointConfig(TestCase):
    """Tests for CheckpointConfig dataclass."""

    def test_defaults(self):
        """It defaults num_to_keep=1, MAX order, no attribute or steps."""
        cfg = CheckpointConfig()
        self.assertEqual(cfg.num_to_keep, 1)
        self.assertIsNone(cfg.checkpoint_score_attribute)
        self.assertEqual(cfg.checkpoint_score_order, CheckpointScoreOrder.MAX)
        self.assertIsNone(cfg.save_every_n_steps)
        self.assertIsNone(cfg.random_seed)

    def test_explicit_fields(self):
        """It stores explicit values for all fields."""
        cfg = CheckpointConfig(
            num_to_keep=3,
            checkpoint_score_attribute="val_loss",
            checkpoint_score_order=CheckpointScoreOrder.MIN,
            random_seed=42,
        )
        self.assertEqual(cfg.num_to_keep, 3)
        self.assertEqual(cfg.checkpoint_score_attribute, "val_loss")
        self.assertEqual(cfg.checkpoint_score_order, CheckpointScoreOrder.MIN)
        self.assertEqual(cfg.random_seed, 42)

    def test_save_every_n_steps_zero_raises(self):
        """save_every_n_steps=0 raises ConfigurationError."""
        with self.assertRaises(
            ConfigurationError, msg="save_every_n_steps must be >= 1"
        ):
            CheckpointConfig(save_every_n_steps=0)

    def test_save_every_n_steps_negative_raises(self):
        """Negative save_every_n_steps raises ConfigurationError."""
        with self.assertRaises(ConfigurationError):
            CheckpointConfig(save_every_n_steps=-5)

    def test_save_every_n_steps_one_ok(self):
        """save_every_n_steps=1 is valid."""
        cfg = CheckpointConfig(save_every_n_steps=1)
        self.assertEqual(cfg.save_every_n_steps, 1)

    def test_save_every_n_steps_none_ok(self):
        """save_every_n_steps=None (default) does not raise."""
        CheckpointConfig(save_every_n_steps=None)


# ---------------------------------------------------------------------------
# ParquetReadConfig
# ---------------------------------------------------------------------------


class TestParquetReadConfig(TestCase):
    """Tests for ParquetReadConfig dataclass."""

    def test_all_none_by_default(self):
        """All optional fields default to None."""
        cfg = ParquetReadConfig()
        for attr in (
            "num_cpus",
            "num_gpus",
            "memory",
            "concurrency",
            "override_num_blocks",
            "shuffle",
            "tensor_column_schema",
            "arrow_parquet_args",
        ):
            self.assertIsNone(getattr(cfg, attr), msg=f"{attr} should be None")

    def test_fields_stored(self):
        """It stores provided values."""
        cfg = ParquetReadConfig(num_cpus=2.0, shuffle="files", concurrency=4)
        self.assertEqual(cfg.num_cpus, 2.0)
        self.assertEqual(cfg.shuffle, "files")
        self.assertEqual(cfg.concurrency, 4)


# ---------------------------------------------------------------------------
# BatchIterConfig
# ---------------------------------------------------------------------------


class TestBatchIterConfig(TestCase):
    """Tests for BatchIterConfig dataclass."""

    def test_required_batch_size(self):
        """batch_size is required."""
        cfg = BatchIterConfig(batch_size=64)
        self.assertEqual(cfg.batch_size, 64)
        self.assertEqual(cfg.num_shuffle_batches, 0)
        self.assertIsNone(cfg.collate_fn)

    def test_all_fields(self):
        """It stores all fields."""
        cfg = BatchIterConfig(
            batch_size=32,
            num_shuffle_batches=4,
            collate_fn="myproject.collate.fn",
        )
        self.assertEqual(cfg.num_shuffle_batches, 4)
        self.assertEqual(cfg.collate_fn, "myproject.collate.fn")


# ---------------------------------------------------------------------------
# DataloadingConfig
# ---------------------------------------------------------------------------


class TestDataloadingConfig(TestCase):
    """Tests for DataloadingConfig dataclass."""

    def test_all_none_by_default(self):
        """Both optional fields default to None."""
        cfg = DataloadingConfig()
        self.assertIsNone(cfg.parquet_read_config)
        self.assertIsNone(cfg.batch_iter_config)

    def test_fields_stored(self):
        """It stores provided sub-configs."""
        pr = ParquetReadConfig(shuffle="files")
        bi = BatchIterConfig(batch_size=32)
        cfg = DataloadingConfig(parquet_read_config=pr, batch_iter_config=bi)
        self.assertIs(cfg.parquet_read_config, pr)
        self.assertIs(cfg.batch_iter_config, bi)


# ---------------------------------------------------------------------------
# LightningTrainerKwargs
# ---------------------------------------------------------------------------


class TestLightningTrainerKwargs(TestCase):
    """Tests for LightningTrainerKwargs validation and defaults."""

    def test_defaults(self):
        """All optional fields default correctly."""
        cfg = LightningTrainerKwargs()
        self.assertIsNone(cfg.strategy)
        self.assertIsNone(cfg.precision)
        self.assertEqual(cfg.fast_dev_run, 0)
        self.assertEqual(cfg.max_steps, -1)
        self.assertTrue(cfg.inference_mode)
        self.assertTrue(cfg.use_distributed_sampler)
        self.assertFalse(cfg.detect_anomaly)
        self.assertFalse(cfg.barebones)
        self.assertEqual(cfg.accumulate_grad_batches, 1)
        self.assertEqual(cfg.overfit_batches, 0.0)

    def test_explicit_fields_stored(self):
        """It stores explicit values."""
        cfg = LightningTrainerKwargs(
            strategy="ddp", max_epochs=10, precision="bf16-mixed"
        )
        self.assertEqual(cfg.strategy, "ddp")
        self.assertEqual(cfg.max_epochs, 10)
        self.assertEqual(cfg.precision, "bf16-mixed")

    def test_limit_train_both_raises(self):
        """Setting both limit_train_batches and limit_train_batches_count raises."""
        with self.assertRaises(ConfigurationError):
            LightningTrainerKwargs(
                limit_train_batches=0.5, limit_train_batches_count=100
            )

    def test_limit_val_both_raises(self):
        """Setting both limit_val_batches and limit_val_batches_count raises."""
        with self.assertRaises(ConfigurationError):
            LightningTrainerKwargs(limit_val_batches=0.1, limit_val_batches_count=10)

    def test_limit_test_both_raises(self):
        """Setting both limit_test_batches and limit_test_batches_count raises."""
        with self.assertRaises(ConfigurationError):
            LightningTrainerKwargs(limit_test_batches=0.2, limit_test_batches_count=5)

    def test_limit_predict_both_raises(self):
        """Setting both limit_predict_batches and limit_predict_batches_count raises."""
        with self.assertRaises(ConfigurationError):
            LightningTrainerKwargs(
                limit_predict_batches=0.3, limit_predict_batches_count=20
            )

    def test_limit_train_float_only_ok(self):
        """Setting only limit_train_batches (no count) is valid."""
        cfg = LightningTrainerKwargs(limit_train_batches=0.8)
        self.assertEqual(cfg.limit_train_batches, 0.8)
        self.assertIsNone(cfg.limit_train_batches_count)

    def test_limit_train_count_only_ok(self):
        """Setting only limit_train_batches_count (no float) is valid."""
        cfg = LightningTrainerKwargs(limit_train_batches_count=100)
        self.assertEqual(cfg.limit_train_batches_count, 100)
        self.assertIsNone(cfg.limit_train_batches)

    def test_mixed_pairs_ok(self):
        """Setting float for train and count for val (different pairs) is valid."""
        cfg = LightningTrainerKwargs(
            limit_train_batches=0.5, limit_val_batches_count=10
        )
        self.assertEqual(cfg.limit_train_batches, 0.5)
        self.assertEqual(cfg.limit_val_batches_count, 10)


# ---------------------------------------------------------------------------
# CustomTrainerConfig
# ---------------------------------------------------------------------------


class TestCustomTrainerConfig(TestCase):
    """Tests for CustomTrainerConfig dataclass."""

    def test_required_train_class(self):
        """train_class is stored; train_constructor_kwargs defaults to None."""
        cfg = CustomTrainerConfig(train_class="myproject.MyTrainer")
        self.assertEqual(cfg.train_class, "myproject.MyTrainer")
        self.assertIsNone(cfg.train_constructor_kwargs)

    def test_kwargs_stored(self):
        """train_constructor_kwargs is stored when provided."""
        cfg = CustomTrainerConfig(
            train_class="myproject.MyTrainer",
            train_constructor_kwargs={"lr": 0.01},
        )
        self.assertEqual(cfg.train_constructor_kwargs, {"lr": 0.01})


# ---------------------------------------------------------------------------
# LightningTrainerConfig
# ---------------------------------------------------------------------------


def _minimal_lightning_config(**overrides) -> LightningTrainerConfig:
    """Build a minimal valid LightningTrainerConfig."""
    defaults = {
        "model_class": "myproject.models.Net",
        "input_columns": {"x": ColumnConfig("torch.float32")},
        "output_columns": {"y": ColumnConfig("torch.float32")},
        "labels": {"label": ColumnConfig("torch.long")},
        "metadata_columns": [],
    }
    defaults.update(overrides)
    return LightningTrainerConfig(**defaults)


class TestLightningTrainerConfig(TestCase):
    """Tests for LightningTrainerConfig dataclass."""

    def test_required_fields_stored(self):
        """It stores all required fields and uses correct defaults."""
        cfg = _minimal_lightning_config()
        self.assertEqual(cfg.model_class, "myproject.models.Net")
        self.assertEqual(cfg.input_columns, {"x": ColumnConfig("torch.float32")})
        self.assertEqual(cfg.metadata_columns, [])
        self.assertIsInstance(cfg.checkpoint_config, CheckpointConfig)
        self.assertIsNone(cfg.model_kwargs)
        self.assertIsNone(cfg.dataloading_config)
        self.assertIsNone(cfg.scaling_config)
        self.assertIsNone(cfg.lightning_trainer_kwargs)
        self.assertIsNone(cfg.hyperparameters)
        self.assertIsNone(cfg.comet)
        self.assertIsNone(cfg.transfer_learning_spec)
        self.assertIsNone(cfg.incremental_training_mode)

    def test_optional_fields_stored(self):
        """It stores all optional fields when provided."""
        comet = CometConfig(
            api_key="k", workspace="ws", project_name="p", experiment_name="e"
        )
        scaling = ScalingConfig(cpu_per_worker=4)
        cfg = _minimal_lightning_config(
            comet=comet,
            scaling_config=scaling,
            hyperparameters={"lr": 0.001},
            incremental_training_mode=IncrementalTrainingModeConfig.BASELINE,
        )
        self.assertIs(cfg.comet, comet)
        self.assertIs(cfg.scaling_config, scaling)
        self.assertEqual(cfg.hyperparameters, {"lr": 0.001})
        self.assertEqual(
            cfg.incremental_training_mode, IncrementalTrainingModeConfig.BASELINE
        )

    def test_checkpoint_config_default_is_independent(self):
        """Default CheckpointConfig instances are not shared."""
        a = _minimal_lightning_config()
        b = _minimal_lightning_config()
        self.assertIsNot(a.checkpoint_config, b.checkpoint_config)


# ---------------------------------------------------------------------------
# TabularTrainerConfig
# ---------------------------------------------------------------------------


class TestTabularTrainerConfig(TestCase):
    """Tests for TabularTrainerConfig validation."""

    def _lightning_cfg(self) -> LightningTrainerConfig:
        return _minimal_lightning_config()

    def test_lightning_only_ok(self):
        """Setting only lightning is valid."""
        cfg = TabularTrainerConfig(lightning=self._lightning_cfg())
        self.assertIsNotNone(cfg.lightning)
        self.assertIsNone(cfg.custom)

    def test_custom_only_ok(self):
        """Setting only custom is valid."""
        cfg = TabularTrainerConfig(
            custom=CustomTrainerConfig(train_class="myproject.Trainer")
        )
        self.assertIsNone(cfg.lightning)
        self.assertIsNotNone(cfg.custom)

    def test_neither_raises(self):
        """Setting neither raises ConfigurationError."""
        with self.assertRaises(ConfigurationError):
            TabularTrainerConfig()

    def test_both_raises(self):
        """Setting both raises ConfigurationError."""
        with self.assertRaises(ConfigurationError):
            TabularTrainerConfig(
                lightning=self._lightning_cfg(),
                custom=CustomTrainerConfig(train_class="myproject.Trainer"),
            )

    def test_error_message_neither(self):
        """ConfigurationError message mentions 'lightning' and 'custom'."""
        with self.assertRaises(ConfigurationError) as ctx:
            TabularTrainerConfig()
        self.assertIn("lightning", str(ctx.exception))
        self.assertIn("custom", str(ctx.exception))
