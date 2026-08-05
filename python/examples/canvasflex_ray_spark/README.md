# CanvasFlex Ray/Spark YAML Pipeline Demo

A `tabular_eval`-shaped pipeline authored entirely through `pipeline_conf.yaml`: a Spark task generates a small synthetic tabular dataset, a Ray task scores it against a threshold, and an optional Ray task summarizes the metrics. Demonstrates YAML-driven task configuration (`config` blocks), per-task resource sizing (`job_specs`), and optional tasks.

## Features

- **YAML Authoring**: workflow wiring and per-task configs live in `pipeline_conf.yaml`, not Python
- **Spark + Ray**: Spark data prep followed by Ray distributed scoring in one workflow
- **job_specs Overrides**: YAML `job_specs` resource values win over the decorator defaults at run time
- **Optional Tasks**: the `summarize` task only runs when `task_configs` defines a `summarize` entry
- **Synthetic Data**: everything is generated at task runtime — no external downloads

## How to Run (Local)

Runs the task bodies in-process: `SparkTask.pre_run` starts a local Spark session and `RayTask.pre_run` starts a local Ray runtime (requires `pyspark` and `ray` installed).

```bash
cd michelangelo-ai/michelangelo/python
source .venv/bin/activate
poetry run python -m examples.canvasflex_ray_spark.run_example
```

Equivalent, through the standard Uniflow context (also validates the workflow build):

```bash
_SPARK_PROPERTIES="spark.master=local[2]" \
  poetry run python -m michelangelo.canvas.pipeline.register \
  examples/canvasflex_ray_spark/pipeline_conf.yaml local-run
```

## How to Run (Remote, k3d sandbox)

Same CLI shape as `python examples/bert_cola/bert_cola.py remote-run ...`, with the YAML path in front:

```bash
poetry run python -m michelangelo.canvas.pipeline.register \
  examples/canvasflex_ray_spark/pipeline_conf.yaml \
  remote-run --image <IMAGE> --storage-url <STORAGE_URL> --yes
```

The workflow is transpiled to Starlark and submitted through the standard remote-run path; each `task_configs` entry travels as a typed `TaskConfig` envelope, so the Spark/Ray task runtimes apply the YAML `job_specs` (e.g. driver/head `cpu: 2` in the YAML wins over the decorators' `cpu=1` defaults — check the created SparkJob/RayJob resources to confirm).

## Expected Output

```
prepare_data: generated 200 rows
evaluate: metrics {'rows': 200, 'correct': 98, 'accuracy': 0.49, 'threshold': 0.5}
summarize: summary {'rows': 200, 'correct': 98, 'accuracy': 0.49, 'threshold': 0.5, 'experiment_name': 'canvasflex-ray-spark-demo'}
result: {'rows': 200, 'correct': 98, 'accuracy': 0.49, 'threshold': 0.5, 'experiment_name': 'canvasflex-ray-spark-demo'}
```

To see the optional-task behavior, delete the `summarize` entry from `pipeline_conf.yaml` and re-run: the workflow returns the raw metrics without the `experiment_name` tag.
