---
name: ma-sandbox-setup
description: Canonical setup sequence for the Michelangelo local sandbox. Use when setting up a new dev machine, diagnosing sandbox issues, or helping someone get unstuck during sandbox creation. Also applies when checking prereqs or explaining what each step does.
user-invocable: true
---

# Michelangelo Sandbox Setup Reference

## Prereqs

Install the required tools if they are not already on your PATH:

```bash
brew install k3d       # cluster manager (v5.x)
brew install helm      # Kubernetes package manager
# kubectl comes with Docker Desktop, or: brew install kubectl
```

Verify all five are on PATH before proceeding:

```bash
which k3d helm kubectl docker poetry
```

IF any command prints "not found" or returns no output: **STOP**. Report which tools are missing and do not proceed to the next step.

**Docker resource limits:** Ensure your Docker runtime (Docker Desktop or Colima) has at least 4 CPUs, 8 GB memory, and 60 GB disk allocated, or pods will crash or fail to schedule.

## Full ordered setup sequence

### 1. Install Python dependencies

```bash
cd <repo-root>/python
poetry install
```

This must be done before any `ma` CLI commands. If skipped, `ma` will fail with an import error because its Python dependencies aren't installed.

### 2. Install the plugin extra (Ray + Spark — optional)

Skip this if you're only doing UI or apiserver work. Required if you'll run or develop pipelines that use Ray or Spark compute:

```bash
poetry install --extras plugin
```

### 3. Build kuberay images (required for Ray history-server)

```bash
bash "$REPO_ROOT/scripts/kuberay/build-kuberay-images.sh"
```

Without this, `kuberay-historyserver` will be stuck in `ImagePullBackOff` after create — the image isn't in any public registry. Ray jobs still work without it, but the build step is cheap and avoids the noise.

### 4. Activate the venv and create the sandbox

```bash
REPO_ROOT=$(git rev-parse --show-toplevel)
source "$REPO_ROOT/python/.venv/bin/activate"     # or prefix every command with: poetry run

ma sandbox create
```

### 5. Seed demo data

```bash
cd "$REPO_ROOT/python"
poetry run ma sandbox demo pipeline
```

This creates the `ma-dev-test` project with training, eval, and trigger pipelines. Without this step the UI will load but show no data.

### 6. Verify

```bash
poetry run ma sandbox health
```

All checks should pass. Then open **http://localhost:8090** — navigate to the `ma-dev-test` project.

## Key commands

| Command                     | What it does                                                    |
| --------------------------- | --------------------------------------------------------------- |
| `ma sandbox create`         | Create cluster + deploy all services                            |
| `ma sandbox sync`           | Restart app services in an existing cluster (fast, skips infra) |
| `ma sandbox health`         | Run health checks: cluster, pods, API resources, envoy, UI      |
| `ma sandbox stop`           | Stop the cluster (preserves state)                              |
| `ma sandbox start`          | Resume a stopped cluster                                        |
| `ma sandbox delete`         | Tear down cluster entirely                                      |
| `ma sandbox demo pipeline`  | Deploy pipeline demo resources                                  |
| `ma sandbox demo inference` | Deploy inference server demo resources                          |

## Debugging tools

If the UI loads but shows no data, or services aren't behaving as expected, use `/ma-sandbox-debug`.

## Known gotchas

**k3d 5.9.0 + k3s version** — k3d 5.9.0 defaulted to k3s v1.35.5 (pre-release, broken). `sandbox.py` now pins `rancher/k3s:v1.30.5-k3s1` explicitly. If you see the API server never come up after `ma sandbox create`, check that you're on a recent checkout.

**`cadence-schema-init` / `ingester-schema-init` / `sandbox-bucket-setup`** — these reach `Completed` status and stay there. That's expected.

## Further reading

Full docs: `docs/getting-started/ma-sandbox-setup.md`.
