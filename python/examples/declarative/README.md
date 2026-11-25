# Declarative pipelines

A declarative pipeline sets `spec.manifest.type` to `DECLARATIVE` and lists
its tasks under `spec.manifest.content`. The apiserver validates the task
graph against an interpreter release. It then pins the pipeline to that
release.

This guide shows you how to write a declarative pipeline. It also shows you
how to validate, create, and pin one with `ma pipeline apply` on the local
sandbox.

## Requirements

- The apiserver must run with `apiserver.declarative.enabled: true`. With the
  flag on, the apiserver does not start unless `releasesDir` holds the
  `defaultInterpreter` release. With the flag off, a declarative write fails
  with `FAILED_PRECONDITION`.
- An existing sandbox must run control-plane images that include
  [#2222](https://github.com/michelangelo-ai/michelangelo/pull/2222). An older
  controllermgr cannot list Pipelines while a pinned pipeline exists. Its log
  then shows `unknown field "interpreterPin"`.
- No interpreter runs declarative pipelines yet. Do not start a PipelineRun of
  a declarative pipeline.

## Contents

| Path | What it is |
|---|---|
| `releases/mock-v1.yaml` | Mock interpreter release. It is the default release. |
| `releases/mock-v2.yaml` | Mock release that adds `timeout_seconds` to `mock/python`. Use it to change the pin. |
| `apiserver/declarative-test.yaml` | Apiserver config overlay. It turns on the feature and loads the mock releases. |
| `pipelines/training.yaml` | Valid pipeline: parameters, parallel tasks, retry, a task output reference, and a failure handler. |
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

Dry run: nothing was saved.
```

`train_large` and `train_small` are in the same level, so they can run in
parallel. Add `-o json` to get the plan as JSON.

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
copy, then apply it. `ma` rejects the file before it calls the apiserver:

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
