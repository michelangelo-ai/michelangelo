"""Tests for michelangelo.lib.evaluator.sanity."""

from __future__ import annotations

from unittest import TestCase

from michelangelo.lib.evaluator.sanity import (
    GLOBAL_SCOPE,
    FailureLogBudget,
    SanityCheckResult,
    format_failures,
    run_sanity_checks,
    segment_scope_label,
    validate_sanity_checks,
    verify_all_checks_ran,
)
from michelangelo.lib.exceptions import ConfigurationError
from michelangelo.workflow.schema.evaluator import (
    ColumnMapping,
    EvaluatorConfig,
    MetricConfig,
    SanityCheck,
    SanityCheckOperator,
)


def _check(
    metric: str = "auroc",
    operator: SanityCheckOperator = SanityCheckOperator.GTE,
    threshold: float = 0.7,
    datasets: tuple[str, ...] = ("test",),
    use_abs: bool = False,
) -> SanityCheck:
    """Build a SanityCheck with sensible defaults."""
    return SanityCheck(
        metric=metric,
        operator=operator,
        threshold=threshold,
        datasets=list(datasets),
        use_abs=use_abs,
    )


def _config(**kwargs) -> EvaluatorConfig:
    """Build an EvaluatorConfig with one metric and the given overrides."""
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


class SegmentScopeLabelTest(TestCase):
    """segment_scope_label renders name=value pairs."""

    def test_single_column(self):
        """Single column."""
        label = segment_scope_label({"city": "SF"}, ["city"])
        self.assertEqual(label, "city=SF")

    def test_multiple_columns_are_comma_joined(self):
        """Multiple columns are comma joined."""
        label = segment_scope_label({"city": "SF", "day": "mon"}, ["city", "day"])
        self.assertEqual(label, "city=SF, day=mon")

    def test_missing_column_renders_none(self):
        """Missing column renders none."""
        self.assertEqual(segment_scope_label({}, ["city"]), "city=None")


class ValidateSanityChecksTest(TestCase):
    """validate_sanity_checks rejects unusable configuration up front."""

    def test_no_checks_is_a_noop(self):
        """No checks is a noop."""
        validate_sanity_checks(_config(), [], [])

    def test_accepts_a_valid_check(self):
        """Accepts a valid check."""
        config = _config(sanity_checks=[_check()])
        validate_sanity_checks(config, ["auroc"], ["test"])

    def test_rejects_metric_the_evaluator_will_not_produce(self):
        """Rejects metric the evaluator will not produce."""
        # The schema rejects a check naming an undeclared metric, so the case
        # left for this function is a declared metric the evaluator does not
        # actually emit.
        config = _config(sanity_checks=[_check()])
        with self.assertRaises(ConfigurationError) as ctx:
            validate_sanity_checks(config, [], ["test"])
        self.assertIn("unknown metric 'auroc'", str(ctx.exception))

    def test_rejects_empty_dataset_list(self):
        """Rejects empty dataset list."""
        config = _config(sanity_checks=[_check(datasets=())])
        with self.assertRaises(ConfigurationError) as ctx:
            validate_sanity_checks(config, ["auroc"], ["test"])
        self.assertIn("empty 'datasets' list", str(ctx.exception))

    def test_rejects_unknown_dataset(self):
        """Rejects unknown dataset."""
        config = _config(sanity_checks=[_check(datasets=("holdout",))])
        with self.assertRaises(ConfigurationError) as ctx:
            validate_sanity_checks(config, ["auroc"], ["test"])
        self.assertIn("['holdout']", str(ctx.exception))

    def test_accepts_segments_with_a_row_floor(self):
        """Accepts segments with a row floor."""
        config = _config(
            sanity_checks=[_check()],
            segment_columns=["city"],
            min_segment_row_count=100,
        )
        validate_sanity_checks(config, ["auroc"], ["test"])

    def test_reports_every_error_at_once(self):
        """Reports every error at once."""
        config = _config(sanity_checks=[_check(datasets=("holdout",))])
        with self.assertRaises(ConfigurationError) as ctx:
            validate_sanity_checks(config, [], ["test"])
        message = str(ctx.exception)
        self.assertIn("unknown metric", message)
        self.assertIn("holdout", message)


class RunSanityChecksTest(TestCase):
    """run_sanity_checks compares one scope's metrics against the checks."""

    def test_passing_check(self):
        """Passing check."""
        results = run_sanity_checks("test", GLOBAL_SCOPE, {"auroc": 0.9}, [_check()])
        self.assertEqual(len(results), 1)
        self.assertTrue(results[0].passed)
        self.assertEqual(results[0].value, 0.9)

    def test_failing_check(self):
        """Failing check."""
        results = run_sanity_checks("test", GLOBAL_SCOPE, {"auroc": 0.5}, [_check()])
        self.assertFalse(results[0].passed)
        self.assertIn("required value >= 0.7", results[0].detail)

    def test_checks_for_other_datasets_are_skipped(self):
        """Checks for other datasets are skipped."""
        results = run_sanity_checks("train", GLOBAL_SCOPE, {"auroc": 0.1}, [_check()])
        self.assertEqual(results, [])

    def test_missing_metric_fails(self):
        """Missing metric fails."""
        results = run_sanity_checks("test", GLOBAL_SCOPE, {}, [_check()])
        self.assertFalse(results[0].passed)
        self.assertIsNone(results[0].value)
        self.assertIn("was not computed", results[0].detail)

    def test_non_scalar_metric_fails(self):
        """Non scalar metric fails."""
        results = run_sanity_checks(
            "test", GLOBAL_SCOPE, {"auroc": [0.9, 0.8]}, [_check()]
        )
        self.assertFalse(results[0].passed)
        self.assertIsNone(results[0].value)
        self.assertIn("is not a scalar", results[0].detail)

    def test_nan_fails_rather_than_skipping(self):
        """Nan fails rather than skipping."""
        results = run_sanity_checks(
            "test", GLOBAL_SCOPE, {"auroc": float("nan")}, [_check()]
        )
        self.assertFalse(results[0].passed)

    def test_bool_is_not_a_metric_value(self):
        """Bool is not a metric value."""
        results = run_sanity_checks("test", GLOBAL_SCOPE, {"auroc": True}, [_check()])
        self.assertFalse(results[0].passed)
        self.assertIn("is not a scalar", results[0].detail)

    def test_use_abs_compares_magnitude(self):
        """Use abs compares magnitude."""
        check = _check(
            metric="bias", operator=SanityCheckOperator.LT, threshold=0.1, use_abs=True
        )
        failing = run_sanity_checks("test", GLOBAL_SCOPE, {"bias": -0.5}, [check])
        passing = run_sanity_checks("test", GLOBAL_SCOPE, {"bias": -0.05}, [check])
        self.assertFalse(failing[0].passed)
        self.assertTrue(passing[0].passed)
        self.assertIn("|-0.5| = 0.5", failing[0].detail)

    def test_every_operator_is_supported(self):
        """Every operator is supported."""
        cases = [
            (SanityCheckOperator.GT, 0.7, 0.7, False),
            (SanityCheckOperator.GTE, 0.7, 0.7, True),
            (SanityCheckOperator.LT, 0.7, 0.7, False),
            (SanityCheckOperator.LTE, 0.7, 0.7, True),
        ]
        for operator, threshold, value, expected in cases:
            with self.subTest(operator=operator):
                results = run_sanity_checks(
                    "test",
                    GLOBAL_SCOPE,
                    {"auroc": value},
                    [_check(operator=operator, threshold=threshold)],
                )
                self.assertEqual(results[0].passed, expected)

    def test_results_are_in_configuration_order(self):
        """Results are in configuration order."""
        checks = [_check(metric="a"), _check(metric="b")]
        results = run_sanity_checks("test", GLOBAL_SCOPE, {"a": 1.0, "b": 1.0}, checks)
        self.assertEqual([r.metric for r in results], ["a", "b"])


class FailureLogBudgetTest(TestCase):
    """FailureLogBudget caps INFO logging across a whole run."""

    def test_budget_is_consumed_then_exhausted(self):
        """Budget is consumed then exhausted."""
        budget = FailureLogBudget(limit=2)
        self.assertTrue(budget.consume())
        self.assertTrue(budget.consume())
        self.assertFalse(budget.consume())

    def test_exhausted_budget_does_not_change_results(self):
        """Exhausted budget does not change results."""
        budget = FailureLogBudget(limit=0)
        results = run_sanity_checks(
            "test", GLOBAL_SCOPE, {"auroc": 0.1}, [_check()], log_budget=budget
        )
        self.assertFalse(results[0].passed)


class VerifyAllChecksRanTest(TestCase):
    """verify_all_checks_ran catches checks whose dataset never appeared."""

    def test_passes_when_every_dataset_was_evaluated(self):
        """Passes when every dataset was evaluated."""
        verify_all_checks_ran([_check()], ["test", "train"])

    def test_raises_for_an_orphaned_check(self):
        """Raises for an orphaned check."""
        with self.assertRaises(ConfigurationError) as ctx:
            verify_all_checks_ran([_check()], ["train"])
        message = str(ctx.exception)
        self.assertIn("'auroc'", message)
        self.assertIn("['train']", message)

    def test_a_check_needs_only_one_of_its_datasets(self):
        """A check needs only one of its datasets."""
        verify_all_checks_ran([_check(datasets=("test", "holdout"))], ["holdout"])


class FormatFailuresTest(TestCase):
    """format_failures renders a capped, actionable failure message."""

    @staticmethod
    def _result(passed: bool, metric: str = "auroc") -> SanityCheckResult:
        return SanityCheckResult(
            dataset="test",
            scope=GLOBAL_SCOPE,
            metric=metric,
            operator=SanityCheckOperator.GTE,
            threshold=0.7,
            value=0.5,
            use_abs=False,
            passed=passed,
            detail="actual 0.5, required value >= 0.7",
        )

    def test_names_the_breach(self):
        """Names the breach."""
        message = format_failures([self._result(False), self._result(True)])
        self.assertIn("1 of 2 check(s) breached", message)
        self.assertIn("test[GLOBAL]/auroc", message)

    def test_passes_are_not_listed(self):
        """Passes are not listed."""
        message = format_failures([self._result(True)])
        self.assertIn("0 of 1 check(s) breached", message)
        self.assertNotIn("test[GLOBAL]/auroc", message)

    def test_long_lists_are_capped_and_summarized(self):
        """Long lists are capped and summarized."""
        results = [self._result(False, metric=f"m{i}") for i in range(25)]
        message = format_failures(results)
        self.assertIn("25 of 25 check(s) breached", message)
        self.assertIn("... and 5 more breach(es) not shown", message)
        self.assertIn("'m24'", message)
        self.assertNotIn("test[GLOBAL]/m24:", message)

    def test_result_describe_and_location(self):
        """Result describe and location."""
        result = self._result(False)
        self.assertEqual(result.location, "test[GLOBAL]/auroc")
        self.assertTrue(result.describe().startswith("[FAIL] test[GLOBAL]/auroc"))
        self.assertTrue(self._result(True).describe().startswith("[PASS]"))
