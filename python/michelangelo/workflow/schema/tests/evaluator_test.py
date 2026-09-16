"""Tests for michelangelo.workflow.schema.evaluator config dataclasses."""

from __future__ import annotations

from unittest import TestCase

from michelangelo.workflow.schema.evaluator import (
    ColumnMapping,
    CustomEvalConfig,
    DataAnalyticsConfig,
    EvaluationConfig,
    EvaluatorConfig,
    Metric,
    MetricConfig,
    SanityCheck,
    SanityCheckOperator,
    Step,
    StepConfig,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError


def _metric(name: str = "auc") -> MetricConfig:
    return MetricConfig(name=name, metric="torchmetrics.classification.BinaryAUROC")


class ColumnMappingTest(TestCase):
    def test_defaults(self):
        m = ColumnMapping(prediction_col="p", target_col="t")
        self.assertIsNone(m.index_col)
        self.assertIsNone(m.sample_weight_col)
        self.assertIsNone(m.extra_cols)

    def test_equality_and_hash_match_on_all_fields(self):
        a = ColumnMapping(prediction_col="p", target_col="t", extra_cols={"m": "c"})
        b = ColumnMapping(prediction_col="p", target_col="t", extra_cols={"m": "c"})
        self.assertEqual(a, b)
        self.assertEqual(hash(a), hash(b))
        self.assertEqual(len({a, b}), 1)

    def test_extra_cols_order_does_not_change_hash(self):
        a = ColumnMapping(prediction_col="p", target_col="t", extra_cols={"x": "1", "y": "2"})
        b = ColumnMapping(prediction_col="p", target_col="t", extra_cols={"y": "2", "x": "1"})
        self.assertEqual(hash(a), hash(b))

    def test_differing_field_breaks_equality(self):
        a = ColumnMapping(prediction_col="p", target_col="t")
        b = ColumnMapping(prediction_col="p", target_col="other")
        self.assertNotEqual(a, b)

    def test_non_columnmapping_is_never_equal(self):
        self.assertNotEqual(ColumnMapping(prediction_col="p", target_col="t"), "p/t")

    def test_usable_as_dict_key(self):
        m = ColumnMapping(prediction_col="p", target_col="t")
        self.assertEqual({m: "group"}[ColumnMapping(prediction_col="p", target_col="t")], "group")


class StepConfigTest(TestCase):
    def test_evaluation_config_alone_is_valid(self):
        cfg = StepConfig(evaluation_config=EvaluationConfig(labels=["y"]))
        self.assertIsNone(cfg.custom_config)

    def test_custom_config_alone_is_valid(self):
        cfg = StepConfig(custom_config=CustomEvalConfig(args={"k": 1}))
        self.assertIsNone(cfg.evaluation_config)

    def test_neither_raises(self):
        with self.assertRaises(ConfigurationError) as ctx:
            StepConfig()
        self.assertIn("neither", str(ctx.exception))

    def test_both_raises(self):
        with self.assertRaises(ConfigurationError) as ctx:
            StepConfig(
                evaluation_config=EvaluationConfig(),
                custom_config=CustomEvalConfig(),
            )
        self.assertIn("evaluation_config", str(ctx.exception))
        self.assertIn("custom_config", str(ctx.exception))


class StepTest(TestCase):
    def test_minimal_step(self):
        step = Step(processor="pkg.mod.Processor")
        self.assertIsNone(step.name)
        self.assertIsNone(step.run_after)
        self.assertIsNone(step.config)


class MetricTest(TestCase):
    def test_parameters_default_is_independent_per_instance(self):
        a, b = Metric(name="a"), Metric(name="b")
        a.parameters["x"] = 1
        self.assertEqual(b.parameters, {})

    def test_metric_config_defaults(self):
        mc = _metric()
        self.assertEqual(mc.params, {})
        self.assertIsNone(mc.columns)
        self.assertIsNone(mc.filter_expr)


class SanityCheckOperatorTest(TestCase):
    def test_values(self):
        self.assertEqual(SanityCheckOperator.GT.value, ">")
        self.assertEqual(SanityCheckOperator.GTE.value, ">=")
        self.assertEqual(SanityCheckOperator.LT.value, "<")
        self.assertEqual(SanityCheckOperator.LTE.value, "<=")


class SanityCheckTest(TestCase):
    def test_defaults(self):
        c = SanityCheck(metric="auc", threshold=0.55)
        self.assertEqual(c.operator, SanityCheckOperator.GTE)
        self.assertFalse(c.use_abs)
        self.assertEqual(c.datasets, ["test"])

    def test_datasets_default_is_independent_per_instance(self):
        a = SanityCheck(metric="auc", threshold=0.5)
        b = SanityCheck(metric="auc", threshold=0.5)
        a.datasets.append("train")
        self.assertEqual(b.datasets, ["test"])

    def test_string_operator_is_coerced(self):
        self.assertEqual(SanityCheck(metric="m", threshold=1.0, operator="<").operator, SanityCheckOperator.LT)

    def test_whitespace_around_operator_is_tolerated(self):
        self.assertEqual(SanityCheck(metric="m", threshold=1.0, operator=" >= ").operator, SanityCheckOperator.GTE)

    def test_empty_operator_names_the_yaml_trap(self):
        """An unquoted `operator: >` in YAML arrives as an empty string."""
        with self.assertRaises(ConfigurationError) as ctx:
            SanityCheck(metric="m", threshold=1.0, operator="")
        self.assertIn("folded block scalar", str(ctx.exception))

    def test_unknown_operator_lists_valid_choices(self):
        with self.assertRaises(ConfigurationError) as ctx:
            SanityCheck(metric="m", threshold=1.0, operator="!=")
        self.assertIn("'>='", str(ctx.exception))

    def test_non_string_operator_raises(self):
        with self.assertRaises(ConfigurationError):
            SanityCheck(metric="m", threshold=1.0, operator=5)

    def test_use_abs_with_negative_lte_threshold_is_unsatisfiable(self):
        with self.assertRaises(ConfigurationError) as ctx:
            SanityCheck(metric="bias", threshold=-0.1, operator="<=", use_abs=True)
        self.assertIn("unsatisfiable", str(ctx.exception))

    def test_use_abs_with_zero_lt_threshold_is_unsatisfiable(self):
        with self.assertRaises(ConfigurationError):
            SanityCheck(metric="bias", threshold=0.0, operator="<", use_abs=True)

    def test_use_abs_with_zero_lte_threshold_is_allowed(self):
        """|value| <= 0 is satisfiable -- exactly when the metric is 0."""
        self.assertEqual(SanityCheck(metric="bias", threshold=0.0, operator="<=", use_abs=True).threshold, 0.0)

    def test_use_abs_with_positive_threshold_is_allowed(self):
        self.assertTrue(SanityCheck(metric="bias", threshold=0.3, operator="<=", use_abs=True).use_abs)

    def test_negative_threshold_without_use_abs_is_allowed(self):
        self.assertEqual(SanityCheck(metric="bias", threshold=-0.1, operator="<=").threshold, -0.1)

    def test_use_abs_does_not_constrain_gte(self):
        self.assertEqual(SanityCheck(metric="bias", threshold=-1.0, operator=">=", use_abs=True).threshold, -1.0)


class EvaluatorConfigTest(TestCase):
    def test_defaults(self):
        cfg = EvaluatorConfig()
        self.assertEqual(cfg.metrics, [])
        self.assertTrue(cfg.enable_global_segment_evaluation)
        self.assertEqual(cfg.min_segment_row_count, 0)
        self.assertIsNone(cfg.data_preprocessor)

    def test_list_defaults_are_independent_per_instance(self):
        a, b = EvaluatorConfig(), EvaluatorConfig()
        a.segment_columns.append("city_id")
        self.assertEqual(b.segment_columns, [])

    def test_sanity_checks_without_metrics_raises(self):
        with self.assertRaises(ConfigurationError) as ctx:
            EvaluatorConfig(sanity_checks=[SanityCheck(metric="auc", threshold=0.5)])
        self.assertIn("TorchMetrics", str(ctx.exception))

    def test_sanity_check_on_unknown_metric_raises(self):
        with self.assertRaises(ConfigurationError) as ctx:
            EvaluatorConfig(
                metrics=[_metric("auc")],
                sanity_checks=[SanityCheck(metric="typo", threshold=0.5)],
            )
        self.assertIn("typo", str(ctx.exception))

    def test_sanity_check_on_known_metric_is_accepted(self):
        cfg = EvaluatorConfig(
            metrics=[_metric("auc")],
            sanity_checks=[SanityCheck(metric="auc", threshold=0.5)],
        )
        self.assertEqual(len(cfg.sanity_checks), 1)

    def test_segmented_sanity_checks_require_min_segment_row_count(self):
        with self.assertRaises(ConfigurationError) as ctx:
            EvaluatorConfig(
                metrics=[_metric("auc")],
                segment_columns=["city_id"],
                sanity_checks=[SanityCheck(metric="auc", threshold=0.5)],
            )
        self.assertIn("min_segment_row_count", str(ctx.exception))

    def test_segmented_sanity_checks_pass_with_min_segment_row_count(self):
        cfg = EvaluatorConfig(
            metrics=[_metric("auc")],
            segment_columns=["city_id"],
            min_segment_row_count=100,
            sanity_checks=[SanityCheck(metric="auc", threshold=0.5)],
        )
        self.assertEqual(cfg.min_segment_row_count, 100)

    def test_segment_columns_without_sanity_checks_need_no_row_count(self):
        self.assertEqual(EvaluatorConfig(segment_columns=["city_id"]).min_segment_row_count, 0)


class DataAnalyticsConfigTest(TestCase):
    def test_defaults(self):
        cfg = DataAnalyticsConfig()
        self.assertFalse(cfg.enabled)
        self.assertEqual(cfg.stats_sampling_ratio, 1.0)

    def test_zero_sampling_ratio_raises(self):
        """v1 defaulted to 0.0, which silently sampled every split to empty."""
        with self.assertRaises(ConfigurationError) as ctx:
            DataAnalyticsConfig(stats_sampling_ratio=0.0)
        self.assertIn("(0, 1]", str(ctx.exception))

    def test_ratio_above_one_raises(self):
        with self.assertRaises(ConfigurationError):
            DataAnalyticsConfig(stats_sampling_ratio=1.5)

    def test_fractional_ratio_is_allowed(self):
        self.assertEqual(DataAnalyticsConfig(stats_sampling_ratio=0.1).stats_sampling_ratio, 0.1)
