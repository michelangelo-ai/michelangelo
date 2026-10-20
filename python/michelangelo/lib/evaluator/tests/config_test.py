"""Tests for michelangelo.lib.evaluator.config."""

from __future__ import annotations

import tempfile
from pathlib import Path
from unittest import TestCase

from michelangelo.lib.evaluator.config import (
    load_config_from_dict,
    load_config_from_yaml,
)


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
