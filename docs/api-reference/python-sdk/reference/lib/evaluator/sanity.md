---
sidebar_label: sanity
title: lib.evaluator.sanity
---

Threshold gating for evaluator metrics.

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

## FailureLogBudget Objects

```python
class FailureLogBudget()
```

Caps how many FAIL results get logged at INFO across one evaluator run.

`format_failures` already caps the raised exception's message at 20
lines, but without this, `run_sanity_checks` logs every breach at INFO
as it happens -- a metric broken globally is usually broken in every segment
too, so a wide segmentation floods the task log long before the exception is
raised. Share one instance across every `run_sanity_checks` call in a
run so the cap is global, not per scope.

#### \_\_init\_\_

```python
def __init__(limit: int = _MAX_FAILURE_LINES) -> None
```

Initialize the budget.

**Arguments**:

- `limit` - How many failures may be logged at INFO before the rest fall
  back to DEBUG.

#### consume

```python
def consume() -> bool
```

Consume one unit of budget.

**Returns**:

  Whether logging at INFO is still allowed.

#### segment\_scope\_label

```python
def segment_scope_label(row: Mapping[str, Any],
                        segment_columns: Iterable[str]) -> str
```

Render one segment row's identity, e.g. ``datestr=2026-08-30``.

Deliberately ``name=value`` rather than the bare-value form
``chart_segment_labels`` uses for chart keys: a failure message has to be
readable on its own, and ``dataset[2026-08-30]`` does not say which column
that was.

**Arguments**:

- `row` - The segment row, holding its segment columns.
- `segment_columns` - The columns that define a segment.
  

**Returns**:

  The rendered scope label.

## SanityCheckResult Objects

```python
@dataclass(frozen=True)
class SanityCheckResult()
```

Outcome of one check against one scope of one dataset.

**Attributes**:

- `dataset` - Dataset the metrics belong to.
- `scope` - :data:`GLOBAL_SCOPE`, or a label from
  `segment_scope_label`.
- `metric` - Metric the check targets.
- `operator` - Comparison the value had to satisfy.
- `threshold` - Bound the value was compared against.
- `value` - The compared value, or ``None`` when the metric was absent or not
  a scalar -- such a check always fails.
- `use_abs` - Whether the absolute value was compared.
- `passed` - Whether the check held.
- `detail` - Human-readable rendering of actual vs. required.

#### location

```python
@property
def location() -> str
```

``dataset[scope]/metric`` -- where the breach is, in one token.

#### describe

```python
def describe() -> str
```

Render one log- and exception-friendly line.

**Returns**:

  The rendered line, prefixed with ``[PASS]`` or ``[FAIL]``.

#### validate\_sanity\_checks

```python
def validate_sanity_checks(config: EvaluatorConfig,
                           metric_names: Iterable[str],
                           dataset_names: Iterable[str]) -> None
```

Validate sanity-check configuration before any data is read.

A typo'd metric name should surface in seconds, not after a multi-gigabyte
evaluation, so this runs as soon as the evaluator and dataset dict are known.

``EvaluatorConfig.__post_init__`` already rejects a check naming a metric
the config does not *declare*, and requires ``min_segment_row_count`` above
zero before segments are gated. This adds what the schema cannot know: the
metric names the evaluator will actually *produce*, and which datasets the
task was handed.

**Arguments**:

- `config` - The evaluator config to validate.
- `metric_names` - Metric names the configured metrics will actually produce.
- `dataset_names` - Datasets the task was handed, before empty ones are
  skipped.
  

**Raises**:

- `ConfigurationError` - On any configuration error, with the valid options
  listed.

#### run\_sanity\_checks

```python
def run_sanity_checks(
        dataset_name: str,
        scope: str,
        metrics: Mapping[str, Any],
        checks: Iterable[SanityCheck],
        log_budget: FailureLogBudget | None = None) -> list[SanityCheckResult]
```

Evaluate every check targeting ``dataset_name`` against one scope.

A metric that is missing, non-scalar, or non-finite (NaN/inf) counts as a
*failure* rather than a skip -- those are precisely the degenerate-model
cases this feature exists to catch, and treating them as "no opinion" would
let them through.

**Arguments**:

- `dataset_name` - Dataset these metrics belong to, e.g. ``"test"``.
- `scope` - :data:`GLOBAL_SCOPE`, or a label from
  `segment_scope_label`.
- `metrics` - Metric name -> value for this scope. Segment rows carry their
  segment columns here too; extra keys are ignored.
- `checks` - All configured checks; those not targeting this dataset are
  ignored.
- `log_budget` - Shared across every scope and dataset in a run to cap how
  many FAILs are logged at INFO. ``None`` logs every failure at INFO,
  which suits tests that check individual scopes in isolation.
  

**Returns**:

  One result per applicable check, in configuration order.

#### verify\_all\_checks\_ran

```python
def verify_all_checks_ran(checks: Iterable[SanityCheck],
                          evaluated_datasets: Iterable[str]) -> None
```

Raise if a configured check never got a chance to run.

`validate_sanity_checks` confirms the target dataset was *handed* to
the task, but an optional split can still be skipped at runtime when it
loads empty. A check whose dataset vanished that way would silently gate
nothing, leaving the author believing they are protected.

**Arguments**:

- `checks` - All configured checks.
- `evaluated_datasets` - Datasets that actually produced metrics.
  

**Raises**:

- `ConfigurationError` - Naming the checks that never ran.

#### format\_failures

```python
def format_failures(results: Iterable[SanityCheckResult]) -> str
```

Render failed results as a multi-line message naming every breach.

Each line names the dataset, the scope (GLOBAL or the segment), the metric,
the actual value, and the threshold it had to clear, so the failure is
actionable straight from the task log without opening the evaluation report.

The list is capped: a metric broken globally is typically broken in every
segment, and an unbounded dump would push the useful lines out of view.

**Arguments**:

- `results` - Every result from the run, passed and failed alike.
  

**Returns**:

  The rendered failure message.

