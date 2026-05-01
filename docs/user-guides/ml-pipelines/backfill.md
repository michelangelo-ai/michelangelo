# Backfill Pipelines

A backfill lets you execute a Uniflow workflow as if it had been triggered at a specific point in the past. This is useful for reprocessing data for past date windows — for example, re-running a training pipeline on last quarter's data after fixing a bug.

Backfill works by setting the `STARLARK_TIME` environment variable before execution. When `STARLARK_TIME` is set, Uniflow's time utilities return the specified historical timestamp instead of the system clock.

## How STARLARK_TIME works

Only time calculations that use Uniflow's time utilities are affected. Standard Python time functions (`time.time()`, `datetime.now()`) continue using the real system clock and will not be influenced by `STARLARK_TIME`.

This is intentional — it ensures workflow determinism for backfills, but requires you to use Uniflow's time utilities in any time-based workflow logic.

## Writing a backfill-friendly workflow

The key pattern is to compute timestamps in the **workflow** using Uniflow's `time()` function, then **pass them as parameters** to your tasks. Tasks should never call `time.time()` or `datetime.now()` directly.

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.spark import SparkTask
from michelangelo.uniflow.core.lib.time import time as uniflow_time

@uniflow.task(config=SparkTask())
def load_data(start_ts: float, end_ts: float):
    # Use start_ts and end_ts to load the data.
    # Do not call time.time() or datetime.now() here.
    ...

@uniflow.workflow()
def my_workflow(start_days_offset: float, end_days_offset: float):
    # uniflow_time() respects STARLARK_TIME when set.
    ts = uniflow_time()

    start_ts = ts - start_days_offset * 60 * 60 * 24
    end_ts = ts - end_days_offset * 60 * 60 * 24

    load_data(start_ts, end_ts)

if __name__ == "__main__":
    ctx = uniflow.create_context()
    ctx.environ.update({"MA_NAMESPACE": "my-project"})
    # Run the workflow for the past 5-day window.
    ctx.run(my_workflow, start_days_offset=5, end_days_offset=1)
```

:::tip Keep tasks backfill-friendly
Avoid standard Python time functions inside `@uniflow.task()` functions. Instead:

- Use `uniflow_time()` in your **workflow** code to obtain the current timestamp.
- Derive all time-based values from that timestamp.
- Pass the calculated values as parameters to your tasks.

This ensures your workflow can be rerun consistently for any point in the past.
:::

## Running a backfill

Set `STARLARK_TIME` to a Unix timestamp (seconds since epoch) representing the historical execution time using the `--env` flag:

```bash
ma pipeline dev-run -f pipeline.yaml --env STARLARK_TIME=unix:<seconds-since-epoch>
```

For example, to run the workflow as if it started on January 1, 2026 at 00:00:00 UTC:

```bash
ma pipeline dev-run -f pipeline.yaml --env STARLARK_TIME=unix:1767225600
```

### Converting a date to a Unix timestamp

Use the `date` command to convert a human-readable timestamp to Unix seconds:

```bash
# Linux (GNU coreutils)
ma pipeline dev-run -f pipeline.yaml \
  --env STARLARK_TIME=unix:$(date -d "2026-01-01T00:00:00Z" +%s)

# macOS / BSD
ma pipeline dev-run -f pipeline.yaml \
  --env STARLARK_TIME=unix:$(date -u -j -f "%Y-%m-%dT%H:%M:%SZ" "2026-01-01T00:00:00Z" +%s)
```

## What's next

- [Pipeline Running Modes](./pipeline-running-modes.md) — understand when to use dev run vs pipeline run
- [Pipeline Management](./pipeline-management.md) — register, run, and monitor pipelines
- [UniFlow Reference](./type-system.md) — type system and workflow primitives
