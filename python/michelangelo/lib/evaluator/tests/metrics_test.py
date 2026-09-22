"""Tests for michelangelo.lib.evaluator.metrics."""

from __future__ import annotations

from unittest import TestCase

import numpy as np
import pandas as pd
import torch
from torchmetrics import Metric

from michelangelo.lib.evaluator.metrics import (
    TASK_TYPE_TO_DTYPES,
    cast_array_to_dtype,
    create_metric_collections,
    create_metric_from_path,
    encode_categorical_column_to_tensor,
    extract_data_for_collection,
    infer_task_type_from_path,
    metric_state_for_logging,
    resolve_dtypes_for_metric,
)
from michelangelo.workflow.schema.evaluator import (
    ColumnMapping,
    EvaluatorConfig,
    MetricConfig,
)


class _RegressionTagged(Metric):
    """Custom metric declaring its task via _metric_type."""

    _metric_type = "regression"

    def __init__(self) -> None:
        super().__init__()
        self.add_state("total", default=torch.tensor(0.0))

    def update(self, preds: torch.Tensor, target: torch.Tensor) -> None:
        self.total += (preds - target).abs().sum()

    def compute(self) -> torch.Tensor:
        return self.total


class _ExplicitDtypes(_RegressionTagged):
    """Custom metric overriding dtypes outright."""

    _dtypes = (torch.float64, torch.int32)


class CastArrayToDtypeTest(TestCase):
    """cast_array_to_dtype converts without aliasing the source."""

    def test_casts_to_the_requested_dtype(self):
        """Casts to the requested dtype."""
        tensor = cast_array_to_dtype(np.array([1, 2, 3]), torch.float32)
        self.assertEqual(tensor.dtype, torch.float32)
        self.assertEqual(tensor.tolist(), [1.0, 2.0, 3.0])

    def test_does_not_alias_the_source_array(self):
        """Does not alias the source array."""
        array = np.array([1.0, 2.0])
        tensor = cast_array_to_dtype(array, torch.float32)
        array[0] = 99.0
        self.assertEqual(tensor[0].item(), 1.0)


class EncodeCategoricalColumnTest(TestCase):
    """encode_categorical_column_to_tensor handles string columns."""

    def test_strings_are_factorized(self):
        """Strings are factorized."""
        tensor = encode_categorical_column_to_tensor(np.array(["a", "b", "a", "c"]))
        self.assertEqual(tensor.tolist(), [0, 1, 0, 2])
        self.assertEqual(tensor.dtype, torch.long)

    def test_numeric_input_is_cast_directly(self):
        """Numeric input is cast directly."""
        tensor = encode_categorical_column_to_tensor(
            np.array([1.0, 2.5]), torch.float, factorize_strings=False
        )
        self.assertEqual(tensor.tolist(), [1.0, 2.5])

    def test_factorization_can_be_disabled_for_numeric_columns(self):
        """Factorization can be disabled for numeric columns."""
        tensor = encode_categorical_column_to_tensor(
            np.array([3, 1, 3]), torch.long, factorize_strings=False
        )
        self.assertEqual(tensor.tolist(), [3, 1, 3])


class ExtractDataForCollectionTest(TestCase):
    """extract_data_for_collection pulls exactly the configured columns."""

    @staticmethod
    def _df() -> pd.DataFrame:
        return pd.DataFrame(
            {
                "pred": [0.9, 0.1],
                "label": [1, 0],
                "query": ["q1", "q2"],
                "weight": [1.0, 2.0],
                "cost": [3.0, 4.0],
            }
        )

    def test_predictions_and_targets(self):
        """Predictions and targets."""
        data = extract_data_for_collection(
            self._df(),
            ColumnMapping(prediction_col="pred", target_col="label"),
            torch.float32,
            torch.long,
        )
        self.assertEqual(set(data), {"predictions", "targets"})
        self.assertEqual(data["predictions"].dtype, torch.float32)
        self.assertEqual(data["targets"].dtype, torch.long)

    def test_no_target_dtype_skips_the_target_column(self):
        """No target dtype skips the target column."""
        data = extract_data_for_collection(
            self._df(),
            ColumnMapping(prediction_col="pred", target_col="label"),
            torch.float32,
            None,
        )
        self.assertNotIn("targets", data)

    def test_optional_columns_are_included(self):
        """Optional columns are included."""
        data = extract_data_for_collection(
            self._df(),
            ColumnMapping(
                prediction_col="pred",
                target_col="label",
                index_col="query",
                sample_weight_col="weight",
                extra_cols={"cost": "cost"},
            ),
            torch.float32,
            torch.long,
        )
        self.assertEqual(data["indexes"].tolist(), [0, 1])
        self.assertEqual(data["sample_weights"].tolist(), [1.0, 2.0])
        self.assertEqual(data["extra_cols"]["cost"].tolist(), [3.0, 4.0])

    def test_absent_optional_columns_are_ignored(self):
        """Absent optional columns are ignored."""
        data = extract_data_for_collection(
            self._df(),
            ColumnMapping(
                prediction_col="pred",
                target_col="label",
                index_col="missing",
                extra_cols={"cost": "missing"},
            ),
            torch.float32,
            torch.long,
        )
        self.assertNotIn("indexes", data)
        self.assertNotIn("extra_cols", data)

    def test_list_valued_predictions_are_stacked(self):
        """List valued predictions are stacked."""
        df = pd.DataFrame({"pred": [[0.1, 0.9], [0.8, 0.2]], "label": [1, 0]})
        data = extract_data_for_collection(
            df,
            ColumnMapping(prediction_col="pred", target_col="label"),
            torch.float32,
            torch.long,
        )
        self.assertEqual(tuple(data["predictions"].shape), (2, 2))


class InferTaskTypeTest(TestCase):
    """infer_task_type_from_path reads the metric's module hierarchy."""

    def test_explicit_submodule_in_the_path(self):
        """Explicit submodule in the path."""
        cases = {
            "torchmetrics.classification.BinaryAUROC": "classification",
            "torchmetrics.regression.MeanSquaredError": "regression",
            "torchmetrics.retrieval.RetrievalMAP": "retrieval",
            "torchmetrics.aggregation.MeanMetric": "aggregation",
        }
        for path, expected in cases.items():
            with self.subTest(path=path):
                self.assertEqual(infer_task_type_from_path(path), expected)

    def test_shorthand_path_is_resolved_by_import(self):
        """Shorthand path is resolved by import."""
        self.assertEqual(
            infer_task_type_from_path("torchmetrics.MeanSquaredError"), "regression"
        )

    def test_unresolvable_path_falls_back_to_classification(self):
        """Unresolvable path falls back to classification."""
        self.assertEqual(
            infer_task_type_from_path("no.such.module.Metric"), "classification"
        )


class ResolveDtypesTest(TestCase):
    """resolve_dtypes_for_metric prefers explicit declarations."""

    def test_explicit_dtypes_win(self):
        """Explicit dtypes win."""
        dtypes = resolve_dtypes_for_metric("whatever.Metric", _ExplicitDtypes())
        self.assertEqual(dtypes, (torch.float64, torch.int32))

    def test_metric_type_tag_is_used(self):
        """Metric type tag is used."""
        dtypes = resolve_dtypes_for_metric("whatever.Metric", _RegressionTagged())
        self.assertEqual(dtypes, TASK_TYPE_TO_DTYPES["regression"])

    def test_path_inference_is_the_fallback(self):
        """Path inference is the fallback."""
        from torchmetrics.classification import BinaryAUROC

        dtypes = resolve_dtypes_for_metric(
            "torchmetrics.classification.BinaryAUROC", BinaryAUROC()
        )
        self.assertEqual(dtypes, TASK_TYPE_TO_DTYPES["classification"])

    def test_aggregation_has_no_target_dtype(self):
        """Aggregation has no target dtype."""
        self.assertIsNone(TASK_TYPE_TO_DTYPES["aggregation"][1])


class CreateMetricFromPathTest(TestCase):
    """create_metric_from_path imports and instantiates by dotted path."""

    def test_creates_the_class(self):
        """Creates the class."""
        metric = create_metric_from_path("torchmetrics.classification.BinaryAUROC", {})
        self.assertEqual(type(metric).__name__, "BinaryAUROC")

    def test_params_are_forwarded(self):
        """Params are forwarded."""
        metric = create_metric_from_path(
            "torchmetrics.classification.MulticlassAUROC", {"num_classes": 3}
        )
        self.assertEqual(metric.num_classes, 3)

    def test_int_params_are_coerced_for_float_signatures(self):
        """Int params are coerced for float signatures."""
        # FBetaScore.beta validates with a strict isinstance(x, float) check,
        # which a YAML 2.0 arriving as int 2 through proto Struct would fail.
        metric = create_metric_from_path(
            "torchmetrics.FBetaScore", {"task": "binary", "beta": 2}
        )
        self.assertIsInstance(metric.beta, float)

    def test_numeric_string_params_are_coerced(self):
        """Numeric string params are coerced."""
        metric = create_metric_from_path(
            "torchmetrics.FBetaScore", {"task": "binary", "beta": "2.0"}
        )
        self.assertEqual(metric.beta, 2.0)

    def test_non_numeric_string_params_are_left_alone(self):
        """Non numeric string params are left alone."""
        metric = create_metric_from_path(
            "torchmetrics.FBetaScore",
            {"task": "binary", "beta": 2.0, "multidim_average": "global"},
        )
        self.assertEqual(metric.multidim_average, "global")

    def test_unknown_path_raises(self):
        """Unknown path raises."""
        with self.assertRaises(ModuleNotFoundError):
            create_metric_from_path("no.such.module.Metric", {})


class MetricStateForLoggingTest(TestCase):
    """metric_state_for_logging surfaces tensor attributes."""

    def test_reads_accumulated_state(self):
        """Reads accumulated state."""
        metric = _RegressionTagged()
        metric.update(torch.tensor([3.0]), torch.tensor([1.0]))
        self.assertEqual(metric_state_for_logging(metric)["total"], 2.0)

    def test_multi_element_state_is_listed(self):
        """Multi element state is listed."""
        metric = _RegressionTagged()
        metric.total = torch.tensor([1.0, 2.0])
        self.assertEqual(metric_state_for_logging(metric)["total"], [1.0, 2.0])


class CreateMetricCollectionsTest(TestCase):
    """create_metric_collections bundles metrics that want the same data."""

    @staticmethod
    def _metric(name: str, path: str, **kwargs) -> MetricConfig:
        return MetricConfig(
            name=name,
            metric=path,
            columns=ColumnMapping(prediction_col="pred", target_col="label"),
            **kwargs,
        )

    def test_same_data_and_dtypes_share_a_collection(self):
        """Same data and dtypes share a collection."""
        config = EvaluatorConfig(
            metrics=[
                self._metric("auroc", "torchmetrics.classification.BinaryAUROC"),
                self._metric(
                    "ap",
                    "torchmetrics.classification.BinaryAveragePrecision",
                ),
            ]
        )
        collections, dtype_map = create_metric_collections(config)
        self.assertEqual(len(collections), 1)
        self.assertEqual(len(dtype_map), 1)
        self.assertEqual(len(next(iter(collections.values()))), 2)

    def test_different_dtypes_do_not_co_bundle(self):
        """Different dtypes do not co bundle."""
        config = EvaluatorConfig(
            metrics=[
                self._metric("auroc", "torchmetrics.classification.BinaryAUROC"),
                self._metric("mse", "torchmetrics.regression.MeanSquaredError"),
            ]
        )
        collections, _ = create_metric_collections(config)
        self.assertEqual(len(collections), 2)

    def test_different_filters_do_not_co_bundle(self):
        """Different filters do not co bundle."""
        config = EvaluatorConfig(
            metrics=[
                self._metric("all", "torchmetrics.classification.BinaryAUROC"),
                self._metric(
                    "hi",
                    "torchmetrics.classification.BinaryAUROC",
                    filter_expr="pred > 0.5",
                ),
            ]
        )
        collections, _ = create_metric_collections(config)
        self.assertEqual(len(collections), 2)

    def test_dtype_map_mirrors_the_collection_keys(self):
        """Dtype map mirrors the collection keys."""
        config = EvaluatorConfig(
            metrics=[self._metric("auroc", "torchmetrics.classification.BinaryAUROC")]
        )
        collections, dtype_map = create_metric_collections(config)
        key = next(iter(collections))
        self.assertEqual(dtype_map[key], (key[2], key[3]))

    def test_no_metrics_gives_no_collections(self):
        """No metrics gives no collections."""
        collections, dtype_map = create_metric_collections(EvaluatorConfig())
        self.assertEqual(collections, {})
        self.assertEqual(dtype_map, {})
