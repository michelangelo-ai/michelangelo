# Uniflow workflow patterns example

Runnable reference for every core Uniflow orchestration pattern: sequential task calls, if/else branching, for loops, concurrent execution, parallel batch runs, and DatasetVariable for cross-task data sharing.

## What it demonstrates

| Pattern | Where in the code |
|---|---|
| **Sequential tasks** | `baseline = generate_shard(0)` → `baseline_stats = compute_stats(baseline)` |
| **If/else branching** | `if large_dataset == True: n_rows = 5000` |
| **For loop** | `for i in range(n_shards): raw = generate_shard(i, n_rows)` |
| **Concurrent run (fan-out/fan-in)** | `future_a = concurrent_run(generate_shard, 100)` → `shard_a = future_a.result()` |
| **Parallel batch** | `new_callable` + `concurrent_batch_run(callables, max_concurrency=3)` |
| **DatasetVariable** | `generate_shard` → `normalize_shard` → `compute_stats` via `DatasetVariable` |

## Starlark restrictions (workflow functions only)

`@uniflow.workflow` functions run in the Cadence/Temporal Starlark interpreter for deterministic, replayable execution. `@uniflow.task` functions are unrestricted Python.

| Not allowed in `@workflow` | Use instead |
|---|---|
| `import` statements | Put imports inside `@uniflow.task` bodies |
| f-strings (`f"..."`) | `"{}".format(x)` |
| `is` / `is not` | `==` / `!=` |
| `try` / `except` | Handle errors inside tasks |
| Chained comparisons (`1 < x < 5`) | `x > 1 and x < 5` |

## Run

From the `python/` directory:

```bash
poetry install --extras "example"
python -m examples.workflow_patterns.workflow_patterns
```

To run with a larger dataset:

```bash
python -m examples.workflow_patterns.workflow_patterns --input '{"large_dataset": true}'
```

For remote execution on a Michelangelo cluster:

```bash
python -m examples.workflow_patterns.workflow_patterns remote-run \
    --project ma-examples \
    --image ghcr.io/michelangelo-ai/examples:main
```

## Files

- `workflow_patterns.py` — tasks, workflow, and entry point in one file.

## Key import paths (OSS)

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.core.lib.concurrent import (
    run as concurrent_run,
    new_callable,
    batch_run as concurrent_batch_run,
)
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.workflow.variables import DatasetVariable
```
