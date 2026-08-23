"""Tests for tabular_native_transform._private.incremental_training."""

from __future__ import annotations

import os
import tempfile
from unittest import TestCase

import pytest
import yaml

torch = pytest.importorskip("torch")
pytest.importorskip("pydantic")

from michelangelo.lib.artifact_manager.storage_backend import (  # noqa: E402
    LocalStorageBackend,
)
from michelangelo.lib.native_transform.torch.transform_layer_spec import (  # noqa: E402
    TransformerMode,
)
from michelangelo.lib.native_transform.torch.transform_spec import (  # noqa: E402
    TransformSpec,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError  # noqa: E402
from michelangelo.workflow.schema.tabular_native_transform import (  # noqa: E402
    IncrementalTrainingConfig,
    TrainingType,
)
from michelangelo.workflow.tasks.tabular_native_transform._private import (  # noqa: E402
    incremental_training,
)

_BASE_RAW_SPEC = {
    "transform_specs": [
        {
            "transform_name": "Concatenate",
            "input_cols": ["col1"],
            "output_cols": ["col1_concatenated"],
        },
        {
            "transform_name": "StandardScaler",
            "input_cols": ["col3"],
            "output_cols": ["col3_scaled"],
            "with_std": True,
        },
    ]
}


def _fitted_base_spec_dict() -> dict:
    """Build a base spec dict as it would be persisted after a BASE run fit.

    The ``StandardScaler`` placeholder resolves to a concrete
    ``NormalizationLayerSpec`` once fitted.
    """
    spec = TransformSpec(raw_transform_specs=_BASE_RAW_SPEC)
    # Simulate fitting: hydrate the StandardScaler placeholder.
    spec.update_standard_scaler_specs({"col3_mean": 1.0, "col3_std": 2.0})
    return spec.to_dict()


class IsIncrementalIsBaselineTest(TestCase):
    """Tests for is_incremental and is_baseline."""

    def test_is_incremental_none_config(self):
        """A None config is not incremental."""
        self.assertFalse(incremental_training.is_incremental(None))

    def test_is_incremental_true(self):
        """TrainingType.INCREMENTAL is reported as incremental."""
        cfg = IncrementalTrainingConfig(training_type=TrainingType.INCREMENTAL)
        self.assertTrue(incremental_training.is_incremental(cfg))

    def test_is_incremental_false_for_base(self):
        """TrainingType.BASE is not reported as incremental."""
        cfg = IncrementalTrainingConfig(training_type=TrainingType.BASE)
        self.assertFalse(incremental_training.is_incremental(cfg))

    def test_is_baseline_true(self):
        """TrainingType.BASE is reported as a baseline run."""
        cfg = IncrementalTrainingConfig(training_type=TrainingType.BASE)
        self.assertTrue(incremental_training.is_baseline(cfg))

    def test_is_baseline_none_config(self):
        """A None config is not a baseline run."""
        self.assertFalse(incremental_training.is_baseline(None))


class LoadIncrementalArtifactsTest(TestCase):
    """Tests for load_incremental_artifacts."""

    def setUp(self):
        """Create a temp-dir-backed LocalStorageBackend for each test."""
        self._tmp = tempfile.mkdtemp()
        self.backend = LocalStorageBackend(base_dir=self._tmp)

    def _upload_base_run(self, spec_dict, feature_stats=None) -> str:
        """Upload a fake base run's artifacts and return its URI."""
        src = tempfile.mkdtemp()
        with open(os.path.join(src, "transform_spec.yaml"), "w") as f:
            yaml.safe_dump(spec_dict, f)
        if feature_stats is not None:
            with open(os.path.join(src, "transform_feature_stats.yaml"), "w") as f:
                yaml.safe_dump(feature_stats, f)
        return self.backend.upload(src, "base-run")

    def test_requires_baseline_model_uri(self):
        """A missing baseline_model_uri raises ConfigurationError."""
        cfg = IncrementalTrainingConfig(training_type=TrainingType.INCREMENTAL)
        with self.assertRaises(ConfigurationError):
            incremental_training.load_incremental_artifacts(cfg, self.backend)

    def test_loads_spec_and_stats(self):
        """The transform spec and feature stats are loaded from the base run."""
        uri = self._upload_base_run(
            _fitted_base_spec_dict(), feature_stats={"col3_mean": 1.0, "col3_std": 2.0}
        )
        cfg = IncrementalTrainingConfig(
            training_type=TrainingType.INCREMENTAL, baseline_model_uri=uri
        )
        spec, stats = incremental_training.load_incremental_artifacts(cfg, self.backend)
        self.assertIsInstance(spec, TransformSpec)
        self.assertEqual(stats, {"col3_mean": 1.0, "col3_std": 2.0})

    def test_missing_feature_stats_file_returns_empty_dict(self):
        """A base run with no feature stats file yields an empty stats dict."""
        uri = self._upload_base_run(_fitted_base_spec_dict())
        cfg = IncrementalTrainingConfig(
            training_type=TrainingType.INCREMENTAL, baseline_model_uri=uri
        )
        _, stats = incremental_training.load_incremental_artifacts(cfg, self.backend)
        self.assertEqual(stats, {})

    def test_missing_transform_spec_file_raises(self):
        """A base run with no transform_spec.yaml raises ConfigurationError."""
        src = tempfile.mkdtemp()
        with open(os.path.join(src, "unrelated.txt"), "w") as f:
            f.write("x")
        uri = self.backend.upload(src, "empty-base-run")
        cfg = IncrementalTrainingConfig(
            training_type=TrainingType.INCREMENTAL, baseline_model_uri=uri
        )
        with self.assertRaises(ConfigurationError):
            incremental_training.load_incremental_artifacts(cfg, self.backend)

    def test_enforce_full_reuse_rejects_non_reuse_layer(self):
        """enforce_full_reuse=True rejects a base spec with a non-REUSE layer."""
        spec = TransformSpec(raw_transform_specs=_BASE_RAW_SPEC)
        spec.update_standard_scaler_specs({"col3_mean": 1.0, "col3_std": 2.0})
        for layer_spec in spec.transform_specs.values():
            layer_spec.mode = TransformerMode.REFIT
        uri = self._upload_base_run(spec.to_dict())
        cfg = IncrementalTrainingConfig(
            training_type=TrainingType.INCREMENTAL,
            baseline_model_uri=uri,
            enforce_full_reuse=True,
        )
        with self.assertRaises(ConfigurationError):
            incremental_training.load_incremental_artifacts(cfg, self.backend)

    def test_enforce_full_reuse_false_skips_validation(self):
        """enforce_full_reuse=False skips the REUSE-mode validation."""
        spec = TransformSpec(raw_transform_specs=_BASE_RAW_SPEC)
        spec.update_standard_scaler_specs({"col3_mean": 1.0, "col3_std": 2.0})
        for layer_spec in spec.transform_specs.values():
            layer_spec.mode = TransformerMode.REFIT
        uri = self._upload_base_run(spec.to_dict())
        cfg = IncrementalTrainingConfig(
            training_type=TrainingType.INCREMENTAL,
            baseline_model_uri=uri,
            enforce_full_reuse=False,
        )
        loaded_spec, _ = incremental_training.load_incremental_artifacts(
            cfg, self.backend
        )
        self.assertIsInstance(loaded_spec, TransformSpec)


class MergeSpecsForSelectiveRefitTest(TestCase):
    """Tests for merge_specs_for_selective_refit."""

    def _base_spec(self) -> TransformSpec:
        """Build a fitted, all-REUSE base spec fixture."""
        spec = TransformSpec(raw_transform_specs=_BASE_RAW_SPEC)
        spec.update_standard_scaler_specs({"col3_mean": 1.0, "col3_std": 2.0})
        for layer_spec in spec.transform_specs.values():
            layer_spec.mode = TransformerMode.REUSE
        return spec

    def test_config_layer_refits_matching_base_layer(self):
        """A REFIT config layer replaces the matching base layer and drops its stats."""
        base_spec = self._base_spec()
        config_spec = TransformSpec(raw_transform_specs=_BASE_RAW_SPEC)
        for layer_spec in config_spec.transform_specs.values():
            if type(layer_spec).__name__ == "StandardScalerLayerSpec":
                layer_spec.mode = TransformerMode.REFIT

        merged, filtered_stats = incremental_training.merge_specs_for_selective_refit(
            base_spec, config_spec, {"col3_mean": 1.0, "col3_std": 2.0}
        )

        modes = {type(ls).__name__: ls.mode for ls in merged.transform_specs.values()}
        self.assertEqual(modes["StandardScalerLayerSpec"], TransformerMode.REFIT)
        # REFIT layer's stats are dropped so the pipeline recomputes them.
        self.assertEqual(filtered_stats, {})

    def test_non_refit_layer_not_matching_base_raises(self):
        """A non-REFIT config layer with no matching base layer raises."""
        base_spec = self._base_spec()
        config_spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["unrelated_col"],
                        "output_cols": ["unrelated_out"],
                    },
                ]
            }
        )
        with self.assertRaises(ConfigurationError):
            incremental_training.merge_specs_for_selective_refit(
                base_spec, config_spec, {}
            )

    def test_reuse_layer_consuming_refit_output_logs_warning(self):
        """A REUSE layer downstream of a REFIT layer merges without raising."""
        base_spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["col1"],
                        "output_cols": ["col1_concatenated"],
                    },
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["col1_concatenated"],
                        "output_cols": ["col1_final"],
                    },
                ]
            }
        )
        for layer_spec in base_spec.transform_specs.values():
            layer_spec.mode = TransformerMode.REUSE

        config_spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Concatenate",
                        "input_cols": ["col1"],
                        "output_cols": ["col1_concatenated"],
                        "mode": TransformerMode.REFIT.value,
                    },
                ]
            }
        )

        merged, _ = incremental_training.merge_specs_for_selective_refit(
            base_spec, config_spec, {}
        )
        self.assertEqual(len(merged.transform_specs), 2)
