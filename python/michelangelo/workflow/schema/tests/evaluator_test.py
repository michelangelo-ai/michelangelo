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
    """Tests for ColumnMapping."""

    def test_defaults(self):
        """It defaults extra_cols to an empty dict."""
        m = ColumnMapping(prediction_col="p", target_col="t")
        self.assertIsNone(m.index_col)
        self.assertIsNone(m.sample_weight_col)
        self.assertIsNone(m.extra_cols)

    def test_equality_and_hash_match_on_all_fields(self):
        """It hashes and compares equal when every field matches."""
        a = ColumnMapping(prediction_col="p", target_col="t", extra_cols={"m": "c"})
        b = ColumnMapping(prediction_col="p", target_col="t", extra_cols={"m": "c"})
        self.assertEqual(a, b)
        self.assertEqual(hash(a), hash(b))
        self.assertEqual(len({a, b}), 1)

    def test_extra_cols_order_does_not_change_hash(self):
        """It ignores extra_cols insertion order when hashing."""
        a = ColumnMapping(
            prediction_col="p", target_col="t", extra_cols={"x": "1", "y": "2"}
        )
        b = ColumnMapping(
            prediction_col="p", target_col="t", extra_cols={"y": "2", "x": "1"}
        )
        self.assertEqual(hash(a), hash(b))

    def test_differing_field_breaks_equality(self):
        """It compares unequal when any field differs."""
        a = ColumnMapping(prediction_col="p", target_col="t")
        b = ColumnMapping(prediction_col="p", target_col="other")
        self.assertNotEqual(a, b)

    def test_non_columnmapping_is_never_equal(self):
        """It never compares equal to a non-ColumnMapping."""
        self.assertNotEqual(ColumnMapping(prediction_col="p", target_col="t"), "p/t")

    def test_usable_as_dict_key(self):
        """It can be used as a dict key."""
        m = ColumnMapping(prediction_col="p", target_col="t")
        self.assertEqual(
            {m: "group"}[ColumnMapping(prediction_col="p", target_col="t")], "group"
        )


class StepConfigTest(TestCase):
    """Tests for StepConfig's one-of validation."""

    def test_evaluation_config_alone_is_valid(self):
        """It accepts a builtin-metric config on its own."""
        cfg = StepConfig(evaluation_config=EvaluationConfig(labels=["y"]))
        self.assertIsNone(cfg.custom_config)

    def test_custom_config_alone_is_valid(self):
        """It accepts a custom-eval config on its own."""
        cfg = StepConfig(custom_config=CustomEvalConfig(args={"k": 1}))
        self.assertIsNone(cfg.evaluation_config)

    def test_neither_raises(self):
        """It rejects a config that sets neither side of the one-of."""
        with self.assertRaises(ConfigurationError) as ctx:
            StepConfig()
        self.assertIn("neither", str(ctx.exception))

    def test_both_raises(self):
        """It rejects a config that sets both sides of the one-of."""
        with self.assertRaises(ConfigurationError) as ctx:
            StepConfig(
                evaluation_config=EvaluationConfig(),
                custom_config=CustomEvalConfig(),
            )
        self.assertIn("evaluation_config", str(ctx.exception))
        self.assertIn("custom_config", str(ctx.exception))


class StepTest(TestCase):
    """Tests for Step."""

    def test_minimal_step(self):
        """It builds from a name and a single step config."""
        step = Step(processor="pkg.mod.Processor")
        self.assertIsNone(step.name)
        self.assertIsNone(step.run_after)
        self.assertIsNone(step.config)


class MetricTest(TestCase):
    """Tests for Metric and MetricConfig."""

    def test_parameters_default_is_independent_per_instance(self):
        """It gives each instance its own parameters dict."""
        a, b = Metric(name="a"), Metric(name="b")
        a.parameters["x"] = 1
        self.assertEqual(b.parameters, {})

    def test_metric_config_defaults(self):
        """It defaults MetricConfig to an empty metric list."""
        mc = _metric()
        self.assertEqual(mc.params, {})
        self.assertIsNone(mc.columns)
        self.assertIsNone(mc.filter_expr)


class SanityCheckOperatorTest(TestCase):
    """Tests for the SanityCheckOperator enum."""

    def test_values(self):
        """It exposes the six comparison operators as their symbols."""
        self.assertEqual(SanityCheckOperator.GT.value, ">")
        self.assertEqual(SanityCheckOperator.GTE.value, ">=")
        self.assertEqual(SanityCheckOperator.LT.value, "<")
        self.assertEqual(SanityCheckOperator.LTE.value, "<=")


class SanityCheckTest(TestCase):
    """Tests for SanityCheck validation."""

    def test_defaults(self):
        """It defaults to >=, use_abs off, and the test dataset."""
        c = SanityCheck(metric="auc", threshold=0.55)
        self.assertEqual(c.operator, SanityCheckOperator.GTE)
        self.assertFalse(c.use_abs)
        self.assertEqual(c.datasets, ["test"])

    def test_datasets_default_is_independent_per_instance(self):
        """It gives each instance its own datasets list."""
        a = SanityCheck(metric="auc", threshold=0.5)
        b = SanityCheck(metric="auc", threshold=0.5)
        a.datasets.append("train")
        self.assertEqual(b.datasets, ["test"])

    def test_string_operator_is_coerced(self):
        """It coerces a plain operator string to the enum."""
        self.assertEqual(
            SanityCheck(metric="m", threshold=1.0, operator="<").operator,
            SanityCheckOperator.LT,
        )

    def test_whitespace_around_operator_is_tolerated(self):
        """It strips whitespace around an operator string."""
        self.assertEqual(
            SanityCheck(metric="m", threshold=1.0, operator=" >= ").operator,
            SanityCheckOperator.GTE,
        )

    def test_empty_operator_names_the_yaml_trap(self):
        """An unquoted `operator: >` in YAML arrives as an empty string."""
        with self.assertRaises(ConfigurationError) as ctx:
            SanityCheck(metric="m", threshold=1.0, operator="")
        self.assertIn("folded block scalar", str(ctx.exception))

    def test_unknown_operator_lists_valid_choices(self):
        """It names the valid operators when given an unknown one."""
        with self.assertRaises(ConfigurationError) as ctx:
            SanityCheck(metric="m", threshold=1.0, operator="!=")
        self.assertIn("'>='", str(ctx.exception))

    def test_non_string_operator_raises(self):
        """It rejects an operator that is neither str nor enum."""
        with self.assertRaises(ConfigurationError):
            SanityCheck(metric="m", threshold=1.0, operator=5)

    def test_use_abs_with_negative_lte_threshold_is_unsatisfiable(self):
        """It rejects |value| <= a negative threshold."""
        with self.assertRaises(ConfigurationError) as ctx:
            SanityCheck(metric="bias", threshold=-0.1, operator="<=", use_abs=True)
        self.assertIn("unsatisfiable", str(ctx.exception))

    def test_use_abs_with_zero_lt_threshold_is_unsatisfiable(self):
        """It rejects |value| < 0."""
        with self.assertRaises(ConfigurationError):
            SanityCheck(metric="bias", threshold=0.0, operator="<", use_abs=True)

    def test_use_abs_with_zero_lte_threshold_is_allowed(self):
        """|value| <= 0 is satisfiable -- exactly when the metric is 0."""
        self.assertEqual(
            SanityCheck(
                metric="bias", threshold=0.0, operator="<=", use_abs=True
            ).threshold,
            0.0,
        )

    def test_use_abs_with_positive_threshold_is_allowed(self):
        """It accepts |value| <= a positive threshold."""
        self.assertTrue(
            SanityCheck(
                metric="bias", threshold=0.3, operator="<=", use_abs=True
            ).use_abs
        )

    def test_negative_threshold_without_use_abs_is_allowed(self):
        """It leaves a negative threshold alone when use_abs is off."""
        self.assertEqual(
            SanityCheck(metric="bias", threshold=-0.1, operator="<=").threshold, -0.1
        )

    def test_use_abs_does_not_constrain_gte(self):
        """It applies the unsatisfiability check only to < and <=."""
        self.assertEqual(
            SanityCheck(
                metric="bias", threshold=-1.0, operator=">=", use_abs=True
            ).threshold,
            -1.0,
        )


class EvaluatorConfigTest(TestCase):
    """Tests for EvaluatorConfig validation."""

    def test_defaults(self):
        """It defaults every collection field to empty."""
        cfg = EvaluatorConfig()
        self.assertEqual(cfg.metrics, [])
        self.assertTrue(cfg.enable_global_segment_evaluation)
        self.assertEqual(cfg.min_segment_row_count, 0)
        self.assertIsNone(cfg.data_preprocessor)

    def test_list_defaults_are_independent_per_instance(self):
        """It gives each instance its own list fields."""
        a, b = EvaluatorConfig(), EvaluatorConfig()
        a.segment_columns.append("city_id")
        self.assertEqual(b.segment_columns, [])

    def test_sanity_checks_without_metrics_raises(self):
        """It rejects sanity checks when no metric is configured."""
        with self.assertRaises(ConfigurationError) as ctx:
            EvaluatorConfig(sanity_checks=[SanityCheck(metric="auc", threshold=0.5)])
        self.assertIn("TorchMetrics", str(ctx.exception))

    def test_sanity_check_on_unknown_metric_raises(self):
        """It rejects a sanity check naming an unconfigured metric."""
        with self.assertRaises(ConfigurationError) as ctx:
            EvaluatorConfig(
                metrics=[_metric("auc")],
                sanity_checks=[SanityCheck(metric="typo", threshold=0.5)],
            )
        self.assertIn("typo", str(ctx.exception))

    def test_sanity_check_on_known_metric_is_accepted(self):
        """It accepts a sanity check naming a configured metric."""
        cfg = EvaluatorConfig(
            metrics=[_metric("auc")],
            sanity_checks=[SanityCheck(metric="auc", threshold=0.5)],
        )
        self.assertEqual(len(cfg.sanity_checks), 1)

    def test_segmented_sanity_checks_require_min_segment_row_count(self):
        """It rejects segmented checks with a zero min_segment_row_count."""
        with self.assertRaises(ConfigurationError) as ctx:
            EvaluatorConfig(
                metrics=[_metric("auc")],
                segment_columns=["city_id"],
                sanity_checks=[SanityCheck(metric="auc", threshold=0.5)],
            )
        self.assertIn("min_segment_row_count", str(ctx.exception))

    def test_segmented_sanity_checks_pass_with_min_segment_row_count(self):
        """It accepts segmented checks with a positive min_segment_row_count."""
        cfg = EvaluatorConfig(
            metrics=[_metric("auc")],
            segment_columns=["city_id"],
            min_segment_row_count=100,
            sanity_checks=[SanityCheck(metric="auc", threshold=0.5)],
        )
        self.assertEqual(cfg.min_segment_row_count, 100)

    def test_segment_columns_without_sanity_checks_need_no_row_count(self):
        """It leaves min_segment_row_count at zero when no check is segmented."""
        self.assertEqual(
            EvaluatorConfig(segment_columns=["city_id"]).min_segment_row_count, 0
        )


class DataAnalyticsConfigTest(TestCase):
    """Tests for DataAnalyticsConfig validation."""

    def test_defaults(self):
        """It defaults to disabled with a full sampling ratio."""
        cfg = DataAnalyticsConfig()
        self.assertFalse(cfg.enabled)
        self.assertEqual(cfg.stats_sampling_ratio, 1.0)

    def test_zero_sampling_ratio_raises(self):
        """v1 defaulted to 0.0, which silently sampled every split to empty."""
        with self.assertRaises(ConfigurationError) as ctx:
            DataAnalyticsConfig(stats_sampling_ratio=0.0)
        self.assertIn("(0, 1]", str(ctx.exception))

    def test_ratio_above_one_raises(self):
        """It rejects a stats_sampling_ratio above one."""
        with self.assertRaises(ConfigurationError):
            DataAnalyticsConfig(stats_sampling_ratio=1.5)

    def test_fractional_ratio_is_allowed(self):
        """It accepts a fractional stats_sampling_ratio."""
        self.assertEqual(
            DataAnalyticsConfig(stats_sampling_ratio=0.1).stats_sampling_ratio, 0.1
        )
