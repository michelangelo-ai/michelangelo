# Declarative pipelines

A declarative pipeline sets `spec.manifest.type` to `DECLARATIVE` and lists
its tasks under `spec.manifest.content`. The apiserver validates the task
graph against an interpreter release. It then pins the pipeline to that
release.

This guide shows you how to write a declarative pipeline. It also shows you
how to validate, create, and pin one with `ma pipeline apply` on the local
sandbox, and how to run one.

## Requirements

- The apiserver must run with `apiserver.declarative.enabled: true`. With the
  flag on, the apiserver does not start unless `releasesDir` holds the
  `defaultInterpreter` release. With the flag off, a declarative write fails
  with `FAILED_PRECONDITION`.
- An existing sandbox must run control-plane images that include
  [#2222](https://github.com/michelangelo-ai/michelangelo/pull/2222). An older
  controllermgr cannot list Pipelines while a pinned pipeline exists. Its log
  then shows `unknown field "interpreterPin"`.
- To run a declarative pipeline, the apiserver and the controller manager must
  include the declarative run path. See [Run a pipeline](#run-a-pipeline).

## Contents

| Path | What it is |
|---|---|
| `releases/mock-v1.yaml` | Mock interpreter release. It is the default release. |
| `releases/mock-v2.yaml` | Mock release that adds `timeout_seconds` to `mock/python`. Use it to change the pin. |
| `apiserver/declarative-test.yaml` | Apiserver config overlay. It turns on the feature and loads the mock releases. |
| `pipelines/training.yaml` | Valid pipeline: parameters, parallel tasks, retry, a task output reference, and a failure handler. |
| `pipelines/run.yaml` | PipelineRun of `declarative-training`. |
| `pipelines/invalid/*.yaml` | Pipelines that the apiserver rejects. The first line of each file names the error. |

The mock releases use `mock://` artifacts and placeholder digests. No blob
store serves the `mock://` scheme, so nothing can fetch these artifacts. The
apiserver does not fetch the artifact, so a placeholder is enough to test
validation and pinning.

## Write a declarative pipeline

Write the `DeclarativeWorkflow` fields under `spec.manifest.content`. See
`pipelines/training.yaml` for a full example.

- You can write enum values as short names, in any case. For example, write
  `when: failed` for `OUTCOME_FAILED`, and `type: DECLARATIVE` for
  `PIPELINE_MANIFEST_TYPE_DECLARATIVE`.
- When a parameter has no `type`, `ma` sets the type from the `default` value.
  A parameter with no `type` and no default is an error.
- Quote a value when its YAML type is not clear. YAML reads `yes` as a
  boolean, `0755` as the integer 493, and `1:30` as the integer 90. Write
  `"0755"`, or set `type: string` on the parameter.
- Quote a mapping key that YAML reads as a number or a boolean, such as `0:`
  or `on:`. `ma` rejects keys that are not strings.
- Write an integer outside ±(2^53-1) as a string. `ma` also rejects `.nan` and
  `.inf`.
- To pin the pipeline to a release, set `spec.manifest.interpreterPin.version`.
  With no version, the apiserver pins a new pipeline to its default release.
  The apiserver sets the pin artifact, the pin digest, and `uniflowTar`.

`ma pipeline apply` needs the YAML file in a git repository. It writes the
current commit to `spec.commit`.

## Start the apiserver

The sandbox apiserver does not load the mock releases. You start a second
apiserver on your machine on port `15568`. It uses the same cluster.

1. Install the tools in the [examples README](../README.md#remote-run): Go,
   Poetry, Docker, k3d, and `kubectl`.

1. Create the sandbox, if you do not have one:

   ```bash
   cd python
   poetry install
   poetry run ma sandbox create
   ```

1. Point the apiserver at the sandbox cluster only:

   ```bash
   export KUBECONFIG=$(k3d kubeconfig write michelangelo-sandbox)
   kubectl config current-context
   ```

   Expected output: `k3d-michelangelo-sandbox`.

1. From the `go/` directory, start the apiserver with the overlay:

   ```bash
   cd go
   CONFIG_DIR=./cmd/apiserver/config:../python/examples/declarative/apiserver \
   RUNTIME_ENVIRONMENT=declarative-test \
   go run ./cmd/apiserver
   ```

1. Look for this line in the log:

   ```text
   "msg":"declarative pipelines","enabled":true,"defaultInterpreter":"mock-v1","releases":["mock-v1","mock-v2"]
   ```

   The apiserver also logs
   `"msg":"CRD updated","name":"pipelines.michelangelo.api"` on every start.
   This line shows that the CRD sync ran and that the cluster did not refuse
   the update. If the Pipeline CRD has no `interpreterPin` field, this update
   adds it.

Keep the apiserver running. Use a second terminal for the next steps.

## Set up the client

1. In the second terminal, install `ma`, and point it at the new apiserver:

   ```bash
   cd python
   poetry install
   export MACTL_ADDRESS=localhost:15568 MACTL_USE_TLS=false
   ```

   `ma` needs no Poetry extras.

1. Create the namespace:

   ```bash
   kubectl --context k3d-michelangelo-sandbox create namespace declarative-test
   ```

1. Create a test branch. Some steps commit changes to `training.yaml`.

   ```bash
   git switch -c declarative-test
   ```

## Test cases

Run each command from the `python/` directory.

### 1. Dry run

```bash
poetry run ma pipeline apply -f examples/declarative/pipelines/training.yaml --dry-run
```

Expected output:

```text
Interpreter: mock-v1
Tasks: 5

LEVEL  TASKS
0      prepare
1      train_large, train_small
2      evaluate
3      notify_failure

WARNINGS
tasks.notify_failure.depends_on[0]: the handler runs, but it gets no error text until outcome mode ships
tasks.train_large.retry.initial_interval is not used yet
tasks.train_small.retry.initial_interval is not used yet

Dry run: nothing was saved.
```

`train_large` and `train_small` are in the same level, so they can run in
parallel. Add `-o json` to get the plan as JSON.

The warnings name features that the apiserver accepts but the interpreter
does not use yet. They do not stop the apply.

The dry run stores nothing. To check, run this command:

```bash
kubectl --context k3d-michelangelo-sandbox -n declarative-test \
  get pipeline declarative-training
```

Expected output:
`Error from server (NotFound): pipelines.michelangelo.api "declarative-training" not found`.

### 2. Validation errors

Apply each file in `pipelines/invalid/`, with or without `--dry-run`:

```bash
poetry run ma pipeline apply -f examples/declarative/pipelines/invalid/cycle.yaml
```

Each command prints one line that starts with `Error: INVALID_ARGUMENT:`, and
exits with code 1.

| File | Expected message |
|---|---|
| `cycle.yaml` | `tasks form a depends_on cycle: a -> b -> a` |
| `unknown_task_function.yaml` | `task "a": task_function "mock/spark" is not a task type in mock-v1` |
| `bad_config.yaml` | `task "a": config field "timeout_seconds" is not declared by task type "mock/python" in mock-v1` |
| `bad_param_ref.yaml` | `task "a": config.args[0] references undeclared parameter "dataset"` |
| `retry_not_supported.yaml` | `task "a": retry is not a capability of task type "mock/python" in mock-v1` |

### 3. Create

1. Apply the pipeline:

   ```bash
   poetry run ma pipeline apply -f examples/declarative/pipelines/training.yaml
   ```

   `ma` prints the stored pipeline. The `content` block shows the stored
   workflow under `[type.googleapis.com/michelangelo.api.v2.DeclarativeWorkflow]`.

1. Read the stored pipeline:

   ```bash
   kubectl --context k3d-michelangelo-sandbox -n declarative-test \
     get pipeline declarative-training -o yaml
   ```

In `spec.manifest`, expect:

- `interpreterPin.version: mock-v1`.
- `interpreterPin.artifact` and `interpreterPin.digest` from `releases/mock-v1.yaml`.
- `uniflowTar` equal to the pin artifact.

Also expect `metadata.generation: 1`, and no
`michelangelo/declarative-explain-plan` annotation.

### 4. Re-apply

`ma` writes the commit SHA to `spec.commit.git_ref`. Thus a new commit alone
changes the spec and increments `generation`.

1. Apply `training.yaml` again with no change and no new commit. Expect the
   same pin. `generation` stays at `1`.

1. Make a commit with no change, and apply again:

   ```bash
   git commit --allow-empty -m "test: no change"
   poetry run ma pipeline apply -f examples/declarative/pipelines/training.yaml
   ```

   Expect `generation: 2` and the same pin. The new commit causes this
   change.

1. Change `learning_rate` of `train_small` to `0.02`. Commit the change, and
   apply again. Expect `generation: 3` and the same pin. This step shows that
   an edit passes validation and keeps the pin.

Commit each change before you apply it. The controller keeps one revision for
each commit. A different spec on the same commit can fail in the controller.

### 5. Change the pin

1. Add the pin version under `spec.manifest` in `training.yaml`:

   ```yaml
   spec:
     manifest:
       type: PIPELINE_MANIFEST_TYPE_DECLARATIVE
       interpreterPin:
         version: mock-v2
   ```

1. Commit the change, and apply it. In the stored pipeline, expect:

   - `interpreterPin.version: mock-v2`.
   - `interpreterPin.artifact: mock://declarative/interpreters/mock-v2.tar.gz`.
   - `interpreterPin.digest: sha256:0000000000000000000000000000000000000000000000000000000000000002`.
   - `uniflowTar: mock://declarative/interpreters/mock-v2.tar.gz`.

1. Add `timeout_seconds: 600` to the `prepare` config. Commit, and apply. The
   apply passes, because `mock-v2` declares `timeout_seconds`. The same change
   fails on `mock-v1`, as in `bad_config.yaml`.

1. Set the version to `mock-v9`, and apply with `--dry-run`. Expected output:

   ```text
   Error: INVALID_ARGUMENT: interpreter_pin.version "mock-v9" is not a known release; use one of [mock-v1, mock-v2]
   ```

   Set the version back to `mock-v2`.

You can also pin a new pipeline on create:

1. Copy `bad_config.yaml`:

   ```bash
   cp examples/declarative/pipelines/invalid/bad_config.yaml \
     examples/declarative/bad_config_mock_v2.yaml
   ```

1. In the copy, add `interpreterPin.version: mock-v2` under `spec.manifest`.

1. Apply the copy with `--dry-run`. The plan shows `Interpreter: mock-v2`, and
   the apply passes.

### 6. Server-owned fields

Copy `training.yaml`:

```bash
cp examples/declarative/pipelines/training.yaml \
  examples/declarative/training_server_owned.yaml
```

Add `uniflowTar` or `interpreterPin.artifact` under `spec.manifest` in the
copy, then apply it. `ma` rejects the file before it sends a Pipeline request to the apiserver:

```text
Error: spec.manifest.uniflowTar is server-owned for declarative pipelines; remove it, because the apiserver sets it from the interpreter pin
Error: spec.manifest.interpreterPin.artifact is server-owned; to change the pin, set only interpreterPin.version
```

### 7. Release not loaded

A pipeline stays pinned to its release. Remove a release only when no
pipeline uses it.

1. Stop the apiserver. Move `releases/mock-v2.yaml` out of the releases
   directory, and start the apiserver again.

1. Apply `training.yaml`. It is pinned to `mock-v2`. Expected output:

   ```text
   Error: FAILED_PRECONDITION: interpreter mock-v2 is not loaded on this apiserver; set interpreter_pin.version to one of [mock-v1]
   ```

1. Stop the apiserver. Move `mock-v2.yaml` back, and start the apiserver
   again.

### 8. Feature flag off

1. Stop the apiserver. Set `declarative.enabled: false` in
   `apiserver/declarative-test.yaml`, and start the apiserver again. The
   startup log shows `"enabled":false`.

1. Apply `training.yaml`. Expected output:

   ```text
   Error: FAILED_PRECONDITION: declarative pipelines are not enabled on this apiserver; an operator must set apiserver.declarative.enabled=true
   ```

YAML and UNIFLOW pipelines work as before.

### 9. Trigger a run with the flag off

With the flag off, as in test 8, trigger a run of `declarative-training`:

```bash
poetry run ma pipeline run -n declarative-test --name declarative-training
```

The apiserver rejects the run with `FAILED_PRECONDITION` and the message
from test 8.

## Run a pipeline

The run controller starts a declarative run with the interpreter release in
the pin. The interpreter runs the tasks with the task-type factories of the
release. The `core-v0.1.0` release has one task type, `core/ray`.

### Prepare the release

1. Build the release tar and the release file:

   ```bash
   cd python
   poetry run python -m michelangelo.uniflow.declarative.assemble \
     michelangelo/uniflow/declarative/releases/core-v0.1.0.src.yaml \
     --root .. --out ../.declarative-build/release
   ```

   The command writes `core-v0.1.0.tar.gz` and `core-v0.1.0.yaml`. The
   release file holds the artifact path and the digest of the tar.

1. Upload the tar to the `artifact` path of the release file. The controller
   manager reads the tar from its blob store. On the sandbox, the S3 API of
   the object store listens on port `9091`:

   ```bash
   secret() {
     kubectl --context k3d-michelangelo-sandbox get secret object-storage-credentials \
       -o "jsonpath={.data.$1}" | base64 -d
   }
   AWS_ACCESS_KEY_ID=$(secret AWS_ACCESS_KEY_ID) \
   AWS_SECRET_ACCESS_KEY=$(secret AWS_SECRET_ACCESS_KEY) \
   aws --endpoint-url http://localhost:9091 s3 cp \
     ../.declarative-build/release/core-v0.1.0.tar.gz \
     s3://default/interpreters/core-v0.1.0.tar.gz
   ```

1. Copy `core-v0.1.0.yaml` into the `releasesDir` of the apiserver, and
   restart the apiserver. The startup log lists `core-v0.1.0`.

Do not change the tar after you upload it. The controller compares the
digest of the tar with the digest in the pin.

### Write a pipeline that runs

- Use the task types of the release, such as `core/ray`.
- Set `spec.manifest.interpreterPin.version` to the release, such as
  `core-v0.1.0`, unless the release is the default release.
- Add the `michelangelo/uniflow-image` annotation. The ImageBuild step fails a
  run with no image. Every task runs in this image, so the image must hold the
  code of each `entry_point`.
- Give each parameter a default. A run cannot set parameter values yet.

### Trigger a run

The run namespace needs a Project with the same name. The controller reads
the worker queue from the Project.

Trigger a run by pipeline name:

```bash
poetry run ma pipeline run -n declarative-test --name <pipeline>
```

You can also apply a PipelineRun file, such as `pipelines/run.yaml`:

```bash
poetry run ma pipeline_run apply -f examples/declarative/pipelines/run.yaml
```

The apiserver checks each declarative run before it creates the run. It
rejects the run in these cases:

| Case | Code |
|---|---|
| The run names a pipeline in another namespace, and the pipeline with that name in the run namespace is declarative. | `INVALID_ARGUMENT` |
| The flag is off. | `FAILED_PRECONDITION` |
| The pipeline has no pin, or the pin does not equal a loaded release. | `FAILED_PRECONDITION` |
| The workflow does not conform to the pinned release. | `INVALID_ARGUMENT` |
| The run input sets `task_configs`, `workflow_config`, `args`, `kw_args`, or `parameter_map`. | `INVALID_ARGUMENT` |
| The run input sets an `environ` key other than `LAST_SCHEDULE_TIMESTAMP`. | `INVALID_ARGUMENT` |
| A parameter has no default. | `INVALID_ARGUMENT` |

### Start failures

When the controller cannot start a run, it sets the `Execute Workflow` step
to `FAILED`. The run then fails, and `status.errorMessage` holds the step
message:

| Case | Message |
|---|---|
| No pin | `declarative pipeline run has no interpreter pin; re-apply the pipeline through the API` |
| Fetch fails | `cannot fetch interpreter <version>` |
| Tar too large | `interpreter <version> is larger than the 1 MiB limit` |
| Digest mismatch | `interpreter <version> does not match the digest in its pin` |
| Spec does not decode | `cannot decode declarative spec: <error>` |

The step message names only the interpreter version. The controller writes the
artifact path, the fetch error, the tar size, and the digests to its log, at
the `WARN` level, with the run name. A fetch failure is final. To try again,
trigger a new run.

### Cron triggers

A cron trigger can run a declarative pipeline. The trigger adds
`LAST_SCHEDULE_TIMESTAMP` to the run environment after the first run.

Do not set `parameter_map` on a cron trigger of a declarative pipeline. The
trigger copies `parameter_map` into the run input, and the apiserver rejects
the run with `parameter overrides are not supported yet`. The trigger retries
the create 5 times. In batch mode, the default, the trigger then logs the error and
creates no run. In concurrent mode, the trigger fails.

## Clean up

1. Delete the test pipelines and the namespace:

   ```bash
   kubectl --context k3d-michelangelo-sandbox delete namespace declarative-test
   ```

1. Stop the apiserver with `Ctrl+C`.

1. Delete the copies from steps 5 and 6. Undo the overlay change from step 8,
   and delete the test branch:

   ```bash
   rm examples/declarative/bad_config_mock_v2.yaml \
     examples/declarative/training_server_owned.yaml
   git restore examples/declarative/apiserver/declarative-test.yaml
   git switch -
   git branch -D declarative-test
   ```

## Troubleshooting

- **`load declarative releases: open ../python/examples/declarative/releases: no such file or directory`**,
  or the apiserver cannot find its config. You started the apiserver from a
  directory other than `go/`. `CONFIG_DIR` and `releasesDir` are both
  relative to `go/`.
- **`default interpreter "mock-v1" has no release file in ...`.** The releases
  directory has no `mock-v1.yaml`, or `defaultInterpreter` in the overlay
  changed.
- **The pin is missing from the stored pipeline.** The Pipeline CRD has no
  `interpreterPin` field. Check that the startup log shows `CRD updated` for
  `pipelines.michelangelo.api`.
- **`ma` connects to the wrong apiserver.** Check `MACTL_ADDRESS`. A
  `~/.ma/config.toml` file does not override the environment variable.
