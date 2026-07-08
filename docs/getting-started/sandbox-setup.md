---
sidebar_position: 3
---

# Sandbox Setup

This guide walks you through setting up a local Michelangelo environment on your laptop. The sandbox runs a fully functional cluster — API server, controller manager, workflow engine, object storage, and supporting services — entirely on your machine, so you can explore Michelangelo or develop against it without any cloud infrastructure.

**Who this is for:** ML engineers, platform engineers, and contributors who want to try Michelangelo locally or develop new features against it.

**What you'll have at the end:** a running sandbox cluster and a successful demo pipeline run, ready for you to build your own workflows on top of.

**Time estimate:** 30–60 minutes on first run (image pulls for all services can take 20–40 minutes depending on your connection). Subsequent recreates with cached images typically take 5–10 minutes.

**Supported platforms:** macOS (Apple Silicon and Intel) and Linux. Windows is not officially supported, but WSL2 with Docker Desktop should work for most steps.

## Get the code

Clone the Michelangelo repository to your machine:

```bash
git clone https://github.com/michelangelo-ai/michelangelo.git
cd michelangelo
```

Throughout this guide, `<repo-root>` refers to the directory you just cloned (for example, `~/michelangelo`).

## Prerequisites

Before you begin, make sure you have the following installed. Install commands below show macOS (Homebrew) and Linux options where they differ; on Linux, follow the linked official guide if you don't see a direct command. Run each verification command to confirm:

| Tool | Install (macOS) | Install (Linux) | Verify |
|------|-----------------|-----------------|--------|
| **Docker** | [Docker Desktop](https://docs.docker.com/get-started/get-docker) or [Colima](https://github.com/abiosoft/colima) | [Docker Engine](https://docs.docker.com/engine/install/) | `docker info` |
| **kubectl** | `brew install kubectl` | [official guide](https://kubernetes.io/docs/tasks/tools/#kubectl) | `kubectl version --client` |
| **k3d** | `brew install k3d` | [official guide](https://k3d.io/#installation) | `k3d --version` |
| **Helm** | `brew install helm` | [official guide](https://helm.sh/docs/intro/install/) | `helm version` |
| **Python 3.11 or 3.12** | [python.org](https://www.python.org/downloads/) or `brew install python@3.11` | distro package manager (e.g., `apt install python3.11`) | `python3 --version` |
| **Poetry** | `curl -sSL https://install.python-poetry.org \| python3 -` | same as macOS | `poetry --version` |
| **temporal** *(Temporal only)* | `brew install temporal` | [official guide](https://docs.temporal.io/cli#install) | `temporal --version` |

> **Python version note:** Python 3.11 or 3.12 is strongly recommended. Python 3.13+ may fail during `poetry install` because pre-built wheels for some ML dependencies are not yet available for newer interpreter versions.

> **Docker daemon note:** `docker info` (the verify command above) requires the Docker daemon to be running — unlike `docker --version`, which only checks the binary. If `docker info` fails, start Docker Desktop or Colima before continuing.

### Colima resource requirements (macOS only)

If you are on macOS and using Colima as your Docker runtime, the default VM resources are too limited for the sandbox. Start Colima with at least:

```bash
colima start --cpu 4 --memory 8 --disk 60
```

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU cores | 4 | 6 |
| Memory (GB) | 8 | 12 |
| Disk (GB) | 60 | 100 |

> **Warning:** Starting Colima with the default settings (2 CPU, 2 GB RAM) will cause pods to crash or fail to schedule. Always pass explicit resource flags.

If Colima is already running with insufficient resources, stop it and restart with the new settings:

```bash
colima stop
colima start --cpu 4 --memory 8 --disk 60
```

### Configure `host.docker.internal`

Docker containers need to communicate with services on your host machine. Verify this hostname resolves correctly:

1. Open your hosts file: `sudo nano /etc/hosts`
2. Look for this line:
   ```
   127.0.0.1 host.docker.internal
   ```
3. If missing, add it to the end of the file and save.

### Install Python dependencies

From the repository you cloned, install the Michelangelo Python packages:

```bash
cd <repo-root>/python
poetry install
```

---

## Quick start

Once prerequisites and Python dependencies are installed:

```bash
# 1. Activate the Poetry virtual environment (from <repo-root>/python)
source .venv/bin/activate

# 2. Build local-only images required by the sandbox
cd <repo-root>
bash scripts/kuberay/build-kuberay-images.sh

# 3. Create the sandbox (30–60 min on first run; images are cached after)
ma sandbox create

# 4. Verify everything works by running the demo pipeline
ma sandbox demo pipeline
```

> **Tip:** If you prefer not to activate the venv, prefix each `ma` command with `poetry run` (e.g., `poetry run ma sandbox create`). If you see `zsh: command not found: ma`, you either skipped step 1 or need to use `poetry run`. See [troubleshooting](#command-not-found-ma) below.

> **Note on image pull times:** During `ma sandbox create`, several pods will sit in `ContainerCreating` for several minutes while images are pulled (some images are 500–800 MB). This is normal. The relevant question is whether pods are making progress — check with `kubectl get pods -w` and look for status transitions. Only act if a pod stays in `ImagePullBackOff` or `CrashLoopBackOff` for more than a few minutes.

### Choosing a workflow engine

`ma sandbox create` defaults to **Cadence**, which is the recommended choice for most users — it's the most-tested path and matches the examples in this guide. Pass `--workflow temporal` only if you specifically want to develop or test against Temporal (for example, if your team is migrating to it). The two engines are interchangeable from a workflow-author perspective; the choice mainly affects which web UI and CLI you use.

### Verifying success

When `ma sandbox create` completes, all Michelangelo services start in your k3d cluster. Verify with:

```bash
kubectl get pods
```

You should see roughly 10–15 pods (the exact count depends on which engine you chose and any `--exclude` flags). On first run, pods may spend 5–10 minutes in `ContainerCreating` while images are pulled — this is expected. All pods should eventually reach `Running` status; if any remain in `ContainerCreating` beyond 15 minutes, check the pod events with `kubectl describe pod <pod-name>`.

Then open the **Michelangelo UI at [http://localhost:8090](http://localhost:8090)** — if the dashboard loads, your sandbox is healthy. See [Sandbox Ports and Endpoints](./ma-sandbox-ports-and-endpoints.md) for the full list of services and their URLs.

---

## Sandbox commands

The `ma sandbox` command manages your local Kubernetes development environment.

> For a complete command reference, see the [CLI Reference - Sandbox Commands](../user-guides/reference/cli.md#sandbox-commands).

### Lifecycle

The typical sandbox workflow:

```
create → (develop) → stop → start → (develop) → delete
           ↓ (if create fails partway)
          sync → (develop) → ...
```

### Create

```bash
ma sandbox create [OPTIONS]
```

| Flag | Description | Default |
|------|-------------|---------|
| `--workflow cadence\|temporal` | Choose workflow engine | `cadence` |
| `--exclude [services]` | Exclude services: `apiserver`, `controllermgr`, `ui`, `worker`, `prometheus`, `grafana` | none |
| `--create-compute-cluster` | Create an additional Ray compute cluster for distributed jobs | disabled |
| `--compute-cluster-name <name>` | Custom name for the compute cluster | auto-generated |
| `--include-experimental [services]` | Include experimental services | none |

**Examples:**

```bash
# Full sandbox with all services (default: Cadence workflow engine)
ma sandbox create

# Sandbox with Temporal workflow engine
ma sandbox create --workflow temporal

# Sandbox without UI, with a Ray compute cluster
ma sandbox create --exclude ui --create-compute-cluster
```

### Sync

```bash
ma sandbox sync
```

Redeploys services into an existing cluster, skipping cluster creation and image import. Use this to recover from a partially failed `create` — for example, if operator deployments timed out but the cluster itself was created successfully.

If `ma sandbox create` fails and you see `Failed to create cluster ... already exists` when you try again, run `ma sandbox sync` instead. See [Recovering from a failed create](#recovering-from-a-failed-create) below.

### Stop / Start

Pause and resume your sandbox without losing state:

```bash
ma sandbox stop    # preserves state
ma sandbox start   # resume where you left off
```

### Delete

Tear down the cluster and remove all resources:

```bash
ma sandbox delete
```

### Demo

Create pre-configured demo resources for testing:

```bash
ma sandbox demo pipeline    # registers and runs a sample pipeline
ma sandbox demo inference   # sets up demo inference server
```

---

## Recovering from a failed create

If `ma sandbox create` fails partway through — for example, with a Helm timeout on `kuberay-operator` or `spark-operator` — the k3d cluster may already exist even though not all services deployed successfully.

Running `ma sandbox create` again will fail with `Failed to create cluster ... because a cluster with that name already exists`.

**Do not delete and recreate** — that discards the cluster and costs another full image-pull cycle. Instead, use `sync`:

```bash
ma sandbox sync
```

`sync` redeploys services into the existing cluster without recreating it or re-importing images. In most cases this resolves transient operator timeouts without the 30–60 minute penalty of a fresh `create`.

If `sync` doesn't resolve the issue, check pod events with `kubectl describe pod <pod-name>` to understand what failed, then delete and recreate as a last resort:

```bash
ma sandbox delete
ma sandbox create
```

---

## Smoke test: run the BERT CoLA example

After `ma sandbox demo pipeline` succeeds, you've already proven the sandbox works end to end. If you'd like to run a real example workflow against it before moving on, the BERT CoLA text-classification example is a quick way to confirm local execution works:

```bash
cd <repo-root>/python
poetry install --extras example
PYTHONPATH=. poetry run python ./examples/bert_cola/bert_cola.py
```

You should see workflow logs in your terminal and, when it finishes, a trained model artifact written to local storage.

For the full story on local vs. remote execution, building Docker images, configuring storage, and using either workflow engine end to end, see:

- [Pipeline Running Modes](../user-guides/ml-pipelines/pipeline-running-modes.md) — the four execution modes Michelangelo supports

> **Note:** Local execution doesn't support caching, retries, or resource constraints. Use remote execution (covered in the ML Pipelines guides) for production-like behavior.

---

## Troubleshooting

### `command not found: ma`

You either skipped the venv activation step or the Poetry environment isn't active in your current shell. Two options:

```bash
# Option 1: activate the venv (from <repo-root>/python)
source .venv/bin/activate

# Option 2: prefix each command with poetry run
poetry run ma sandbox create
```

If neither works, run `poetry install` from `<repo-root>/python` first to make sure the environment was created.

### `Failed to create cluster ... already exists`

The k3d cluster was created but `ma sandbox create` failed before all services deployed. **Do not delete and recreate** — use `sync` to redeploy services into the existing cluster without re-importing images:

```bash
ma sandbox sync
```

See [Recovering from a failed create](#recovering-from-a-failed-create) for the full recovery flow.

### `history-server` stuck in `ImagePullBackOff`

The `kuberay-historyserver` image is not available in any public registry — it must be built locally before running `ma sandbox create`. If you skipped the build step:

```bash
cd <repo-root>
bash scripts/kuberay/build-kuberay-images.sh
ma sandbox sync   # redeploy without recreating the cluster
```

### `Progress deadline exceeded` from operator installs

During `ma sandbox create`, Helm deploys operators (kuberay, spark) with a timeout. On slow connections or underpowered machines, the timeout can fire before the operator pod finishes pulling its image — even though the deployment will succeed on its own a few minutes later.

This is a **first-run phenomenon** caused by large image pulls racing against Helm's deadline. On subsequent `create` or `sync` runs with cached images, operator deployments complete well within the timeout.

If you see `Progress deadline exceeded` in the output, verify that the operators are still coming up:

```bash
kubectl get pods -A
```

Within a few minutes of the Helm error, you should see the operator deployments transition to `Running`. If they do, the sandbox is healthy despite the error message — run `ma sandbox sync` to finish deploying any remaining services. Only proceed to `ma sandbox delete` if pods are stuck in `ImagePullBackOff` or `CrashLoopBackOff` with no sign of progress.

### Grafana pod in `CrashLoopBackOff`

Grafana's sandbox configuration installs dashboard plugins at startup by fetching them from an external source. On restricted, proxied, or intermittent network connections this fetch can fail, causing a crash loop.

Check if this is the cause:

```bash
kubectl logs <grafana-pod-name> | grep -i "plugin\|install\|GF_INSTALL"
```

If plugin installation is the failure point, you can exclude Grafana from the sandbox and proceed without it:

```bash
ma sandbox sync --exclude grafana
```

Grafana is used for metrics dashboards and is not required for pipeline execution or the Michelangelo UI.
### `ma` CLI commands fail with `connection attempt timed out`

The `ma` CLI connects to the API server over gRPC at `localhost:15566`. The sandbox maps this port via k3d NodePort so it works out of the box after `ma sandbox create`.

If `ma` commands time out, verify the NodePort mapping is active:

```bash
kubectl get svc michelangelo-apiserver -o jsonpath='{.spec.type}'
# Should print "NodePort"
```

The default CLI configuration (`~/.ma/config.toml`) uses `address = "127.0.0.1:15566"`.

### `ModuleNotFoundError: No module named 'grpc_reflection'`

This error occurs when Python dependencies aren't fully installed. Fix it by reinstalling from the `python/` directory:

```bash
cd <repo-root>/python
poetry install
```

If the error persists, try removing the virtual environment and reinstalling:

```bash
rm -rf .venv
poetry install
```

### Pods stuck in `ImagePullBackOff` or `ErrImagePull`

The cluster can't pull a Docker image. Check which image is failing:

```bash
kubectl describe pod <pod-name> | grep -A 5 "Events"
```

Common causes:
- **Network issues**: Ensure Docker can reach `ghcr.io` (try `docker pull ghcr.io/michelangelo-ai/worker:latest`)
- **Local-only image not built**: If `history-server` is the failing pod, see [`history-server` stuck in `ImagePullBackOff`](#history-server-stuck-in-imagepullbackoff) above

### Worker crashes with `Namespace default is not found` (Temporal only)

The Temporal `default` namespace must be registered after the sandbox starts. If the worker is in `CrashLoopBackOff`:

```bash
# Port-forward the Temporal frontend
kubectl port-forward svc/michelangelo-temporal-frontend 7233:7233 &

# Register the default namespace
temporal operator namespace create default

# Restart the worker to pick it up
kubectl rollout restart deployment/michelangelo-worker
```

### Pods stuck in `CrashLoopBackOff`

A service is starting but immediately crashing. Check its logs:

```bash
kubectl logs <pod-name>
```

If a single service is wedged, the simplest recovery is to delete the pod and let Kubernetes recreate it:

```bash
kubectl delete pod <pod-name>
```

If that doesn't help, recreate the sandbox cleanly:

```bash
ma sandbox delete
ma sandbox create
```

### Port already in use

If `ma sandbox create` fails because a port is already bound:

```bash
# Find what's using the port (e.g., port 9090)
lsof -i :9090

# Kill the process if it's safe to do so
kill <PID>
```

See [Sandbox Ports and Endpoints](./ma-sandbox-ports-and-endpoints.md) for the full list of ports used.

### Poetry install fails with build errors on macOS

If you see C++ compilation errors during `poetry install`:

```bash
export CC=clang
export CXX=clang++
poetry install
```

Add those exports to your `~/.zshrc` to make them permanent.

---

## What's next?

- **Build your first pipeline** -- Follow [Getting Started with ML Pipelines](../user-guides/getting-started/getting-started.md) to create a training workflow (~30 min)
- **Explore example projects** -- Try [California Housing XGBoost](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/pipelines/california_housing_xgb), [BERT Text Classification](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/bert_cola), or [GPT Fine-tuning](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/gpt_oss_20b_finetune)
- **Learn the CLI** -- See the [CLI Reference](../user-guides/reference/cli.md) for managing pipelines and projects
