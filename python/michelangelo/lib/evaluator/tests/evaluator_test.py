"""Tests for michelangelo.lib.evaluator.evaluator."""

from __future__ import annotations

import tempfile
from pathlib import Path
from unittest import TestCase

import pandas as pd
import torch
from pandas.errors import UndefinedVariableError
from torchmetrics import Metric

from michelangelo.lib.evaluator.evaluator import (
    TorchMetricEvaluator,
    create_evaluator,
)
from michelangelo.workflow.schema.evaluator import (
    ColumnMapping,
    EvaluatorConfig,
    MetricConfig,
)


class _SumOfCosts(Metric):
    """Custom metric that consumes an extra column through **kwargs."""

    _metric_type = "classification"

    def __init__(self) -> None:
        super().__init__()
        self.add_state("total", default=torch.tensor(0.0))

    def update(self, preds: torch.Tensor, target: torch.Tensor, **kwargs) -> None:
        self.total += kwargs["cost"].sum()

    def compute(self) -> torch.Tensor:
        return self.total


class _IgnoresExtras(Metric):
    """Custom metric whose update() takes no **kwargs."""

    _metric_type = "classification"

    def __init__(self) -> None:
        super().__init__()
        self.add_state("count", default=torch.tensor(0.0))

    def update(self, preds: torch.Tensor, target: torch.Tensor) -> None:
        self.count += preds.numel()

    def compute(self) -> torch.Tensor:
        return self.count


_DF = pd.DataFrame(
    {
        "pred": [0.9, 0.8, 0.3, 0.1],
        "label": [1, 1, 0, 0],
        "cost": [1.0, 2.0, 3.0, 4.0],
        "city": ["SF", "SF", "LA", "LA"],
    }
)


def _config(**kwargs) -> EvaluatorConfig:
    """Config with a single binary AUROC over (pred, label)."""
    return EvaluatorConfig(
        metrics=[
            MetricConfig(
                name="auroc",
                metric="torchmetrics.classification.BinaryAUROC",
                columns=ColumnMapping(prediction_col="pred", target_col="label"),
            )
        ],
        **kwargs,
    )


class ConstructionTest(TestCase):
    """The evaluator accepts every documented config form."""

    def test_from_config_object(self):
        """From config object."""
        evaluator = TorchMetricEvaluator(_config())
        self.assertEqual(evaluator.get_metric_names(), ["auroc"])

    def test_from_dict(self):
        """From dict."""
        evaluator = TorchMetricEvaluator(
            {
                "metrics": [
                    {
                        "name": "auroc",
                        "metric": "torchmetrics.classification.BinaryAUROC",
                        "columns": {"prediction_col": "pred", "target_col": "label"},
                    }
                ]
            }
        )
        self.assertEqual(evaluator.get_metric_names(), ["auroc"])

    def test_from_yaml_path(self):
        """From yaml path."""
        yaml_text = (
            "metrics:\n"
            "  - name: auroc\n"
            "    metric: torchmetrics.classification.BinaryAUROC\n"
            "    columns:\n"
            "      prediction_col: pred\n"
            "      target_col: label\n"
        )
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "evaluator.yaml"
            path.write_text(yaml_text)
            self.assertEqual(TorchMetricEvaluator(path).get_metric_names(), ["auroc"])

    def test_invalid_config_type_raises(self):
        """Invalid config type raises."""
        with self.assertRaises(TypeError):
            TorchMetricEvaluator(42)

    def test_create_evaluator_helper(self):
        """Create evaluator helper."""
        self.assertIsInstance(create_evaluator(_config()), TorchMetricEvaluator)

    def test_repr_reports_counts(self):
        """Repr reports counts."""
        self.assertEqual(
            repr(TorchMetricEvaluator(_config())),
            "TorchMetricEvaluator(metrics=1, collections=1)",
        )


class EvaluateTest(TestCase):
    """evaluate() computes every configured metric over a DataFrame."""

    def test_computes_a_scalar_metric(self):
        """Computes a scalar metric."""
        results = TorchMetricEvaluator(_config()).evaluate(_DF)
        self.assertEqual(set(results), {"auroc"})
        self.assertAlmostEqual(results["auroc"], 1.0)

    def test_filter_expr_restricts_the_rows(self):
        """Filter expr restricts the rows."""
        config = EvaluatorConfig(
            metrics=[
                MetricConfig(
                    name="count",
                    metric=f"{__name__}._IgnoresExtras",
                    columns=ColumnMapping(prediction_col="pred", target_col="label"),
                    filter_expr="pred > 0.5",
                )
            ]
        )
        results = TorchMetricEvaluator(config).evaluate(_DF)
        self.assertEqual(results["count"], 2.0)

    def test_aggregation_metric_needs_no_target(self):
        """Aggregation metric needs no target."""
        config = EvaluatorConfig(
            metrics=[
                MetricConfig(
                    name="mean_pred",
                    metric="torchmetrics.aggregation.MeanMetric",
                    columns=ColumnMapping(prediction_col="pred", target_col="label"),
                )
            ]
        )
        results = TorchMetricEvaluator(config).evaluate(_DF)
        self.assertAlmostEqual(results["mean_pred"], 0.525, places=5)

    def test_extra_columns_reach_a_custom_metric(self):
        """Extra columns reach a custom metric."""
        config = EvaluatorConfig(
            metrics=[
                MetricConfig(
                    name="cost",
                    metric=f"{__name__}._SumOfCosts",
                    columns=ColumnMapping(
                        prediction_col="pred",
                        target_col="label",
                        extra_cols={"cost": "cost"},
                    ),
                )
            ]
        )
        results = TorchMetricEvaluator(config).evaluate(_DF)
        self.assertAlmostEqual(results["cost"], 10.0)

    def test_extra_columns_are_withheld_from_metrics_that_reject_them(self):
        """Extra columns are withheld from metrics that reject them."""
        config = EvaluatorConfig(
            metrics=[
                MetricConfig(
                    name="count",
                    metric=f"{__name__}._IgnoresExtras",
                    columns=ColumnMapping(
                        prediction_col="pred",
                        target_col="label",
                        extra_cols={"cost": "cost"},
                    ),
                )
            ]
        )
        results = TorchMetricEvaluator(config).evaluate(_DF)
        self.assertEqual(results["count"], 4.0)

    def test_multi_element_results_are_not_scalarized(self):
        """Multi element results are not scalarized."""
        config = EvaluatorConfig(
            metrics=[
                MetricConfig(
                    name="per_class",
                    metric="torchmetrics.classification.MulticlassAUROC",
                    params={"num_classes": 2, "average": None},
                    columns=ColumnMapping(prediction_col="probs", target_col="label"),
                )
            ]
        )
        df = pd.DataFrame(
            {"probs": [[0.9, 0.1], [0.2, 0.8], [0.7, 0.3]], "label": [0, 1, 0]}
        )
        results = TorchMetricEvaluator(config).evaluate(df)
        self.assertIsInstance(results["per_class"], torch.Tensor)
        self.assertEqual(results["per_class"].numel(), 2)

    def test_evaluating_twice_does_not_accumulate_state(self):
        """Evaluating twice does not accumulate state."""
        evaluator = TorchMetricEvaluator(_config())
        first = evaluator.evaluate(_DF)
        second = evaluator.evaluate(_DF)
        self.assertAlmostEqual(first["auroc"], second["auroc"])

    def test_unusable_filter_expression_raises(self):
        """Unusable filter expression raises."""
        config = EvaluatorConfig(
            metrics=[
                MetricConfig(
                    name="auroc",
                    metric="torchmetrics.classification.BinaryAUROC",
                    columns=ColumnMapping(prediction_col="pred", target_col="label"),
                    filter_expr="no_such_column > 1",
                )
            ]
        )
        with self.assertRaises(UndefinedVariableError):
            TorchMetricEvaluator(config).evaluate(_DF)


class ComputeTest(TestCase):
    """compute() reads whatever state the collections currently hold."""

    def test_computes_from_accumulated_state(self):
        """Computes from accumulated state."""
        evaluator = TorchMetricEvaluator(_config())
        collection = next(iter(evaluator.collections.values()))
        collection.update(
            torch.tensor([0.9, 0.1]), torch.tensor([1, 0], dtype=torch.long)
        )
        self.assertAlmostEqual(evaluator.compute()["auroc"], 1.0)


class ColumnIntrospectionTest(TestCase):
    """The evaluator reports which columns its metrics need."""

    def test_required_columns(self):
        """Required columns."""
        config = EvaluatorConfig(
            metrics=[
                MetricConfig(
                    name="auroc",
                    metric="torchmetrics.classification.BinaryAUROC",
                    columns=ColumnMapping(
                        prediction_col="pred",
                        target_col="label",
                        index_col="query",
                        sample_weight_col="weight",
                        extra_cols={"cost": "cost_col"},
                    ),
                )
            ]
        )
        self.assertEqual(
            TorchMetricEvaluator(config).get_required_columns(),
            {"pred", "label", "query", "weight", "cost_col"},
        )

    def test_segment_columns_are_required_too(self):
        """Segment columns are required too."""
        config = _config(segment_columns=["city"])
        self.assertIn("city", TorchMetricEvaluator(config).get_required_columns())

    def test_column_mappings_are_listed(self):
        """Column mappings are listed."""
        mappings = TorchMetricEvaluator(_config()).get_column_mappings()
        self.assertEqual(len(mappings), 1)
        self.assertEqual(mappings[0].prediction_col, "pred")

    def test_validate_dataset_accepts_a_complete_frame(self):
        """Validate dataset accepts a complete frame."""
        TorchMetricEvaluator(_config()).validate_dataset(_DF)

    def test_validate_dataset_names_every_missing_column(self):
        """Validate dataset names every missing column."""
        config = EvaluatorConfig(
            metrics=[
                MetricConfig(
                    name="auroc",
                    metric="torchmetrics.classification.BinaryAUROC",
                    columns=ColumnMapping(
                        prediction_col="missing_pred",
                        target_col="missing_label",
                        index_col="missing_index",
                        sample_weight_col="missing_weight",
                    ),
                )
            ]
        )
        with self.assertRaises(ValueError) as ctx:
            TorchMetricEvaluator(config).validate_dataset(_DF)
        message = str(ctx.exception)
        for expected in (
            "prediction_col:missing_pred",
            "target_col:missing_label",
            "index_col:missing_index",
            "sample_weight_col:missing_weight",
        ):
            self.assertIn(expected, message)
