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
- `pipeline_conf.yaml` — the pipeline configuration: workflow reference, per-task `config`,
  per-task `job_specs`.
- `pipeline.yaml` — the Pipeline resource for production registration (`ma pipeline apply`):
  project (namespace), pipeline name, task image annotation, and a manifest pointing at
  `pipeline_conf.yaml`.
- `run_example.py` — loads `pipeline_conf.yaml` and runs the workflow in-process.
- `Dockerfile` — slim CPU-only image (~2.2GB) for running the pipeline in the `ma sandbox` k3d
  stack.

All commands below run from the repo's `python/` directory with the venv active:

```bash
cd michelangelo-ai/michelangelo/python
source .venv/bin/activate
```

## Run locally (in-process)

Task bodies run in this process: `SparkTask.pre_run` starts a local Spark session and
`RayTask.pre_run` a local Ray runtime (requires `pyspark` and `ray` installed).

```bash
python -m examples.canvasflex_tabular_train.run_example
```

Equivalent, through the standard Uniflow context (also validates the workflow build/transpile):

```bash
_SPARK_PROPERTIES="spark.master=local[2]" \
  python -m michelangelo.canvas.pipeline.register \
  examples/canvasflex_tabular_train/pipeline_conf.yaml local-run
```

## Sandbox prerequisites (for both remote modes)

With an `ma sandbox create` k3d cluster up, build and import the task image once:

```bash
docker build -t canvasflex-demo:latest -f ./examples/canvasflex_tabular_train/Dockerfile .
k3d image import canvasflex-demo:latest -c michelangelo-sandbox
```

## Run remotely (ad hoc dev loop)

Submits straight to Cadence — no registration, nothing recorded in MA Studio. Good for iterating.

```bash
UFC_CADENCE_TRANSPORT=grpc UFC_CADENCE_ADDRESS=127.0.0.1:7833 \
  python -m michelangelo.canvas.pipeline.register \
  examples/canvasflex_tabular_train/pipeline_conf.yaml \
  remote-run --image docker.io/library/canvasflex-demo:latest --storage-url s3://default --yes
```

## Register and run (production path)

Registers the pipeline with the apiserver — `ma pipeline apply` detects the
`pipeline_conf.yaml` manifest, resolves it, uploads the workflow tarball, and stores the
Pipeline resource; runs are then created against the registered pipeline (this is what MA
Studio shows). No bazel targets and no per-pipeline scripts involved.

One-time: register the sandbox demo project (`ma-dev-test`, the project all example pipelines
use — a project owns the namespace pipelines live in):

```bash
ma project apply -f michelangelo/cli/sandbox/demo/project.yaml
```

Register (or re-register after edits) the pipeline:

```bash
ma pipeline apply -f examples/canvasflex_tabular_train/pipeline.yaml
```

Start a run (run names must be unique — bump the suffix per run). The task image comes from
`pipeline.yaml`'s `michelangelo/uniflow-image` annotation:

```bash
cat <<'EOF' > /tmp/canvasflex-run.yaml
apiVersion: michelangelo.api/v2
kind: PipelineRun
metadata:
  name: canvasflex-tabular-train-run-1
  namespace: ma-dev-test
spec:
  pipeline:
    name: canvasflex-tabular-train
    namespace: ma-dev-test
EOF
ma pipeline_run apply -f /tmp/canvasflex-run.yaml
```

Watch it:

```bash
ma pipeline_run get -n ma-dev-test
```

If your `~/.ma/config.toml` points at another deployment, prefix the `ma` commands with the
sandbox connection settings:

```bash
MACTL_ADDRESS=127.0.0.1:15566 MACTL_USE_TLS=false MACTL_RPC_SERVICE=ma-apiserver ma ...
```

In all remote modes, the created SparkApplication/RayCluster CRs carry the `job_specs` values
(2-core driver and executors, 2 Ray workers at cpu=2), not the Starlark defaults — that's the
override path working.

## Scope

Plain YAML only: no `{{var.}}`/`{{task.}}`/`{{fn.}}` templating and no custom tags like
`!py_import` — those are follow-up work. Optional tasks (internal's
`if "task_name" in task_configs:` idiom) are supported by the loader and workflow layer; this
example keeps the graph to two required tasks for clarity.
