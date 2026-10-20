"""Threshold gating for evaluator metrics.

The evaluator computes metrics and writes reports, but without gating, a model
that trained to garbage still produces a green run and flows on to the pusher. A
``sanity_checks`` entry asserts a bound on a metric the pipeline already
computes; any breach fails the evaluator task.

Every check is applied to every *scope* of every dataset it targets: the
aggregate GLOBAL metrics when they were computed, plus each surviving segment.
One bad segment fails the run, and the message says which.

Segments below ``min_segment_row_count`` are filtered out upstream and never
checked. ``EvaluatorConfig`` requires ``min_segment_row_count > 0`` whenever
both ``sanity_checks`` and ``segment_columns`` are set -- at the default of 0,
filtering is a no-op and a 1-row segment (e.g. single-class AUROC -> NaN) would
be gated exactly like the GLOBAL aggregate, failing a healthy model on its very
first breach.

Everything here is pure: no Ray, no pandas, no I/O. The task module owns
orchestration.
"""

from __future__ import annotations

import logging
import math
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

from michelangelo.lib.exceptions import ConfigurationError
from michelangelo.workflow.schema.evaluator import SanityCheckOperator

if TYPE_CHECKING:
    from collections.abc import Iterable, Mapping

    from michelangelo.workflow.schema.evaluator import EvaluatorConfig, SanityCheck

__all__ = [
    "GLOBAL_SCOPE",
    "FailureLogBudget",
    "SanityCheckResult",
    "format_failures",
    "run_sanity_checks",
    "segment_scope_label",
    "validate_sanity_checks",
    "verify_all_checks_ran",
]

_logger = logging.getLogger(__name__)

# Scope label for the aggregate row, matching the evaluator's own GLOBAL naming.
GLOBAL_SCOPE = "GLOBAL"

# A metric broken globally is usually broken in every segment too, so an
# unbounded failure list can run to thousands of lines. Show enough to diagnose,
# count the rest.
_MAX_FAILURE_LINES = 20


class FailureLogBudget:
    """Caps how many FAIL results get logged at INFO across one evaluator run.

    :func:`format_failures` already caps the raised exception's message at 20
    lines, but without this, :func:`run_sanity_checks` logs every breach at INFO
    as it happens -- a metric broken globally is usually broken in every segment
    too, so a wide segmentation floods the task log long before the exception is
    raised. Share one instance across every :func:`run_sanity_checks` call in a
    run so the cap is global, not per scope.
    """

    def __init__(self, limit: int = _MAX_FAILURE_LINES) -> None:
        """Initialize the budget.

        Args:
            limit: How many failures may be logged at INFO before the rest fall
                back to DEBUG.
        """
        self._remaining = limit

    def consume(self) -> bool:
        """Consume one unit of budget.

        Returns:
            Whether logging at INFO is still allowed.
        """
        if self._remaining <= 0:
            return False
        self._remaining -= 1
        return True


# The enum's value is the comparison symbol, so it doubles as the human-readable
# rendering of the requirement -- no separate symbol table to keep in sync.
_COMPARATORS = {
    SanityCheckOperator.GT: lambda value, threshold: value > threshold,
    SanityCheckOperator.GTE: lambda value, threshold: value >= threshold,
    SanityCheckOperator.LT: lambda value, threshold: value < threshold,
    SanityCheckOperator.LTE: lambda value, threshold: value <= threshold,
}


def segment_scope_label(row: Mapping[str, Any], segment_columns: Iterable[str]) -> str:
    """Render one segment row's identity, e.g. ``datestr=2026-08-30``.

    Deliberately ``name=value`` rather than the bare-value form
    ``chart_segment_labels`` uses for chart keys: a failure message has to be
    readable on its own, and ``dataset[2026-08-30]`` does not say which column
    that was.

    Args:
        row: The segment row, holding its segment columns.
        segment_columns: The columns that define a segment.

    Returns:
        The rendered scope label.
    """
    return ", ".join(f"{column}={row.get(column)}" for column in segment_columns)


@dataclass(frozen=True)
class SanityCheckResult:
    """Outcome of one check against one scope of one dataset.

    Attributes:
        dataset: Dataset the metrics belong to.
        scope: :data:`GLOBAL_SCOPE`, or a label from
            :func:`segment_scope_label`.
        metric: Metric the check targets.
        operator: Comparison the value had to satisfy.
        threshold: Bound the value was compared against.
        value: The compared value, or ``None`` when the metric was absent or not
            a scalar -- such a check always fails.
        use_abs: Whether the absolute value was compared.
        passed: Whether the check held.
        detail: Human-readable rendering of actual vs. required.
    """

    dataset: str
    scope: str
    metric: str
    operator: SanityCheckOperator
    threshold: float
    value: float | None
    use_abs: bool
    passed: bool
    detail: str

    @property
    def location(self) -> str:
        """``dataset[scope]/metric`` -- where the breach is, in one token."""
        return f"{self.dataset}[{self.scope}]/{self.metric}"

    def describe(self) -> str:
        """Render one log- and exception-friendly line.

        Returns:
            The rendered line, prefixed with ``[PASS]`` or ``[FAIL]``.
        """
        status = "PASS" if self.passed else "FAIL"
        return f"[{status}] {self.location}: {self.detail}"


def _coerce_scalar(value: Any) -> float | None:
    """Convert a metric value to a float, or ``None`` if it is not scalar.

    Metric values reach us as Python floats -- the evaluator already calls
    ``.item()`` on single-element tensors -- but numpy scalars and multi-element
    tensors also turn up: a per-class AUROC, for instance, computes to a vector.
    A vector has no meaningful comparison to a scalar threshold, so it is
    reported as unusable rather than reduced. ``bool`` is rejected outright: it
    is an ``int`` subclass and never a metric.

    Args:
        value: The raw metric value.

    Returns:
        The value as a float, or ``None`` when it is not a scalar.
    """
    if isinstance(value, bool) or value is None:
        return None
    if isinstance(value, (int, float)):
        return float(value)
    # numpy scalars and 0-d/1-element tensors expose .item(); larger ones raise.
    item = getattr(value, "item", None)
    if callable(item):
        try:
            return float(item())
        except Exception:
            return None
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def validate_sanity_checks(
    config: EvaluatorConfig,
    metric_names: Iterable[str],
    dataset_names: Iterable[str],
) -> None:
    """Validate sanity-check configuration before any data is read.

    A typo'd metric name should surface in seconds, not after a multi-gigabyte
    evaluation, so this runs as soon as the evaluator and dataset dict are known.

    ``EvaluatorConfig.__post_init__`` already rejects a check naming a metric
    the config does not *declare*, and requires ``min_segment_row_count`` above
    zero before segments are gated. This adds what the schema cannot know: the
    metric names the evaluator will actually *produce*, and which datasets the
    task was handed.

    Args:
        config: The evaluator config to validate.
        metric_names: Metric names the configured metrics will actually produce.
        dataset_names: Datasets the task was handed, before empty ones are
            skipped.

    Raises:
        ConfigurationError: On any configuration error, with the valid options
            listed.
    """
    if not config.sanity_checks:
        return

    known_metrics = set(metric_names)
    known_datasets = set(dataset_names)
    errors: list[str] = []

    for check in config.sanity_checks:
        if check.metric not in known_metrics:
            errors.append(
                f"sanity_check references unknown metric {check.metric!r}. "
                f"Configured metrics: {sorted(known_metrics)}"
            )
        if not check.datasets:
            errors.append(
                f"sanity_check on {check.metric!r} has an empty 'datasets' "
                "list, so it would never run. Name at least one dataset."
            )
        unknown = sorted(set(check.datasets) - known_datasets)
        if unknown:
            errors.append(
                f"sanity_check on {check.metric!r} targets dataset(s) "
                f"{unknown}, which this pipeline does not evaluate. Available "
                f"datasets: {sorted(known_datasets)}"
            )

    if errors:
        raise ConfigurationError(
            "Invalid evaluator sanity_checks:\n  - " + "\n  - ".join(errors)
        )


def run_sanity_checks(
    dataset_name: str,
    scope: str,
    metrics: Mapping[str, Any],
    checks: Iterable[SanityCheck],
    log_budget: FailureLogBudget | None = None,
) -> list[SanityCheckResult]:
    """Evaluate every check targeting ``dataset_name`` against one scope.

    A metric that is missing, non-scalar, or non-finite (NaN/inf) counts as a
    *failure* rather than a skip -- those are precisely the degenerate-model
    cases this feature exists to catch, and treating them as "no opinion" would
    let them through.

    Args:
        dataset_name: Dataset these metrics belong to, e.g. ``"test"``.
        scope: :data:`GLOBAL_SCOPE`, or a label from
            :func:`segment_scope_label`.
        metrics: Metric name -> value for this scope. Segment rows carry their
            segment columns here too; extra keys are ignored.
        checks: All configured checks; those not targeting this dataset are
            ignored.
        log_budget: Shared across every scope and dataset in a run to cap how
            many FAILs are logged at INFO. ``None`` logs every failure at INFO,
            which suits tests that check individual scopes in isolation.

    Returns:
        One result per applicable check, in configuration order.
    """
    results: list[SanityCheckResult] = []

    for check in checks:
        if dataset_name not in check.datasets:
            continue

        operator = SanityCheckOperator(check.operator)
        compare = _COMPARATORS[operator]
        subject = "|value|" if check.use_abs else "value"
        requirement = f"required {subject} {operator.value} {check.threshold:g}"

        # check/operator are bound as defaults: the closure is only ever called
        # within this iteration, but leaving them free would be a late-binding
        # bug in waiting.
        def build_result(
            passed: bool,
            value: float | None,
            detail: str,
            *,
            check: SanityCheck = check,
            operator: SanityCheckOperator = operator,
        ) -> SanityCheckResult:
            return SanityCheckResult(
                dataset=dataset_name,
                scope=scope,
                metric=check.metric,
                operator=operator,
                threshold=check.threshold,
                value=value,
                use_abs=check.use_abs,
                passed=passed,
                detail=detail,
            )

        if check.metric not in metrics:
            # Only the metric names are listed: a segment row also carries its
            # segment columns, and dumping those adds noise without helping.
            result = build_result(
                False,
                None,
                f"metric was not computed for this scope ({requirement})",
            )
        else:
            raw = _coerce_scalar(metrics[check.metric])
            if raw is None:
                result = build_result(
                    False,
                    None,
                    f"actual {metrics[check.metric]!r} is not a scalar, so it "
                    f"cannot be compared ({requirement})",
                )
            else:
                value = abs(raw) if check.use_abs else raw
                if not math.isfinite(value):
                    result = build_result(
                        False, value, f"actual {value}, {requirement}"
                    )
                else:
                    rendered = (
                        f"|{raw:.6g}| = {value:.6g}"
                        if check.use_abs
                        else f"{value:.6g}"
                    )
                    result = build_result(
                        compare(value, check.threshold),
                        value,
                        f"actual {rendered}, {requirement}",
                    )

        # Passes are DEBUG: a wide segmentation multiplies every check by the
        # segment count, and a few thousand PASS lines bury the handful that
        # matter. Failures are INFO up to log_budget, then DEBUG too --
        # format_failures's capped message is still the authoritative list.
        if result.passed:
            _logger.debug("[SANITY] %s", result.describe())
        elif log_budget is None or log_budget.consume():
            _logger.info("[SANITY] %s", result.describe())
        else:
            _logger.debug("[SANITY] %s", result.describe())
        results.append(result)

    return results


def verify_all_checks_ran(
    checks: Iterable[SanityCheck],
    evaluated_datasets: Iterable[str],
) -> None:
    """Raise if a configured check never got a chance to run.

    :func:`validate_sanity_checks` confirms the target dataset was *handed* to
    the task, but an optional split can still be skipped at runtime when it
    loads empty. A check whose dataset vanished that way would silently gate
    nothing, leaving the author believing they are protected.

    Args:
        checks: All configured checks.
        evaluated_datasets: Datasets that actually produced metrics.

    Raises:
        ConfigurationError: Naming the checks that never ran.
    """
    evaluated = set(evaluated_datasets)
    orphaned = [
        f"{check.metric!r} (targets {sorted(check.datasets)})"
        for check in checks
        if not set(check.datasets) & evaluated
    ]
    if orphaned:
        raise ConfigurationError(
            "Evaluator sanity_checks did not run because their target datasets "
            f"were not evaluated (evaluated: {sorted(evaluated)}): "
            + "; ".join(orphaned)
        )


def format_failures(results: Iterable[SanityCheckResult]) -> str:
    """Render failed results as a multi-line message naming every breach.

    Each line names the dataset, the scope (GLOBAL or the segment), the metric,
    the actual value, and the threshold it had to clear, so the failure is
    actionable straight from the task log without opening the evaluation report.

    The list is capped: a metric broken globally is typically broken in every
    segment, and an unbounded dump would push the useful lines out of view.

    Args:
        results: Every result from the run, passed and failed alike.

    Returns:
        The rendered failure message.
    """
    results = list(results)
    failures = [result for result in results if not result.passed]
    shown = failures[:_MAX_FAILURE_LINES]

    lines = "\n  - ".join(f"{result.location}: {result.detail}" for result in shown)
    message = (
        f"Evaluator sanity checks failed: {len(failures)} of {len(results)} "
        f"check(s) breached.\n  - {lines}"
    )

    hidden = len(failures) - len(shown)
    if hidden:
        breached_metrics = sorted({result.metric for result in failures})
        message += (
            f"\n  ... and {hidden} more breach(es) not shown. Metrics breached "
            f"overall: {breached_metrics}"
        )
    return message
