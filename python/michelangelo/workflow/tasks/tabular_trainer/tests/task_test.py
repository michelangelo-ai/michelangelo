"""Tests for michelangelo.workflow.tasks.tabular_trainer.task."""

from __future__ import annotations

import warnings
from unittest import TestCase
from unittest.mock import Mock, patch

import numpy as np

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.tabular_trainer import (
    BatchIterConfig,
    CheckpointConfig,
    CometConfig,
    CustomTrainerConfig,
    DataloadingConfig,
    ExperimentTrackerConfig,
    IncrementalTrainingModeConfig,
    MlflowConfig,
    TabularTrainerConfig,
)
from michelangelo.workflow.tasks.tabular_trainer.task import (
    _apply_incremental_training_metadata,
    train_tabular,
)
from michelangelo.workflow.tasks.tabular_trainer.tests.fixtures import (
    make_model_artifact,
    make_tabular_config,
    mock_train_dataset,
    mock_validation_dataset,
)
from michelangelo.workflow.variables.metadata import ModelMetadata

_TRAINER_TASK = "michelangelo.workflow.tasks.tabular_trainer.task"

# ---------------------------------------------------------------------------
# _apply_incremental_training_metadata
# ---------------------------------------------------------------------------


class TestApplyIncrementalTrainingMetadata(TestCase):
    """Tests for _apply_incremental_training_metadata."""

    def test_no_initial_model_no_mode_no_change(self):
        """No fields set when initial_model is None and mode is not BASELINE."""
        meta = ModelMetadata()
        _apply_incremental_training_metadata(meta, None, None)
        self.assertFalse(meta.is_incremental_training)
        self.assertIsNone(meta.baseline_model_identifier)

    def test_initial_model_incremental_propagates(self):
        """Chain propagates when initial_model.is_incremental_training is True."""
        meta = ModelMetadata()
        initial = make_model_artifact(
            is_incremental_training=True,
            baseline_model_identifier="baseline-v1",
        )
        _apply_incremental_training_metadata(meta, initial, None)
        self.assertTrue(meta.is_incremental_training)
        self.assertEqual(meta.baseline_model_identifier, "baseline-v1")

    def test_initial_model_not_incremental_no_propagation(self):
        """Transfer-learning initial_model (not incremental) starts fresh."""
        meta = ModelMetadata()
        initial = make_model_artifact(is_incremental_training=False)
        _apply_incremental_training_metadata(meta, initial, None)
        self.assertFalse(meta.is_incremental_training)
        self.assertIsNone(meta.baseline_model_identifier)

    def test_baseline_mode_sets_incremental(self):
        """BASELINE mode marks model as incremental with no identifier."""
        meta = ModelMetadata()
        _apply_incremental_training_metadata(
            meta, None, IncrementalTrainingModeConfig.BASELINE
        )
        self.assertTrue(meta.is_incremental_training)
        self.assertIsNone(meta.baseline_model_identifier)

    def test_none_mode_no_change(self):
        """NONE mode leaves metadata unchanged."""
        meta = ModelMetadata()
        _apply_incremental_training_metadata(
            meta, None, IncrementalTrainingModeConfig.NONE
        )
        self.assertFalse(meta.is_incremental_training)

    def test_incremental_initial_model_takes_priority_over_baseline_mode(self):
        """Continuation wins over BASELINE when initial_model is incremental.

        When both initial_model.is_incremental_training is True and
        incremental_training_mode == BASELINE, the continuation branch fires
        first — the existing chain continues rather than restarting.
        """
        meta = ModelMetadata()
        initial = make_model_artifact(
            is_incremental_training=True,
            baseline_model_identifier="baseline-v1",
        )
        _apply_incremental_training_metadata(
            meta, initial, IncrementalTrainingModeConfig.BASELINE
        )
        self.assertTrue(meta.is_incremental_training)
        self.assertEqual(meta.baseline_model_identifier, "baseline-v1")


# ---------------------------------------------------------------------------
# ModelMetadata.to_registry_dict — new fields
# ---------------------------------------------------------------------------


class TestModelMetadataRegistryDict(TestCase):
    """Tests for the two new fields in ModelMetadata.to_registry_dict."""

    def test_is_incremental_training_included(self):
        """is_incremental_training always appears in the dict."""
        meta = ModelMetadata()
        d = meta.to_registry_dict()
        self.assertIn("is_incremental_training", d)
        self.assertEqual(d["is_incremental_training"], "false")

    def test_is_incremental_training_true(self):
        """is_incremental_training=True serialises to 'true'."""
        meta = ModelMetadata(is_incremental_training=True)
        self.assertEqual(meta.to_registry_dict()["is_incremental_training"], "true")

    def test_baseline_model_identifier_omitted_when_none(self):
        """baseline_model_identifier is omitted when None."""
        meta = ModelMetadata()
        self.assertNotIn("baseline_model_identifier", meta.to_registry_dict())

    def test_baseline_model_identifier_included_when_set(self):
        """baseline_model_identifier appears when set."""
        meta = ModelMetadata(baseline_model_identifier="base-v1")
        self.assertEqual(
            meta.to_registry_dict()["baseline_model_identifier"], "base-v1"
        )


# ---------------------------------------------------------------------------
# train_tabular — guard rails
# ---------------------------------------------------------------------------


class TestTrainTabularGuards(TestCase):
    """Tests for train_tabular input validation."""

    def test_custom_backend_raises_not_implemented(self):
        """config.custom raises NotImplementedError."""
        config = TabularTrainerConfig(
            custom=CustomTrainerConfig(train_class="myproject.Trainer")
        )
        with self.assertRaises(NotImplementedError):
            train_tabular(
                config,
                mock_train_dataset(),
                mock_validation_dataset(),
            )

    def test_save_every_n_steps_raises(self):
        """save_every_n_steps set raises NotImplementedError."""
        config = make_tabular_config(
            checkpoint_config=CheckpointConfig(save_every_n_steps=100)
        )
        with (
            self.assertRaises(NotImplementedError),
            patch(
                f"{_TRAINER_TASK}.get_module_attr",
                return_value=lambda **kw: Mock(),
            ),
        ):
            train_tabular(
                config,
                mock_train_dataset(),
                mock_validation_dataset(),
            )

    def test_transfer_learning_spec_raises(self):
        """transfer_learning_spec set raises NotImplementedError."""
        from michelangelo.workflow.schema.tabular_trainer import (
            TransferLearningSpecConfig,
        )

        config = make_tabular_config(
            transfer_learning_spec=TransferLearningSpecConfig(
                transfer_learning_spec={"layers": 3}
            )
        )
        with (
            self.assertRaises(NotImplementedError),
            patch(
                f"{_TRAINER_TASK}.get_module_attr",
                return_value=lambda **kw: Mock(),
            ),
        ):
            train_tabular(
                config,
                mock_train_dataset(),
                mock_validation_dataset(),
            )

    def test_mlflow_config_raises_not_implemented(self):
        """ExperimentTrackerConfig(mlflow=...) raises NotImplementedError."""
        config = make_tabular_config(
            experiment_tracker=ExperimentTrackerConfig(
                mlflow=MlflowConfig(
                    tracking_uri="http://localhost:5000",
                    experiment_name="test-exp",
                )
            )
        )
        with (
            self.assertRaises(NotImplementedError),
            patch(
                f"{_TRAINER_TASK}.get_module_attr",
                return_value=lambda **kw: Mock(),
            ),
        ):
            train_tabular(
                config,
                mock_train_dataset(),
                mock_validation_dataset(),
            )

    def test_empty_train_dataset_raises(self):
        """Zero-row train dataset raises ConfigurationError."""
        train_ds = mock_train_dataset()
        train_ds.value.take.return_value = []

        with (
            self.assertRaises(ConfigurationError),
            patch(
                f"{_TRAINER_TASK}.get_module_attr",
                return_value=lambda **kw: Mock(),
            ),
            patch(f"{_TRAINER_TASK}.LightningTrainerParam"),
            patch(f"{_TRAINER_TASK}.LightningTrainerWithStateDict") as mt,
        ):
            mt.return_value.train.return_value = None
            mt.return_value.update_model_state_dict.return_value = None
            train_tabular(
                make_tabular_config(),
                train_ds,
                mock_validation_dataset(),
            )


# ---------------------------------------------------------------------------
# train_tabular — success path helpers
# ---------------------------------------------------------------------------


def _run_train(config=None, train_ds=None, val_ds=None, **kwargs):
    """Run train_tabular with all Ray/Lightning deps mocked."""
    config = config or make_tabular_config()
    train_ds = train_ds or mock_train_dataset()
    val_ds = val_ds or mock_validation_dataset()
    with (
        patch(
            f"{_TRAINER_TASK}.get_module_attr",
            return_value=lambda **kw: Mock(),
        ),
        patch(f"{_TRAINER_TASK}.os.path.isfile", return_value=True),
        patch(f"{_TRAINER_TASK}.ModelVariable") as mv_cls,
        patch(f"{_TRAINER_TASK}.LightningTrainerParam"),
        patch(f"{_TRAINER_TASK}.LightningTrainerWithStateDict") as mt,
    ):
        mt.return_value.train.return_value = None
        mt.return_value.update_model_state_dict.return_value = None
        result = train_tabular(config, train_ds, val_ds, **kwargs)
    return result, mv_cls, mt


# ---------------------------------------------------------------------------
# train_tabular — success path
# ---------------------------------------------------------------------------


class TestTrainTabularLightning(TestCase):
    """Tests for the lightning success path of train_tabular."""

    def test_returns_model_variable(self):
        """Returns the constructed ModelVariable instance."""
        result, mv_cls, _ = _run_train()
        self.assertIs(result, mv_cls.return_value)

    def test_model_variable_save_called(self):
        """ModelVariable.save() is called once to persist the trained model."""
        _, mv_cls, _ = _run_train()
        mv_cls.return_value.save.assert_called_once()

    def test_model_variable_assembled_false(self):
        """Constructed ModelVariable's metadata has assembled=False."""
        _, mv_cls, _ = _run_train()
        metadata = mv_cls.call_args.kwargs["metadata"]
        self.assertFalse(metadata.assembled)

    def test_model_variable_deployable_false(self):
        """Constructed ModelVariable's metadata has deployable=False."""
        _, mv_cls, _ = _run_train()
        metadata = mv_cls.call_args.kwargs["metadata"]
        self.assertFalse(metadata.deployable)

    def test_model_variable_training_framework_lightning(self):
        """Constructed ModelVariable's metadata carries training_framework."""
        _, mv_cls, _ = _run_train()
        metadata = mv_cls.call_args.kwargs["metadata"]
        self.assertEqual(metadata.training_framework, "lightning")

    def test_datasets_loaded(self):
        """load_ray_dataset() is called on both datasets."""
        train_ds = mock_train_dataset()
        val_ds = mock_validation_dataset()
        _run_train(train_ds=train_ds, val_ds=val_ds)
        train_ds.load_ray_dataset.assert_called_once()
        val_ds.load_ray_dataset.assert_called_once()

    def test_default_precision_remote(self):
        """Default precision is bf16-mixed for is_local_run=False."""
        with (
            patch(
                f"{_TRAINER_TASK}.get_module_attr",
                return_value=lambda **kw: Mock(),
            ),
            patch(f"{_TRAINER_TASK}.ModelVariable"),
            patch(f"{_TRAINER_TASK}.LightningTrainerParam") as mp,
            patch(f"{_TRAINER_TASK}.LightningTrainerWithStateDict") as mt,
        ):
            mt.return_value.train.return_value = None
            mt.return_value.update_model_state_dict.return_value = None
            train_tabular(
                make_tabular_config(),
                mock_train_dataset(),
                mock_validation_dataset(),
                is_local_run=False,
            )
        kwargs_dict = mp.call_args.kwargs.get("lightning_trainer_kwargs", {})
        self.assertEqual(kwargs_dict.get("precision"), "bf16-mixed")

    def test_default_precision_local(self):
        """Default precision is '32' for is_local_run=True."""
        with (
            patch(
                f"{_TRAINER_TASK}.get_module_attr",
                return_value=lambda **kw: Mock(),
            ),
            patch(f"{_TRAINER_TASK}.ModelVariable"),
            patch(f"{_TRAINER_TASK}.LightningTrainerParam") as mp,
            patch(f"{_TRAINER_TASK}.LightningTrainerWithStateDict") as mt,
        ):
            mt.return_value.train.return_value = None
            mt.return_value.update_model_state_dict.return_value = None
            train_tabular(
                make_tabular_config(),
                mock_train_dataset(),
                mock_validation_dataset(),
                is_local_run=True,
            )
        kwargs_dict = mp.call_args.kwargs.get("lightning_trainer_kwargs", {})
        self.assertEqual(kwargs_dict.get("precision"), "32")

    def test_batch_iter_config_overrides_hyperparameters(self):
        """BatchIterConfig.batch_size overrides hyperparameters.batch_size."""
        config = make_tabular_config(
            hyperparameters={"batch_size": 4},
            dataloading_config=DataloadingConfig(
                batch_iter_config=BatchIterConfig(batch_size=16)
            ),
        )
        with (
            patch(
                f"{_TRAINER_TASK}.get_module_attr",
                return_value=lambda **kw: Mock(),
            ),
            patch(f"{_TRAINER_TASK}.ModelVariable"),
            patch(f"{_TRAINER_TASK}.LightningTrainerParam") as mp,
            patch(f"{_TRAINER_TASK}.LightningTrainerWithStateDict") as mt,
        ):
            mt.return_value.train.return_value = None
            mt.return_value.update_model_state_dict.return_value = None
            with warnings.catch_warnings():
                warnings.simplefilter("ignore")
                train_tabular(
                    config,
                    mock_train_dataset(),
                    mock_validation_dataset(),
                )
        self.assertEqual(mp.call_args.kwargs.get("batch_size"), 16)

    def test_comet_param_built_from_experiment_tracker(self):
        """CometParam is constructed when experiment_tracker.comet is set."""
        config = make_tabular_config(
            experiment_tracker=ExperimentTrackerConfig(
                comet=CometConfig(
                    api_key="k",
                    workspace="ws",
                    project_name="proj",
                    experiment_name="exp",
                )
            )
        )
        with (
            patch(
                f"{_TRAINER_TASK}.get_module_attr",
                return_value=lambda **kw: Mock(),
            ),
            patch(f"{_TRAINER_TASK}.CometParam") as mock_comet,
            patch(f"{_TRAINER_TASK}.ModelVariable"),
            patch(f"{_TRAINER_TASK}.LightningTrainerParam"),
            patch(f"{_TRAINER_TASK}.LightningTrainerWithStateDict") as mt,
        ):
            mt.return_value.train.return_value = None
            mt.return_value.update_model_state_dict.return_value = None
            train_tabular(
                config,
                mock_train_dataset(),
                mock_validation_dataset(),
            )
        mock_comet.assert_called_once_with(
            api_key="k", project_name="proj", experiment_name="exp", workspace="ws"
        )

    def test_no_comet_when_experiment_tracker_none(self):
        """comet_param is None when experiment_tracker is not set."""
        with (
            patch(
                f"{_TRAINER_TASK}.get_module_attr",
                return_value=lambda **kw: Mock(),
            ),
            patch(f"{_TRAINER_TASK}.ModelVariable"),
            patch(f"{_TRAINER_TASK}.LightningTrainerParam") as mp,
            patch(f"{_TRAINER_TASK}.LightningTrainerWithStateDict") as mt,
        ):
            mt.return_value.train.return_value = None
            mt.return_value.update_model_state_dict.return_value = None
            train_tabular(
                make_tabular_config(),
                mock_train_dataset(),
                mock_validation_dataset(),
            )
        self.assertIsNone(mp.call_args.kwargs.get("comet_param"))

    def test_initial_model_sets_weights_path(self):
        """initial_weights_path is read directly from initial_model.path.

        No storage backend is involved — ModelArtifact.path for a lightning
        warm-start points directly at the state-dict file, matching what
        LightningTrainerParam.initial_weights_path expects.
        """
        initial = make_model_artifact(path="/tmp/base/model.pt")
        with (
            patch(
                f"{_TRAINER_TASK}.get_module_attr",
                return_value=lambda **kw: Mock(),
            ),
            patch(f"{_TRAINER_TASK}.os.path.isfile", return_value=True),
            patch(f"{_TRAINER_TASK}.ModelVariable"),
            patch(f"{_TRAINER_TASK}.LightningTrainerParam") as mp,
            patch(f"{_TRAINER_TASK}.LightningTrainerWithStateDict") as mt,
        ):
            mt.return_value.train.return_value = None
            mt.return_value.update_model_state_dict.return_value = None
            train_tabular(
                make_tabular_config(),
                mock_train_dataset(),
                mock_validation_dataset(),
                initial_model=initial,
            )
        self.assertEqual(
            mp.call_args.kwargs["initial_weights_path"], "/tmp/base/model.pt"
        )

    def test_initial_model_missing_file_raises(self):
        """A nonexistent initial_model.path raises ConfigurationError."""
        initial = make_model_artifact(path="/tmp/definitely_does_not_exist/model.pt")
        with (
            self.assertRaises(ConfigurationError),
            patch(
                f"{_TRAINER_TASK}.get_module_attr",
                return_value=lambda **kw: Mock(),
            ),
        ):
            train_tabular(
                make_tabular_config(),
                mock_train_dataset(),
                mock_validation_dataset(),
                initial_model=initial,
            )

    def test_no_initial_model_no_weights_path(self):
        """initial_weights_path is None without initial_model."""
        with (
            patch(
                f"{_TRAINER_TASK}.get_module_attr",
                return_value=lambda **kw: Mock(),
            ),
            patch(f"{_TRAINER_TASK}.ModelVariable"),
            patch(f"{_TRAINER_TASK}.LightningTrainerParam") as mp,
            patch(f"{_TRAINER_TASK}.LightningTrainerWithStateDict") as mt,
        ):
            mt.return_value.train.return_value = None
            mt.return_value.update_model_state_dict.return_value = None
            train_tabular(
                make_tabular_config(),
                mock_train_dataset(),
                mock_validation_dataset(),
            )
        self.assertIsNone(mp.call_args.kwargs["initial_weights_path"])

    def test_incremental_metadata_propagated(self):
        """is_incremental_training propagates from initial_model."""
        initial = make_model_artifact(
            is_incremental_training=True, baseline_model_identifier="base-v1"
        )
        _, mv_cls, _ = _run_train(initial_model=initial)
        metadata = mv_cls.call_args.kwargs["metadata"]
        self.assertTrue(metadata.is_incremental_training)
        self.assertEqual(metadata.baseline_model_identifier, "base-v1")

    def test_baseline_mode_sets_incremental(self):
        """BASELINE incremental mode sets is_incremental_training=True."""
        config = make_tabular_config(
            incremental_training_mode=IncrementalTrainingModeConfig.BASELINE
        )
        _, mv_cls, _ = _run_train(config=config)
        metadata = mv_cls.call_args.kwargs["metadata"]
        self.assertTrue(metadata.is_incremental_training)

    def test_metadata_columns_excluded_from_sample(self):
        """metadata_columns are passed to collate_sample_row."""
        sample_row = {
            "x": np.array([1.0]),
            "label": np.array([0]),
            "user_id": "abc",
        }
        train_ds = mock_train_dataset(sample_row)
        config = make_tabular_config(metadata_columns=["user_id"])

        with patch(
            f"{_TRAINER_TASK}.collate_sample_row",
            wraps=lambda row, fn, cols: row,
        ) as mock_collate:
            _run_train(config=config, train_ds=train_ds)
        mock_collate.assert_called_once()
        _, _, meta_cols = mock_collate.call_args[0]
        self.assertIn("user_id", meta_cols)


# ---------------------------------------------------------------------------
# train_tabular — default RunConfig storage
# ---------------------------------------------------------------------------


class TestTrainTabularDefaultRunConfig(TestCase):
    """Tests for train_tabular's default-RunConfig construction."""

    _CREATE_RUN_CONFIG = "michelangelo.uniflow.plugins.ray.run_config.create_run_config"

    def test_none_run_config_delegates_to_create_run_config(self):
        """run_config=None builds the default via the shared UniFlow helper."""
        with patch(self._CREATE_RUN_CONFIG) as mock_create:
            _, _, mt = _run_train()
        mock_create.assert_called_once()
        self.assertIs(mt.call_args.kwargs["run_config"], mock_create.return_value)

    def test_create_run_config_receives_checkpoint_config(self):
        """The default RunConfig is built with the resolved CheckpointConfig."""
        config = make_tabular_config(checkpoint_config=CheckpointConfig(num_to_keep=3))
        with patch(self._CREATE_RUN_CONFIG) as mock_create:
            _run_train(config=config)
        checkpoint_config = mock_create.call_args.kwargs["checkpoint_config"]
        self.assertEqual(checkpoint_config.num_to_keep, 3)

    def test_explicit_run_config_not_overridden(self):
        """An explicitly-passed run_config skips create_run_config entirely."""
        import ray.train

        explicit = ray.train.RunConfig(storage_path="/explicit/path")
        with patch(self._CREATE_RUN_CONFIG) as mock_create:
            _, _, mt = _run_train(run_config=explicit)
        mock_create.assert_not_called()
        self.assertIs(mt.call_args.kwargs["run_config"], explicit)


# ---------------------------------------------------------------------------
# train_tabular — dispatch tests
# ---------------------------------------------------------------------------


class TestTrainTabularDispatch(TestCase):
    """Dispatch and contract tests for train_tabular."""

    def test_lightning_config_dispatches_to_lightning(self):
        """A config with lightning= dispatches to _train_lightning."""
        with patch(
            f"{_TRAINER_TASK}._train_lightning",
            return_value=Mock(),
        ) as mock_tl:
            train_tabular(
                make_tabular_config(),
                mock_train_dataset(),
                mock_validation_dataset(),
            )
        mock_tl.assert_called_once()

    def test_custom_config_raises_not_implemented(self):
        """A config with custom= raises NotImplementedError."""
        config = TabularTrainerConfig(
            custom=CustomTrainerConfig(train_class="myproject.Trainer")
        )
        with self.assertRaises(NotImplementedError):
            train_tabular(
                config,
                mock_train_dataset(),
                mock_validation_dataset(),
            )
