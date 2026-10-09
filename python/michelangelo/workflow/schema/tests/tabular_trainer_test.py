"""Tests for michelangelo.workflow.schema.tabular_trainer config dataclasses."""

from __future__ import annotations

from typing import ClassVar
from unittest import TestCase

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.tabular_trainer import (
    CheckpointConfig,
    CheckpointScoreOrder,
    ColumnConfig,
    CometConfig,
    CustomTrackerConfig,
    CustomTrainerConfig,
    ExperimentTrackerConfig,
    IncrementalTrainingModeConfig,
    LightningTrainerConfig,
    LightningTrainerKwargs,
    MlflowConfig,
    ScalingConfig,
    TabularTrainerConfig,
    TrackerConfig,
    column_names,
    normalize_columns,
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

    def test_shape_defaults_to_empty_list(self):
        """``shape`` defaults to ``[]`` (scalar) when omitted."""
        cfg = ColumnConfig(data_type="torch.float32")
        self.assertEqual(cfg.shape, [])

    def test_shape_stored(self):
        """It stores an explicit shape."""
        cfg = ColumnConfig(data_type="torch.long", shape=[128])
        self.assertEqual(cfg.data_type, "torch.long")
        self.assertEqual(cfg.shape, [128])

    def test_shape_default_is_not_shared_between_instances(self):
        """Each instance gets its own default list, not a shared mutable one."""
        cfg1 = ColumnConfig(data_type="torch.float32")
        cfg2 = ColumnConfig(data_type="torch.float32")
        cfg1.shape.append(1)
        self.assertEqual(cfg1.shape, [1])
        self.assertEqual(cfg2.shape, [])


# ---------------------------------------------------------------------------
# TrackerConfig
# ---------------------------------------------------------------------------


class TestTrackerConfig(TestCase):
    """Tests for the TrackerConfig base class."""

    def test_default_oss_supported_true(self):
        """Base class defaults to _oss_supported=True and does not raise."""
        cfg = TrackerConfig()
        self.assertTrue(cfg._oss_supported)


class TestTrackerConfigSerialization(TestCase):
    """Tracker configs must round-trip through serialization.

    They must round-trip through both ``dataclasses.asdict`` and the UniFlow
    ``DataclassCodec``, since task args/kwargs are codec-encoded on the
    driver and decoded on the worker. ``_oss_supported`` is a ``ClassVar``
    specifically so it is excluded from both.
    """

    def test_comet_config_asdict_roundtrip(self):
        """dataclasses.asdict()/cls(**dct) round-trips CometConfig."""
        import dataclasses

        cfg = CometConfig(
            api_key="k", workspace="ws", project_name="proj", experiment_name="exp"
        )
        dct = dataclasses.asdict(cfg)
        self.assertNotIn("_oss_supported", dct)
        self.assertEqual(CometConfig(**dct), cfg)

    def test_custom_tracker_config_asdict_roundtrip(self):
        """dataclasses.asdict()/cls(**dct) round-trips CustomTrackerConfig."""
        import dataclasses

        cfg = CustomTrackerConfig(factory_fn="myproject.loggers.make_logger")
        dct = dataclasses.asdict(cfg)
        self.assertNotIn("_oss_supported", dct)
        self.assertEqual(CustomTrackerConfig(**dct), cfg)

    def test_comet_config_codec_roundtrip(self):
        """CometConfig round-trips through the UniFlow DataclassCodec."""
        from michelangelo.uniflow.core.codec import DataclassCodec

        codec = DataclassCodec()
        cfg = CometConfig(
            api_key="k", workspace="ws", project_name="proj", experiment_name="exp"
        )
        decoded = codec.decode(codec.encode(cfg))
        self.assertEqual(decoded, cfg)

    def test_nested_experiment_tracker_config_codec_roundtrip(self):
        """Nested tracker configs survive an outer codec round-trip.

        ``ExperimentTrackerConfig(tracker=CometConfig(...))`` round-trips
        through the codec at both levels (the outer dict encodes the nested
        dataclass as a plain dict via ``dataclasses.asdict`` semantics).
        """
        from michelangelo.uniflow.core.codec import DataclassCodec

        codec = DataclassCodec()
        comet = CometConfig(
            api_key="k", workspace="ws", project_name="proj", experiment_name="exp"
        )
        cfg = ExperimentTrackerConfig(tracker=comet)
        decoded = codec.decode(codec.encode(cfg))
        self.assertEqual(decoded.tracker, comet)

    def test_mlflow_config_asdict_roundtrip(self):
        """dataclasses.asdict()/cls(**dct) round-trips MlflowConfig."""
        import dataclasses

        cfg = MlflowConfig(
            experiment_name="exp", tracking_uri="http://mlflow.example.com"
        )
        dct = dataclasses.asdict(cfg)
        self.assertNotIn("_oss_supported", dct)
        self.assertEqual(MlflowConfig(**dct), cfg)

    def test_mlflow_config_codec_roundtrip(self):
        """MlflowConfig round-trips through the UniFlow DataclassCodec."""
        from michelangelo.uniflow.core.codec import DataclassCodec

        codec = DataclassCodec()
        cfg = MlflowConfig(
            experiment_name="exp", tracking_uri="http://mlflow.example.com"
        )
        decoded = codec.decode(codec.encode(cfg))
        self.assertEqual(decoded, cfg)


# ---------------------------------------------------------------------------
# CometConfig
# ---------------------------------------------------------------------------


class TestCometConfig(TestCase):
    """Tests for CometConfig dataclass."""

    def test_all_fields_stored(self):
        """It stores all required fields and defaults tags to an empty list."""
        cfg = CometConfig(
            api_key="k", workspace="ws", project_name="proj", experiment_name="exp"
        )
        self.assertEqual(cfg.api_key, "k")
        self.assertEqual(cfg.workspace, "ws")
        self.assertEqual(cfg.project_name, "proj")
        self.assertEqual(cfg.experiment_name, "exp")
        self.assertEqual(cfg.tags, [])

    def test_is_tracker_config(self):
        """CometConfig extends TrackerConfig and is OSS-supported."""
        cfg = CometConfig(
            api_key="k", workspace="ws", project_name="proj", experiment_name="exp"
        )
        self.assertIsInstance(cfg, TrackerConfig)
        self.assertTrue(cfg._oss_supported)

    def test_tags_instances_are_independent(self):
        """Default tags lists are not shared between instances."""
        a = CometConfig(
            api_key="k", workspace="w", project_name="p", experiment_name="e"
        )
        b = CometConfig(
            api_key="k", workspace="w", project_name="p", experiment_name="e"
        )
        a.tags.append("x")
        self.assertEqual(b.tags, [])


# ---------------------------------------------------------------------------
# CustomTrackerConfig
# ---------------------------------------------------------------------------


class TestCustomTrackerConfig(TestCase):
    """Tests for CustomTrackerConfig (bring-your-own tracker)."""

    def test_required_field(self):
        """factory_fn is required; factory_kwargs defaults to an empty dict."""
        cfg = CustomTrackerConfig(factory_fn="myproject.loggers.make_wandb_logger")
        self.assertEqual(cfg.factory_fn, "myproject.loggers.make_wandb_logger")
        self.assertEqual(cfg.factory_kwargs, {})

    def test_factory_kwargs_stored(self):
        """factory_kwargs round-trips."""
        cfg = CustomTrackerConfig(
            factory_fn="myproject.loggers.make_wandb_logger",
            factory_kwargs={"project": "ctr-model"},
        )
        self.assertEqual(cfg.factory_kwargs, {"project": "ctr-model"})

    def test_is_tracker_config(self):
        """CustomTrackerConfig extends TrackerConfig and is OSS-supported."""
        cfg = CustomTrackerConfig(factory_fn="myproject.loggers.make_wandb_logger")
        self.assertIsInstance(cfg, TrackerConfig)
        self.assertTrue(cfg._oss_supported)

    def test_factory_kwargs_instances_are_independent(self):
        """Default factory_kwargs dicts are not shared between instances."""
        a = CustomTrackerConfig(factory_fn="f")
        b = CustomTrackerConfig(factory_fn="f")
        a.factory_kwargs["k"] = "v"
        self.assertEqual(b.factory_kwargs, {})


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
# MlflowConfig
# ---------------------------------------------------------------------------


class TestMlflowConfig(TestCase):
    """Tests for MlflowConfig dataclass."""

    def test_construction_succeeds(self):
        """Constructing MlflowConfig with just experiment_name works."""
        cfg = MlflowConfig(experiment_name="tabular-ctr")
        self.assertEqual(cfg.experiment_name, "tabular-ctr")
        self.assertIsNone(cfg.tracking_uri)
        self.assertIsNone(cfg.run_name)
        self.assertEqual(cfg.tags, {})

    def test_all_fields_stored(self):
        """It stores all fields when explicitly provided."""
        cfg = MlflowConfig(
            experiment_name="exp",
            tracking_uri="http://mlflow.example.com",
            run_name="run-1",
            tags={"team": "ctr"},
        )
        self.assertEqual(cfg.experiment_name, "exp")
        self.assertEqual(cfg.tracking_uri, "http://mlflow.example.com")
        self.assertEqual(cfg.run_name, "run-1")
        self.assertEqual(cfg.tags, {"team": "ctr"})

    def test_tracking_uri_is_optional(self):
        """tracking_uri defaults to None."""
        cfg = MlflowConfig(experiment_name="exp")
        self.assertIsNone(cfg.tracking_uri)

    def test_is_oss_supported(self):
        """MlflowConfig no longer gates on OSS support (issue #1427 closed)."""
        self.assertTrue(MlflowConfig(experiment_name="exp")._oss_supported)


# ---------------------------------------------------------------------------
# ExperimentTrackerConfig
# ---------------------------------------------------------------------------


class TestExperimentTrackerConfig(TestCase):
    """Tests for ExperimentTrackerConfig validation and legacy-field promotion."""

    def _comet(self) -> CometConfig:
        return CometConfig(
            api_key="k", workspace="ws", project_name="p", experiment_name="e"
        )

    def test_no_tracker_ok(self):
        """Setting nothing is valid (no tracking)."""
        cfg = ExperimentTrackerConfig()
        self.assertIsNone(cfg.tracker)
        self.assertIsNone(cfg.comet)
        self.assertIsNone(cfg.mlflow)

    def test_tracker_field_ok(self):
        """Setting tracker= directly (preferred style) is valid."""
        comet = self._comet()
        cfg = ExperimentTrackerConfig(tracker=comet)
        self.assertIs(cfg.tracker, comet)

    def test_custom_tracker_via_tracker_field(self):
        """CustomTrackerConfig is accepted via the tracker field."""
        custom = CustomTrackerConfig(factory_fn="myproject.loggers.make_wandb_logger")
        cfg = ExperimentTrackerConfig(tracker=custom)
        self.assertIs(cfg.tracker, custom)

    def test_legacy_comet_promoted_to_tracker(self):
        """Legacy comet= is promoted into the tracker field."""
        comet = self._comet()
        cfg = ExperimentTrackerConfig(comet=comet)
        self.assertIs(cfg.comet, comet)
        self.assertIs(cfg.tracker, comet)

    def test_legacy_mlflow_promoted_to_tracker(self):
        """Legacy mlflow= is promoted into the tracker field."""
        mlflow = MlflowConfig(experiment_name="exp")
        cfg = ExperimentTrackerConfig(mlflow=mlflow)
        self.assertIs(cfg.mlflow, mlflow)
        self.assertIs(cfg.tracker, mlflow)

    def test_tracker_and_legacy_mixed_raises(self):
        """Setting both tracker and a legacy field raises ConfigurationError."""
        with self.assertRaises(ConfigurationError):
            ExperimentTrackerConfig(tracker=self._comet(), comet=self._comet())

    def test_multiple_legacy_fields_raises(self):
        """Setting both legacy fields raises ConfigurationError."""
        with self.assertRaises(ConfigurationError):
            ExperimentTrackerConfig(
                comet=self._comet(), mlflow=MlflowConfig(experiment_name="e")
            )


# ---------------------------------------------------------------------------
# LightningTrainerConfig
# ---------------------------------------------------------------------------


def _minimal_lightning_config(**overrides) -> LightningTrainerConfig:
    """Build a minimal valid LightningTrainerConfig."""
    defaults = {
        "model_class": "myproject.models.Net",
        "input_columns": {"x": ColumnConfig("torch.float32", [1])},
        "output_columns": {"y": ColumnConfig("torch.float32", [1])},
        "labels": {"label": ColumnConfig("torch.long", [1])},
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
        self.assertEqual(cfg.input_columns, {"x": ColumnConfig("torch.float32", [1])})
        self.assertEqual(cfg.metadata_columns, [])
        self.assertIsInstance(cfg.checkpoint_config, CheckpointConfig)
        self.assertIsNone(cfg.model_kwargs)
        self.assertIsNone(cfg.dataloading_config)
        self.assertIsNone(cfg.scaling_config)
        self.assertIsNone(cfg.lightning_trainer_kwargs)
        self.assertIsNone(cfg.hyperparameters)
        self.assertIsNone(cfg.experiment_tracker)
        self.assertIsNone(cfg.transfer_learning_spec)
        self.assertIsNone(cfg.incremental_training_mode)

    def test_optional_fields_stored(self):
        """It stores all optional fields when provided."""
        tracker = ExperimentTrackerConfig(
            tracker=CometConfig(
                api_key="k", workspace="ws", project_name="p", experiment_name="e"
            )
        )
        scaling = ScalingConfig(cpu_per_worker=4)
        cfg = _minimal_lightning_config(
            experiment_tracker=tracker,
            scaling_config=scaling,
            hyperparameters={"lr": 0.001},
            incremental_training_mode=IncrementalTrainingModeConfig.BASELINE,
        )
        self.assertIs(cfg.experiment_tracker, tracker)
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


# ---------------------------------------------------------------------------
# Column specs: list form, dict form, ordering, validation
# ---------------------------------------------------------------------------


def _cols(*names: str) -> list[ColumnConfig]:
    """Build a list-form column spec with the given names."""
    return [ColumnConfig("torch.float32", [1], name=n) for n in names]


def _lightning(**overrides) -> LightningTrainerConfig:
    """Build a minimal LightningTrainerConfig with optional overrides."""
    base = {
        "model_class": "m",
        "input_columns": _cols("a"),
        "output_columns": _cols("y"),
        "labels": _cols("l"),
        "metadata_columns": [],
    }
    return LightningTrainerConfig(**{**base, **overrides})


class TestColumnConfigName(TestCase):
    """Tests for ``ColumnConfig.name``."""

    def test_name_defaults_to_none(self):
        """``name`` is optional and defaults to ``None``."""
        self.assertIsNone(ColumnConfig("torch.float32").name)

    def test_positional_construction_unchanged(self):
        """``name`` is last, so ``ColumnConfig(dtype, shape)`` still works."""
        cfg = ColumnConfig("torch.float32", [4])
        self.assertEqual(
            (cfg.data_type, cfg.shape, cfg.name), ("torch.float32", [4], None)
        )


class TestNormalizeColumns(TestCase):
    """Tests for ``normalize_columns`` and ``column_names``."""

    def test_list_form_preserves_order(self):
        """List form keeps its order."""
        names = ["zeta", "alpha", "mid", "beta"]
        self.assertEqual(column_names(_cols(*names)), names)

    def test_dict_form_uses_keys_in_insertion_order(self):
        """Dict form copies each key into ``name`` in insertion order."""
        spec = {n: ColumnConfig("torch.long", [2]) for n in ["z", "a", "m"]}
        out = normalize_columns(spec)
        self.assertEqual([c.name for c in out], ["z", "a", "m"])
        self.assertEqual(out[0], ColumnConfig("torch.long", [2], "z"))

    def test_dict_form_does_not_mutate_input(self):
        """Normalizing a dict does not set ``name`` on the caller's entries."""
        entry = ColumnConfig("torch.long")
        normalize_columns({"a": entry})
        self.assertIsNone(entry.name)

    def test_dict_name_matching_key_allowed(self):
        """A dict entry whose ``name`` equals its key is accepted."""
        out = normalize_columns({"a": ColumnConfig("torch.long", name="a")})
        self.assertEqual(out[0].name, "a")

    def test_dict_name_conflicting_with_key_rejected(self):
        """A dict entry whose ``name`` differs from its key is ambiguous."""
        with self.assertRaisesRegex(ConfigurationError, "conflicting name"):
            normalize_columns({"a": ColumnConfig("torch.long", name="b")}, "labels")

    def test_mapping_entries_are_coerced(self):
        """Plain-dict entries (e.g. from YAML or Struct) are accepted."""
        out = normalize_columns(
            [{"data_type": "torch.long", "shape": [1], "name": "a"}]
        )
        self.assertEqual(out, [ColumnConfig("torch.long", [1], "a")])

    def test_mapping_entry_with_unknown_key_rejected(self):
        """Mapping entries with unknown keys raise a clear error."""
        with self.assertRaisesRegex(ConfigurationError, r"input_columns\[0\]"):
            normalize_columns([{"data_type": "x", "bogus": 1}], "input_columns")

    def test_non_column_entry_rejected(self):
        """Entries that are neither ColumnConfig nor mapping are rejected."""
        with self.assertRaisesRegex(ConfigurationError, "must be a ColumnConfig"):
            normalize_columns(["a"], "labels")

    def test_non_dict_non_list_rejected(self):
        """A spec that is neither dict nor list is rejected."""
        with self.assertRaisesRegex(ConfigurationError, "dict or a list"):
            normalize_columns("a", "labels")  # type: ignore[arg-type]

    def test_tuple_accepted(self):
        """Tuples are treated like lists."""
        self.assertEqual(column_names(tuple(_cols("a", "b"))), ["a", "b"])

    def test_missing_name_in_list_rejected(self):
        """List entries must have a name."""
        with self.assertRaisesRegex(ConfigurationError, r"input_columns\[0\].*name"):
            normalize_columns([ColumnConfig("torch.long")], "input_columns")

    def test_empty_name_rejected(self):
        """Empty or whitespace-only names are rejected."""
        for bad in ("", "  "):
            with (
                self.subTest(name=bad),
                self.assertRaisesRegex(ConfigurationError, "non-empty"),
            ):
                normalize_columns(_cols(bad))

    def test_empty_dict_key_rejected(self):
        """Empty dict keys are rejected."""
        with self.assertRaisesRegex(ConfigurationError, "non-empty"):
            normalize_columns({"": ColumnConfig("torch.long")})

    def test_surrounding_whitespace_rejected(self):
        """Names that differ only by surrounding whitespace are not accepted."""
        with self.assertRaisesRegex(ConfigurationError, "leading or trailing"):
            normalize_columns(_cols("a", " a"))
        with self.assertRaisesRegex(ConfigurationError, "leading or trailing"):
            normalize_columns({"a ": ColumnConfig("torch.long")})

    def test_duplicate_name_rejected(self):
        """Duplicate names in list form are rejected."""
        with self.assertRaisesRegex(ConfigurationError, "duplicate column name 'a'"):
            normalize_columns(_cols("a", "b", "a"), "output_columns")

    def test_empty_spec_is_valid(self):
        """An empty spec normalizes to an empty list."""
        self.assertEqual(normalize_columns([]), [])
        self.assertEqual(normalize_columns({}), [])


class TestLightningTrainerConfigColumns(TestCase):
    """Tests for column validation in ``LightningTrainerConfig``."""

    def test_list_form_accepted_for_each_field(self):
        """Each of the three fields accepts the list form."""
        cfg = _lightning(
            input_columns=_cols("b", "a"),
            output_columns=_cols("q", "p"),
            labels=_cols("y", "x"),
        )
        self.assertEqual(column_names(cfg.input_columns), ["b", "a"])
        self.assertEqual(column_names(cfg.output_columns), ["q", "p"])
        self.assertEqual(column_names(cfg.labels), ["y", "x"])

    def test_dict_form_still_accepted(self):
        """Dict form remains valid for backward compatibility."""
        cfg = _lightning(input_columns={"a": ColumnConfig("torch.float32", [1])})
        self.assertEqual(column_names(cfg.input_columns), ["a"])

    def test_invalid_columns_rejected_per_field(self):
        """Duplicate names are rejected for every column field."""
        for field_name in ("input_columns", "output_columns", "labels"):
            with (
                self.subTest(field=field_name),
                self.assertRaisesRegex(
                    ConfigurationError, f"{field_name} contains duplicate"
                ),
            ):
                _lightning(**{field_name: _cols("a", "a")})

    def test_empty_name_rejected_at_construction(self):
        """Empty names fail at construction, not mid-training."""
        with self.assertRaisesRegex(ConfigurationError, "non-empty"):
            _lightning(labels=_cols(""))


class TestColumnOrderSerialization(TestCase):
    """Order through protobuf ``Struct`` and the UniFlow codec."""

    NAMES: ClassVar[list[str]] = ["zeta", "alpha", "mid", "beta"]

    @staticmethod
    def _struct_roundtrip(value):
        from google.protobuf import json_format, struct_pb2

        struct = struct_pb2.Struct()
        json_format.ParseDict({"v": value}, struct)
        return json_format.MessageToDict(struct)["v"]

    def test_dict_form_loses_order_through_struct(self):
        """Regression: documents why a dict cannot carry column order."""
        spec = {n: {"data_type": "torch.float32"} for n in self.NAMES}
        recovered = list(self._struct_roundtrip(spec))
        self.assertNotEqual(recovered, self.NAMES)

    def test_list_form_preserves_order_through_struct(self):
        """List form keeps order through a ``Struct`` round-trip."""
        import dataclasses

        spec = [dataclasses.asdict(c) for c in _cols(*self.NAMES)]
        recovered = self._struct_roundtrip(spec)
        self.assertEqual(column_names(recovered), self.NAMES)

    def test_codec_roundtrip_preserves_order(self):
        """``LightningTrainerConfig`` list columns round-trip via the codec."""
        from michelangelo.uniflow.core.codec import DataclassCodec

        codec = DataclassCodec()
        cfg = _lightning(
            input_columns=_cols(*self.NAMES),
            output_columns=_cols("q", "p"),
            labels=_cols("y", "x"),
        )
        decoded = codec.decode(codec.encode(cfg))
        for field_name in ("input_columns", "output_columns", "labels"):
            self.assertEqual(
                normalize_columns(getattr(decoded, field_name)),
                normalize_columns(getattr(cfg, field_name)),
            )
        self.assertEqual(column_names(decoded.input_columns), self.NAMES)

    def test_codec_roundtrip_dict_form(self):
        """Dict form still round-trips via the codec."""
        from michelangelo.uniflow.core.codec import DataclassCodec

        codec = DataclassCodec()
        cfg = _lightning(input_columns={"a": ColumnConfig("torch.long", [3])})
        decoded = codec.decode(codec.encode(cfg))
        self.assertEqual(
            normalize_columns(decoded.input_columns),
            [ColumnConfig("torch.long", [3], "a")],
        )

    def test_real_codec_and_struct_path_preserves_list_order(self):
        """Encoder -> protobuf Struct -> decoder keeps list-form column order."""
        import json

        from google.protobuf import json_format, struct_pb2

        from michelangelo.uniflow.core.codec import decoder, encoder

        def via_struct(cfg):
            struct = struct_pb2.Struct()
            json_format.ParseDict(json.loads(encoder.encode(cfg)), struct)
            return decoder.decode(json.dumps(json_format.MessageToDict(struct)))

        listed = via_struct(_lightning(input_columns=_cols(*self.NAMES)))
        self.assertEqual(column_names(listed.input_columns), self.NAMES)

        as_dict = via_struct(
            _lightning(
                input_columns={
                    n: ColumnConfig("torch.float32", [1]) for n in self.NAMES
                }
            )
        )
        self.assertEqual(set(column_names(as_dict.input_columns)), set(self.NAMES))

    def test_encoded_payload_always_carries_name(self):
        """Documents version skew: ``name`` is encoded even when ``None``."""
        import json

        from michelangelo.uniflow.core.codec import encoder

        payload = json.loads(encoder.encode(ColumnConfig("torch.long")))
        self.assertIn("name", payload)
        self.assertIsNone(payload["name"])

    def test_old_shape_payload_decodes(self):
        """A payload from an SDK without ``name`` still decodes."""
        from michelangelo.uniflow.core.codec import decoder

        payload = (
            '{"data_type":"torch.long","shape":[1],'
            '"__class__":"michelangelo.workflow.schema.tabular_trainer.ColumnConfig",'
            '"__codec__":"dataclass"}'
        )
        self.assertEqual(decoder.decode(payload), ColumnConfig("torch.long", [1]))
