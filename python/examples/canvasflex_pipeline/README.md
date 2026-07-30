# CanvasFlex-style YAML Pipeline Authoring Demo

Demonstrates the new `pipeline_conf.yaml`-driven authoring flow (`michelangelo.canvas.pipeline`):
a single unified `pipeline_task` decorator plus a config loader that resolves a workflow function
and its per-task configs from YAML, instead of wiring tasks together in plain Python.

## Files

- `workflow.py` — the workflow and task functions, each task decorated with `pipeline_task`.
- `pipeline_conf.yaml` — the YAML config: which workflow to run, its workflow-level config, and
  each task's config.
- `run_example.py` — loads `pipeline_conf.yaml` and runs the workflow in-process.

## How to Run

```bash
cd michelangelo-ai/michelangelo/python
source .venv/bin/activate
poetry run python -m examples.canvasflex_pipeline.run_example
```

## Expected Output

```
result: {'experiment_name': 'canvasflex-yaml-demo', 'model': {'model': 'model-trained-on-cats-vs-dogs', 'final_loss': 0.02}}
```

## Scope

This example uses a minimal local `TaskConfig` so it runs without a Ray/Spark cluster. It also uses
plain YAML (no `{{var.}}`/`{{task.}}`/`{{fn.}}` templating, no custom YAML tags) — those are
follow-up work, not part of this phase.
