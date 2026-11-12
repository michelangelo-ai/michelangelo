"""Tests for the offline inference step of the tabular inference task."""

from __future__ import annotations

from unittest import TestCase
from unittest.mock import MagicMock, patch

from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend
from michelangelo.lib.inference.ray.inference import OutputColumnMetadata
from michelangelo.lib.model_manager.constants import RawModelType
from michelangelo.lib.model_manager.registry.client import InMemoryRegistryClient
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.inference import (
    BatchOptions,
    ModelIdentifier,
    OfflineInferenceConfig,
    PaddingOptions,
)
from michelangelo.workflow.tasks.tabular_inference.offline.inference import (
    offline_inference,
)
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_CUSTOM,
    TRAINING_FRAMEWORK_LIGHTNING,
    TRAINING_FRAMEWORK_PYTORCH,
    ModelMetadata,
)
from michelangelo.workflow.variables.types import AssembledModel, ModelArtifact

_PATCH_TARGET = (
    "michelangelo.workflow.tasks.tabular_inference.offline.inference."
    "batch_model_inference"
)


def _assembled(framework, raw_path="raw/path", deployable_path="deployable/path"):
    return AssembledModel(
        raw_model=ModelArtifact(
            path=raw_path, metadata=ModelMetadata(training_framework=framework)
        ),
        deployable_model=ModelArtifact(path=deployable_path),
    )


class OfflineInferenceTest(TestCase):
    """Tests for ``offline_inference``."""

    @patch(_PATCH_TARGET)
    def test_no_config_returns_datasets_unchanged(self, mock_batch):
        """An empty config skips inference."""
        datasets = {"train": MagicMock()}

        results, metadata = offline_inference([], datasets)

        self.assertIs(results, datasets)
        self.assertEqual(metadata, [])
        mock_batch.assert_not_called()

    @patch(_PATCH_TARGET)
    def test_batch_and_padding_options_are_forwarded(self, mock_batch):
        """Every batch option and the padding values reach the inference config."""
        mock_batch.return_value = ({}, [])
        backend = LocalStorageBackend("/tmp")
        config = [
            OfflineInferenceConfig(
                input_columns=["f1"],
                batch_options=BatchOptions(
                    batch_size=8,
                    num_gpus=0.5,
                    concurrency=2,
                    num_cpus=3,
                    max_concurrency=4,
                    max_tasks_in_flight_per_actor=5,
                    max_restarts=6,
                    max_task_retries=7,
                ),
                padding_options=PaddingOptions(column_padding_values={"f1": -1}),
                columns_to_keep=["label"],
            )
        ]

        offline_inference(config, {}, _assembled("custom"), storage_backend=backend)

        inference_configs, _, storage_backend = mock_batch.call_args.args
        self.assertIs(storage_backend, backend)
        (cfg,) = inference_configs
        self.assertEqual(cfg.input_columns, ["f1"])
        self.assertEqual(cfg.columns_to_keep, ["label"])
        self.assertEqual(
            vars(cfg.batch_options),
            {
                "batch_size": 8,
                "num_gpus": 0.5,
                "concurrency": 2,
                "num_cpus": 3,
                "max_concurrency": 4,
                "max_tasks_in_flight_per_actor": 5,
                "max_restarts": 6,
                "max_task_retries": 7,
            },
        )
        self.assertEqual(cfg.padding_options.column_padding_values, {"f1": -1})

    @patch(_PATCH_TARGET)
    def test_unset_options_stay_none(self, mock_batch):
        """Unset batch and padding options are passed on as None."""
        mock_batch.return_value = ({}, [])

        offline_inference([OfflineInferenceConfig()], {}, _assembled("custom"))

        (cfg,) = mock_batch.call_args.args[0]
        self.assertIsNone(cfg.batch_options)
        self.assertIsNone(cfg.padding_options)
        self.assertIsNone(cfg.model.name)
        self.assertIsNone(cfg.model.revision_id)

    @patch(_PATCH_TARGET)
    def test_returns_results_and_output_column_metadata(self, mock_batch):
        """The datasets and the output column metadata come back."""
        scored = {"train": MagicMock()}
        mock_batch.return_value = (
            scored,
            [OutputColumnMetadata(column_name="prediction", model_name="m")],
        )

        results, metadata = offline_inference(
            [OfflineInferenceConfig()], {}, _assembled("custom")
        )

        self.assertIs(results, scored)
        self.assertEqual(
            metadata, [OutputColumnMetadata(column_name="prediction", model_name="m")]
        )

    @patch(_PATCH_TARGET)
    def test_pytorch_assembled_model_uses_deployable_package(self, mock_batch):
        """A PyTorch model is scored from the deployable package."""
        mock_batch.return_value = ({}, [])

        offline_inference(
            [OfflineInferenceConfig()], {}, _assembled(TRAINING_FRAMEWORK_PYTORCH)
        )

        (cfg,) = mock_batch.call_args.args[0]
        self.assertEqual(cfg.model.model_path, "deployable/path")
        self.assertEqual(cfg.model.model_type, RawModelType.TORCH)

    @patch(_PATCH_TARGET)
    def test_lightning_assembled_model_uses_deployable_package(self, mock_batch):
        """A Lightning model is scored from the deployable package."""
        mock_batch.return_value = ({}, [])

        offline_inference(
            [OfflineInferenceConfig()], {}, _assembled(TRAINING_FRAMEWORK_LIGHTNING)
        )

        (cfg,) = mock_batch.call_args.args[0]
        self.assertEqual(cfg.model.model_path, "deployable/path")
        self.assertEqual(cfg.model.model_type, RawModelType.LIGHTNING)

    @patch(_PATCH_TARGET)
    def test_custom_assembled_model_uses_raw_package(self, mock_batch):
        """A custom model is scored from the raw package as custom Python."""
        mock_batch.return_value = ({}, [])

        offline_inference(
            [OfflineInferenceConfig()], {}, _assembled(TRAINING_FRAMEWORK_CUSTOM)
        )

        (cfg,) = mock_batch.call_args.args[0]
        self.assertEqual(cfg.model.model_path, "raw/path")
        self.assertEqual(cfg.model.model_type, RawModelType.CUSTOM_PYTHON)

    @patch(_PATCH_TARGET)
    def test_other_frameworks_use_raw_package_and_pass_through(self, mock_batch):
        """An unknown or missing framework uses the raw package, unchanged."""
        mock_batch.return_value = ({}, [])

        for framework in ("xgboost", None):
            offline_inference([OfflineInferenceConfig()], {}, _assembled(framework))

            (cfg,) = mock_batch.call_args.args[0]
            self.assertEqual(cfg.model.model_path, "raw/path")
            self.assertEqual(cfg.model.model_type, framework)

    @patch(_PATCH_TARGET)
    def test_assembled_model_wins_over_registry_model(self, mock_batch):
        """The assembled model's package is used for every entry."""
        mock_batch.return_value = ({}, [])
        config = [
            OfflineInferenceConfig(model=ModelIdentifier(name="m", revision_id=2)),
            OfflineInferenceConfig(),
        ]

        offline_inference(config, {}, _assembled(TRAINING_FRAMEWORK_PYTORCH))

        first, second = mock_batch.call_args.args[0]
        self.assertEqual(first.model.model_path, "deployable/path")
        self.assertEqual((first.model.name, first.model.revision_id), ("m", 2))
        self.assertEqual(second.model.model_path, "deployable/path")

    @patch(_PATCH_TARGET)
    def test_registry_model_uses_deployable_artifact(self, mock_batch):
        """A registered non-custom model is scored from its deployable artifact."""
        mock_batch.return_value = ({}, [])
        registry = InMemoryRegistryClient()
        registry.register_model(
            "m",
            artifact_uri="s3://raw",
            deployable_artifact_uri="s3://deployable",
            labels={"training_framework": TRAINING_FRAMEWORK_PYTORCH},
        )
        registry.register_model(
            "m",
            artifact_uri="s3://raw2",
            deployable_artifact_uri="s3://deployable2",
            labels={"training_framework": "xgboost"},
        )
        backend = LocalStorageBackend("/tmp")

        offline_inference(
            [
                OfflineInferenceConfig(model=ModelIdentifier(name="m", revision_id=1)),
                OfflineInferenceConfig(model=ModelIdentifier(name="m")),
            ],
            {},
            storage_backend=backend,
            registry_client=registry,
        )

        first, second = mock_batch.call_args.args[0]
        self.assertEqual(first.model.model_path, "s3://deployable")
        self.assertEqual(first.model.model_type, RawModelType.TORCH)
        self.assertEqual(second.model.model_path, "s3://deployable2")
        self.assertIsNone(second.model.model_type)

    @patch(_PATCH_TARGET)
    def test_registry_model_without_label_has_no_model_type(self, mock_batch):
        """A registered model with no framework label has no model type."""
        mock_batch.return_value = ({}, [])
        registry = InMemoryRegistryClient()
        registry.register_model(
            "m", artifact_uri="s3://raw", deployable_artifact_uri="s3://deployable"
        )

        offline_inference(
            [OfflineInferenceConfig(model=ModelIdentifier(name="m"))],
            {},
            storage_backend=LocalStorageBackend("/tmp"),
            registry_client=registry,
        )

        (cfg,) = mock_batch.call_args.args[0]
        self.assertEqual(cfg.model.model_path, "s3://deployable")
        self.assertIsNone(cfg.model.model_type)

    @patch(_PATCH_TARGET)
    def test_custom_registry_model_uses_raw_artifact(self, mock_batch):
        """A registered custom model is scored from its raw artifact."""
        mock_batch.return_value = ({}, [])
        registry = InMemoryRegistryClient()
        registry.register_model(
            "c",
            artifact_uri="s3://raw",
            deployable_artifact_uri="s3://deployable",
            labels={"training_framework": TRAINING_FRAMEWORK_CUSTOM},
        )

        offline_inference(
            [OfflineInferenceConfig(model=ModelIdentifier(name="c"))],
            {},
            storage_backend=LocalStorageBackend("/tmp"),
            registry_client=registry,
        )

        (cfg,) = mock_batch.call_args.args[0]
        self.assertEqual(cfg.model.model_path, "s3://raw")
        self.assertEqual(cfg.model.model_type, RawModelType.CUSTOM_PYTHON)

    @patch(_PATCH_TARGET)
    def test_registry_model_needs_registry_client(self, mock_batch):
        """A registry model without a client names the missing argument."""
        with self.assertRaisesRegex(ConfigurationError, "registry_client"):
            offline_inference(
                [OfflineInferenceConfig(model=ModelIdentifier(name="m"))],
                {},
                storage_backend=LocalStorageBackend("/tmp"),
            )
        mock_batch.assert_not_called()

    @patch(_PATCH_TARGET)
    def test_registry_model_needs_storage_backend(self, mock_batch):
        """A registry model without a backend names the missing argument."""
        with self.assertRaisesRegex(ConfigurationError, "storage_backend"):
            offline_inference(
                [OfflineInferenceConfig(model=ModelIdentifier(name="m"))],
                {},
                registry_client=InMemoryRegistryClient(),
            )
        mock_batch.assert_not_called()

    @patch(_PATCH_TARGET)
    def test_registry_model_without_artifact_is_rejected(self, mock_batch):
        """A registered model whose needed artifact URI is empty is rejected."""
        registry = InMemoryRegistryClient()
        registry.register_model(
            "m",
            artifact_uri="s3://raw",
            labels={"training_framework": TRAINING_FRAMEWORK_PYTORCH},
        )
        with self.assertRaisesRegex(ConfigurationError, "no deployable_artifact_uri"):
            offline_inference(
                [OfflineInferenceConfig(model=ModelIdentifier(name="m"))],
                {},
                storage_backend=LocalStorageBackend("/tmp"),
                registry_client=registry,
            )
        mock_batch.assert_not_called()
