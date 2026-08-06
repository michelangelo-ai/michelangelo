# CanvasFlex Tabular Train YAML Pipeline Demo

A minimal form of the internal `tabular_train` pipeline, authored entirely through
`pipeline_conf.yaml`: a Spark task loads a dataset from a configurable Spark SQL query and splits
it into train/validation, then a Ray task grid-searches a (deliberately simple) threshold model in
parallel. The workflow is generic — every use-case-specific value comes from YAML.

## Make it your own

All of these change behavior with **zero workflow-code changes**:

- **Data source** — `source.spark_sql.query`: the demo generates rows with `FROM range(200)`; point
  it at a real table instead (`SELECT label, score FROM my_db.my_table WHERE datestr = ...`) when a
  metastore or `s3a://` data is reachable. A registered-dataset variant (internal's
  `source.dataset.{namespace,name}`) is future work — OSS has no dataset registry yet.
- **Split** — `split.ratio.train_ratio`.
- **Columns / hyperparameters** — `feature_column`, `target_column`, `num_thresholds`.
- **Cluster sizing** — per-task `job_specs` (Spark driver/executor resources + instances, Ray
  head/worker resources + min/max instances). The decorators pass bare `SparkTask()`/`RayTask()`,
  so sizing comes from `job_specs`, falling back to the Starlark defaults when absent — the same
  division of labor as internal (`Ray()`/`Spark()` decorators, YAML owns resources).

## Files

- `workflow.py` — task config schemas, the two tasks (decorated with `pipeline_task`), and the
  workflow; the package `__init__` re-exports the workflow as the stable `workflow_function` alias
  that the YAML references.
- `pipeline_conf.yaml` — the YAML config: workflow reference, per-task `config`, per-task
  `job_specs`.
- `run_example.py` — loads `pipeline_conf.yaml` and runs the workflow in-process.
- `Dockerfile` — slim CPU-only image (~2.2GB) for running the pipeline in the `ma sandbox` k3d
  stack.

## How to Run (Local)

Runs the task bodies in-process: `SparkTask.pre_run` starts a local Spark session and
`RayTask.pre_run` starts a local Ray runtime (requires `pyspark` and `ray` installed).

```bash
cd michelangelo-ai/michelangelo/python
source .venv/bin/activate
poetry run python -m examples.canvasflex_tabular_train.run_example
```

Equivalent, through the standard Uniflow context (also validates the workflow build):

```bash
_SPARK_PROPERTIES="spark.master=local[2]" \
  poetry run python -m michelangelo.canvas.pipeline.register \
  examples/canvasflex_tabular_train/pipeline_conf.yaml local-run
```

## How to Run (Sandbox e2e)

With an `ma sandbox create` k3d cluster up:

```bash
cd michelangelo-ai/michelangelo/python
docker build -t canvasflex-demo:latest -f ./examples/canvasflex_tabular_train/Dockerfile .
k3d image import canvasflex-demo:latest -c michelangelo-sandbox
UFC_CADENCE_TRANSPORT=grpc UFC_CADENCE_ADDRESS=127.0.0.1:7833 \
  poetry run python -m michelangelo.canvas.pipeline.register \
  examples/canvasflex_tabular_train/pipeline_conf.yaml \
  remote-run --image docker.io/library/canvasflex-demo:latest --storage-url s3://default --yes
```

The created SparkApplication/RayCluster CRs carry the `job_specs` values (2-core driver and
executors, 2 Ray workers at cpu=2), not the Starlark defaults — that's the override path working.

## Scope

Plain YAML only: no `{{var.}}`/`{{task.}}`/`{{fn.}}` templating and no custom tags like
`!py_import` — those are follow-up work. Optional tasks (internal's
`if "task_name" in task_configs:` idiom) are supported by the loader and workflow layer; this
example keeps the graph to two required tasks for clarity.
