"""Tests for the offline inference step of the tabular inference task."""

from __future__ import annotations

import dataclasses
from unittest import TestCase
from unittest.mock import MagicMock, patch

import michelangelo.lib.inference.ray.inference as lib_inference
import michelangelo.workflow.schema.inference as schema_inference
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
    def test_assembled_and_registry_entries_resolve_separately(self, mock_batch):
        """An entry with a model uses the registry even next to an assembled model."""
        mock_batch.return_value = ({}, [])
        registry = InMemoryRegistryClient()
        registry.register_model(
            "m", artifact_uri="s3://raw", deployable_artifact_uri="s3://deployable"
        )
        registry.register_model(
            "m", artifact_uri="s3://raw2", deployable_artifact_uri="s3://deployable2"
        )
        config = [
            OfflineInferenceConfig(model=ModelIdentifier(name="m", revision_id=1)),
            OfflineInferenceConfig(),
        ]

        offline_inference(
            config,
            {},
            _assembled(TRAINING_FRAMEWORK_PYTORCH),
            storage_backend=LocalStorageBackend("/tmp"),
            registry_client=registry,
        )

        registry_entry, assembled_entry = mock_batch.call_args.args[0]
        self.assertEqual(registry_entry.model.model_path, "s3://deployable")
        self.assertEqual(
            (registry_entry.model.name, registry_entry.model.revision_id), ("m", 1)
        )
        self.assertEqual(assembled_entry.model.model_path, "deployable/path")
        self.assertIsNone(assembled_entry.model.name)
        self.assertIsNone(assembled_entry.model.revision_id)

    @patch(_PATCH_TARGET)
    def test_entry_without_any_model_is_rejected(self, mock_batch):
        """An entry with no model and no assembled model fails before Ray runs."""
        config = [
            OfflineInferenceConfig(model=ModelIdentifier(name="m")),
            OfflineInferenceConfig(),
        ]
        with self.assertRaisesRegex(ConfigurationError, r"local_models\[1\]"):
            offline_inference(config, {}, registry_client=InMemoryRegistryClient())
        mock_batch.assert_not_called()

    @patch(_PATCH_TARGET)
    def test_two_entries_scoring_the_assembled_model_are_rejected(self, mock_batch):
        """Two entries without a model would score the same model twice."""
        config = [OfflineInferenceConfig(), OfflineInferenceConfig()]
        with self.assertRaisesRegex(ConfigurationError, "duplicate every row"):
            offline_inference(config, {}, _assembled(TRAINING_FRAMEWORK_CUSTOM))
        mock_batch.assert_not_called()

    @patch(_PATCH_TARGET)
    def test_pytorch_assembled_model_without_deployable_is_rejected(self, mock_batch):
        """A PyTorch model is scored from its deployable package, so it must exist."""
        assembled = _assembled(TRAINING_FRAMEWORK_PYTORCH)
        assembled.deployable_model = None
        with self.assertRaisesRegex(ConfigurationError, "no deployable package"):
            offline_inference([OfflineInferenceConfig()], {}, assembled)
        mock_batch.assert_not_called()

    @patch(_PATCH_TARGET)
    def test_custom_assembled_model_needs_no_deployable(self, mock_batch):
        """A custom model is scored from its raw package, deployable or not."""
        mock_batch.return_value = ({}, [])
        assembled = _assembled(TRAINING_FRAMEWORK_CUSTOM)
        assembled.deployable_model = None

        offline_inference([OfflineInferenceConfig()], {}, assembled)

        (cfg,) = mock_batch.call_args.args[0]
        self.assertEqual(cfg.model.model_path, "raw/path")

    @patch(_PATCH_TARGET)
    def test_unregistered_model_is_a_configuration_error(self, mock_batch):
        """An unknown name or revision fails like any other config mistake."""
        registry = InMemoryRegistryClient()
        registry.register_model("m", artifact_uri="s3://raw")
        for model, match in (
            (ModelIdentifier(name="typo"), "'typo' revision None not found"),
            (ModelIdentifier(name="m", revision_id=9), "'m' revision 9 not found"),
        ):
            with (
                self.subTest(model=model),
                self.assertRaisesRegex(ConfigurationError, match),
            ):
                offline_inference(
                    [OfflineInferenceConfig(model=model)],
                    {},
                    storage_backend=LocalStorageBackend("/tmp"),
                    registry_client=registry,
                )
        mock_batch.assert_not_called()

    @patch(_PATCH_TARGET)
    def test_latest_registry_version_fills_revision_id(self, mock_batch):
        """Without a revision, the version the registry returned is recorded."""
        mock_batch.return_value = ({}, [])
        registry = InMemoryRegistryClient()
        for _ in range(3):
            registry.register_model(
                "m", artifact_uri="s3://raw", deployable_artifact_uri="s3://dep"
            )

        offline_inference(
            [OfflineInferenceConfig(model=ModelIdentifier(name="m"))],
            {},
            storage_backend=LocalStorageBackend("/tmp"),
            registry_client=registry,
        )

        (cfg,) = mock_batch.call_args.args[0]
        self.assertEqual((cfg.model.name, cfg.model.revision_id), ("m", 3))

    @patch(_PATCH_TARGET)
    def test_non_numeric_registry_version_leaves_revision_id_unset(self, mock_batch):
        """A version that is not a number cannot be recorded as a revision."""
        mock_batch.return_value = ({}, [])
        registry = MagicMock()
        registry.get_model.return_value.version = "champion"
        registry.get_model.return_value.labels = {}
        registry.get_model.return_value.deployable_artifact_uri = "s3://dep"

        offline_inference(
            [OfflineInferenceConfig(model=ModelIdentifier(name="m"))],
            {},
            storage_backend=LocalStorageBackend("/tmp"),
            registry_client=registry,
        )

        (cfg,) = mock_batch.call_args.args[0]
        self.assertIsNone(cfg.model.revision_id)

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


def _field_names(cls):
    return {f.name for f in dataclasses.fields(cls)}


def test_schema_options_match_the_library_options():
    """Every schema option has a library field to be forwarded to.

    ``offline_inference`` copies these field by field, so a field added to the
    schema and not to the library would be dropped silently.
    """
    assert _field_names(schema_inference.BatchOptions) == _field_names(
        lib_inference.BatchOptions
    )
    assert _field_names(schema_inference.PaddingOptions) == _field_names(
        lib_inference.PaddingOptions
    )
    assert _field_names(schema_inference.ModelIdentifier) <= _field_names(
        lib_inference.ModelIdentifier
    )
