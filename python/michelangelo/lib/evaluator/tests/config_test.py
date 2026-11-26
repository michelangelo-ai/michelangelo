"""Tests for michelangelo.lib.evaluator.config."""

from __future__ import annotations

import tempfile
from pathlib import Path
from unittest import TestCase

from michelangelo.lib.evaluator.config import (
    load_config_from_dict,
    load_config_from_yaml,
)
from michelangelo.workflow.schema.evaluator import (
    ProjectionMethod,
    SanityCheckOperator,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError


class LoadConfigFromDictTest(TestCase):
    """load_config_from_dict turns nested dicts into schema objects."""

    def test_empty_dict_yields_no_metrics(self):
        """Empty dict yields no metrics."""
        self.assertEqual(load_config_from_dict({}).metrics, [])

    def test_minimal_metric(self):
        """Minimal metric."""
        config = load_config_from_dict(
            {"metrics": [{"name": "auc", "metric": "torchmetrics.AUROC"}]}
        )
        self.assertEqual(len(config.metrics), 1)
        metric = config.metrics[0]
        self.assertEqual(metric.name, "auc")
        self.assertEqual(metric.metric, "torchmetrics.AUROC")
        self.assertEqual(metric.params, {})
        self.assertIsNone(metric.columns)
        self.assertIsNone(metric.filter_expr)

    def test_full_metric(self):
        """Full metric."""
        config = load_config_from_dict(
            {
                "metrics": [
                    {
                        "name": "auc",
                        "metric": "torchmetrics.classification.BinaryAUROC",
                        "params": {"task": "binary"},
                        "filter_expr": "label >= 0",
                        "columns": {
                            "prediction_col": "pred",
                            "target_col": "label",
                            "index_col": "query_id",
                            "sample_weight_col": "weight",
                            "extra_cols": {"cost": "cost_col"},
                        },
                    }
                ]
            }
        )
        metric = config.metrics[0]
        self.assertEqual(metric.params, {"task": "binary"})
        self.assertEqual(metric.filter_expr, "label >= 0")
        self.assertEqual(metric.columns.prediction_col, "pred")
        self.assertEqual(metric.columns.target_col, "label")
        self.assertEqual(metric.columns.index_col, "query_id")
        self.assertEqual(metric.columns.sample_weight_col, "weight")
        self.assertEqual(metric.columns.extra_cols, {"cost": "cost_col"})

    def test_optional_column_fields_default_to_none(self):
        """Optional column fields default to none."""
        config = load_config_from_dict(
            {
                "metrics": [
                    {
                        "name": "auc",
                        "metric": "torchmetrics.AUROC",
                        "columns": {"prediction_col": "p", "target_col": "t"},
                    }
                ]
            }
        )
        columns = config.metrics[0].columns
        self.assertIsNone(columns.index_col)
        self.assertIsNone(columns.sample_weight_col)
        self.assertIsNone(columns.extra_cols)

    def test_several_metrics_keep_their_order(self):
        """Several metrics keep their order."""
        config = load_config_from_dict(
            {
                "metrics": [
                    {"name": "a", "metric": "torchmetrics.AUROC"},
                    {"name": "b", "metric": "torchmetrics.MeanSquaredError"},
                ]
            }
        )
        self.assertEqual([m.name for m in config.metrics], ["a", "b"])


class LoadConfigFromYamlTest(TestCase):
    """load_config_from_yaml parses the same shape from a file."""

    def test_round_trip(self):
        """Round trip."""
        yaml_text = (
            "metrics:\n"
            "  - name: auc\n"
            "    metric: torchmetrics.classification.BinaryAUROC\n"
            "    columns:\n"
            "      prediction_col: pred\n"
            "      target_col: label\n"
        )
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "evaluator.yaml"
            path.write_text(yaml_text)
            config = load_config_from_yaml(str(path))

        self.assertEqual(len(config.metrics), 1)
        self.assertEqual(config.metrics[0].columns.target_col, "label")


_AUC = {
    "name": "auc",
    "metric": "torchmetrics.classification.BinaryAUROC",
    "columns": {"prediction_col": "score", "target_col": "label"},
}


class LoadConfigTopLevelFieldsTest(TestCase):
    """Fields besides ``metrics`` survive loading instead of being dropped."""

    def test_sanity_checks(self):
        """Sanity checks become SanityCheck objects with a parsed operator."""
        config = load_config_from_dict(
            {
                "metrics": [_AUC],
                "sanity_checks": [
                    {"metric": "auc", "threshold": 0.55, "operator": ">="}
                ],
            }
        )
        (check,) = config.sanity_checks
        self.assertEqual(check.metric, "auc")
        self.assertEqual(check.threshold, 0.55)
        self.assertEqual(check.operator, SanityCheckOperator.GTE)

    def test_embedding_projections(self):
        """Projections become EmbeddingProjection objects with a parsed method."""
        config = load_config_from_dict(
            {
                "metrics": [_AUC],
                "embedding_projections": [
                    {
                        "name": "users",
                        "embedding_col": "user_embedding",
                        "color_cols": ["country"],
                        "method": "pca",
                    }
                ],
            }
        )
        (projection,) = config.embedding_projections
        self.assertEqual(projection.embedding_col, "user_embedding")
        self.assertEqual(projection.color_cols, ["country"])
        self.assertEqual(projection.method, ProjectionMethod.PCA)

    def test_scalar_fields(self):
        """Segment and preprocessing settings pass through."""
        config = load_config_from_dict(
            {
                "metrics": [_AUC],
                "segment_columns": ["region"],
                "enable_global_segment_evaluation": False,
                "min_segment_row_count": 10,
                "data_preprocessor": "my_pkg.prep.explode",
                "segment_shuffle_partitions": 64,
            }
        )
        self.assertEqual(config.segment_columns, ["region"])
        self.assertFalse(config.enable_global_segment_evaluation)
        self.assertEqual(config.min_segment_row_count, 10)
        self.assertEqual(config.data_preprocessor, "my_pkg.prep.explode")
        self.assertEqual(config.segment_shuffle_partitions, 64)

    def test_defaults_when_absent(self):
        """Absent fields keep the EvaluatorConfig defaults."""
        config = load_config_from_dict({"metrics": [_AUC]})
        self.assertEqual(config.sanity_checks, [])
        self.assertEqual(config.embedding_projections, [])
        self.assertEqual(config.segment_columns, [])
        self.assertTrue(config.enable_global_segment_evaluation)

    def test_schema_validation_still_runs(self):
        """A sanity check on an unknown metric fails as it would in code."""
        with self.assertRaisesRegex(ConfigurationError, "unknown metric"):
            load_config_from_dict(
                {
                    "metrics": [_AUC],
                    "sanity_checks": [
                        {"metric": "nope", "threshold": 0.5, "operator": ">="}
                    ],
                }
            )

    def test_unknown_top_level_key_raises(self):
        """A misspelled key fails instead of being silently dropped."""
        with self.assertRaisesRegex(ConfigurationError, "sanity_check"):
            load_config_from_dict({"metrics": [_AUC], "sanity_check": []})

    def test_unknown_entry_field_raises(self):
        """An unknown field in an entry names the entry."""
        with self.assertRaisesRegex(ConfigurationError, r"sanity_checks\[0\]"):
            load_config_from_dict(
                {
                    "metrics": [_AUC],
                    "sanity_checks": [{"metric": "auc", "threshold": 0.5, "op": ">="}],
                }
            )

    def test_yaml_keeps_sanity_checks(self):
        """A YAML config keeps its sanity checks."""
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "config.yaml"
            path.write_text(
                "metrics:\n"
                "  - name: auc\n"
                "    metric: torchmetrics.classification.BinaryAUROC\n"
                "    columns: {prediction_col: score, target_col: label}\n"
                "sanity_checks:\n"
                '  - {metric: auc, threshold: 0.55, operator: ">="}\n'
            )
            config = load_config_from_yaml(str(path))
        self.assertEqual(len(config.sanity_checks), 1)
