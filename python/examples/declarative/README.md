# Declarative pipelines: test guide

This guide shows you how to test declarative pipelines (RFC
[michelangelo-ai/enhancements#24](https://github.com/michelangelo-ai/enhancements/pull/24))
on the local sandbox. You run an apiserver from
[#2461](https://github.com/michelangelo-ai/michelangelo/pull/2461) on your machine,
and you write pipelines with `ma` from
[#2462](https://github.com/michelangelo-ai/michelangelo/pull/2462).

These PRs cover how you write and validate a declarative pipeline. They do not
run it. No interpreter exists yet, so do not start a PipelineRun of a
declarative pipeline.

## Contents

| Path | What it is |
|---|---|
| `releases/mock-v1.yaml` | Mock interpreter release. It is the default release. |
| `releases/mock-v2.yaml` | Mock release that adds `timeout_seconds` to `mock/python`. Use it to test a pin upgrade. |
| `apiserver/declarative-test.yaml` | Apiserver config overlay. It turns on the feature and loads the mock releases. |
| `pipelines/hello.yaml` | Valid pipeline: three tasks in a chain. |
| `pipelines/training.yaml` | Valid pipeline: parameters, parallel tasks, retry, a task output reference, and a failure handler. |
| `pipelines/invalid/*.yaml` | Pipelines that the apiserver must reject. Each file names the expected error. |
| `upgrade_pin.py` | Moves a pipeline to another release through the API. `ma` cannot do this yet. |

The mock releases use placeholder artifacts and digests. The apiserver does not
fetch the artifact, so a placeholder is enough to test validation and pinning.

## Before you begin

- Install the tools in the [examples README](../README.md#remote-run): Go,
  Poetry, Docker, k3d, and `kubectl`.
- Create the sandbox, if you do not have one:

  ```bash
  cd python
  poetry run ma sandbox create
  ```

- Check out both PRs in one tree. The two branches merge without conflicts.

  ```bash
  git fetch origin pull/2461/head:declarative-p3 pull/2462/head:declarative-p4
  git checkout declarative-p4
  git merge --no-edit declarative-p3
  ```

## Start the apiserver

The sandbox apiserver listens on port `15566` and runs older code. You start a
second apiserver on your machine on port `15568`. It uses the same cluster.

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

1. Look for these lines in the log:

   ```text
   "msg":"declarative pipelines","enabled":true,"defaultInterpreter":"mock-v1","releases":["mock-v1","mock-v2"]
   "msg":"CRD updated","name":"pipelines.michelangelo.api"
   ```

   The second line shows that the apiserver added `interpreterPin` to the
   Pipeline CRD. Without that field, Kubernetes drops the pin.

Keep the apiserver running. Use a second terminal for the next steps.

## Set up the client

In the second terminal, point `ma` at the new apiserver and create the
namespace:

```bash
cd python
export MACTL_ADDRESS=localhost:15568 MACTL_USE_TLS=false
kubectl --context k3d-michelangelo-sandbox create namespace declarative-test
```

`ma pipeline apply` needs the YAML file in a git repository. This checkout is
one, so you can apply the example files from where they are.

## Test cases

Run each command from the `python/` directory.

### 1. Dry run

```bash
poetry run ma pipeline apply -f examples/declarative/pipelines/hello.yaml --dry-run
```

Expected output:

```text
Interpreter: mock-v1
Tasks: 3

LEVEL  TASKS
0      extract
1      transform
2      load
```

The dry run stores nothing. To check, run
`kubectl -n declarative-test get pipeline declarative-hello`. Expected result:
`NotFound`.

Do the same with `training.yaml`. The plan has 5 tasks in 4 levels.
`train_large` and `train_small` are in the same level. Add `-o json` to get the
plan as JSON.

### 2. Validation errors

Apply each file in `pipelines/invalid/`, with or without `--dry-run`:

```bash
poetry run ma pipeline apply -f examples/declarative/pipelines/invalid/cycle.yaml
```

Each command fails with one line. The apiserver returns `INVALID_ARGUMENT`.

| File | Expected message |
|---|---|
| `cycle.yaml` | `tasks form a depends_on cycle: a -> b -> a` |
| `unknown_task_function.yaml` | `task "a": task_function "mock/spark" is not a task type in mock-v1` |
| `bad_config.yaml` | `task "a": config field "timeout_seconds" is not declared by task type "mock/python" in mock-v1` |
| `bad_param_ref.yaml` | `task "a": config.args[0] references undeclared parameter "dataset"` |
| `retry_not_supported.yaml` | `task "a": retry is not a capability of task type "mock/python" in mock-v1` |

### 3. Create

```bash
poetry run ma pipeline apply -f examples/declarative/pipelines/hello.yaml
kubectl --context k3d-michelangelo-sandbox -n declarative-test \
  get pipeline declarative-hello -o yaml
```

In `spec.manifest`, expect:

- `interpreterPin.version: mock-v1`.
- `interpreterPin.artifact` and `interpreterPin.digest` from `releases/mock-v1.yaml`.
- `uniflowTar` equal to the pin artifact.
- `metadata.generation: 1`, and no `michelangelo/declarative-explain-plan` annotation.

### 4. Re-apply

Apply `hello.yaml` again with no change. Expect the same pin, and
`generation` stays at `1`.

Change a task in `hello.yaml`, commit the change, and apply again. Expect
`generation: 2` and the same pin.

### 5. Pin upgrade

```bash
PYTHONPATH=. poetry run python examples/declarative/upgrade_pin.py \
  declarative-test declarative-hello mock-v2
```

Expected output:

```text
pin: mock-v1 -> mock-v2
artifact: s3://mock-artifacts/declarative/interpreters/mock-v2.tar.gz
digest: sha256:0000000000000000000000000000000000000000000000000000000000000002
uniflow_tar: s3://mock-artifacts/declarative/interpreters/mock-v2.tar.gz
```

Add `--dry-run` to see the result without a write. An unknown version fails
with `INVALID_ARGUMENT` and lists the loaded releases.

After the upgrade, the pipeline validates against `mock-v2`. To check, add
`timeout_seconds: 600` to the `extract` config in `hello.yaml`, commit, and
apply. The apply passes. The same change on a pipeline pinned to `mock-v1`
fails, as in `bad_config.yaml`.

### 6. Server-owned fields

Add `interpreterPin` or `uniflowTar` under `spec.manifest` in a copy of
`hello.yaml`, then apply it. `ma` rejects the file before it calls the
apiserver:

```text
Error: spec.manifest.interpreterPin is server-owned for declarative pipelines; remove it
```

### 7. Feature flag off

Stop the apiserver. Start it again with `declarative.enabled: false` in
`apiserver/declarative-test.yaml`. The startup log shows `"enabled":false`.

Apply `hello.yaml`. Expected result: `FAILED_PRECONDITION`. YAML and UNIFLOW
pipelines work as before.

## What you cannot test yet

- **Run a pipeline.** No interpreter exists, and the run path does not read
  the pin.
- **Pin upgrade with `ma`.** `ma` rejects `interpreterPin` in the YAML. Use
  `upgrade_pin.py`.
- **`resume_from` and caching.** These come with the interpreter.
- **The controller ERROR state.** The sandbox controller runs older code. Unit
  tests in #2461 cover this state.
- **The UI.** The UI does not show declarative pipelines or the explain plan.

## Clean up

1. Delete the test pipelines and the namespace:

   ```bash
   kubectl --context k3d-michelangelo-sandbox delete namespace declarative-test
   ```

1. Stop the apiserver with `Ctrl+C`.

The CRD fields that the new apiserver added stay in the cluster. The sandbox
apiserver runs older code. If it restarts, its CRD sync can fail on those
fields. To reset the cluster, run `poetry run ma sandbox delete`, then
`poetry run ma sandbox create`.

## Troubleshooting

- **`default interpreter "mock-v1" has no release file`.** You started the
  apiserver from a directory other than `go/`. The `releasesDir` path in the
  overlay is relative to `go/`.
- **The pin is missing from the stored pipeline.** The Pipeline CRD has no
  `interpreterPin` field. Check that the startup log shows `CRD updated` for
  `pipelines.michelangelo.api`.
- **`ma` connects to the wrong apiserver.** Check `MACTL_ADDRESS`. A
  `~/.ma/config.toml` file does not override the environment variable.
