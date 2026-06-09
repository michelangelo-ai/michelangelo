---
sidebar_position: 3
---

# Sandbox Setup

This guide walks you through setting up a local Michelangelo environment on your laptop. The sandbox runs a fully functional cluster — API server, controller manager, workflow engine, object storage, and supporting services — entirely on your machine, so you can explore Michelangelo or develop against it without any cloud infrastructure.

**Who this is for:** ML engineers, platform engineers, and contributors who want to try Michelangelo locally or develop new features against it.

**What you'll have at the end:** a running sandbox cluster and a successful demo pipeline run, ready for you to build your own workflows on top of.

**Time estimate:** ~20 minutes (assuming prerequisites are installed; first-time pulls of container images can add 5–10 minutes).

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
| **Docker** | [Docker Desktop](https://docs.docker.com/get-started/get-docker) or [Colima](https://github.com/abiosoft/colima) | [Docker Engine](https://docs.docker.com/engine/install/) | `docker --version` |
| **kubectl** | `brew install kubectl` | [official guide](https://kubernetes.io/docs/tasks/tools/#kubectl) | `kubectl version --client` |
| **k3d** | `brew install k3d` | [official guide](https://k3d.io/#installation) | `k3d --version` |
| **Helm** | `brew install helm` | [official guide](https://helm.sh/docs/intro/install/) | `helm version` |
| **Python 3.9+** | [python.org](https://www.python.org/downloads/) or `brew install python@3.11` | distro package manager (e.g., `apt install python3`) | `python3 --version` |
| **Poetry** | `curl -sSL https://install.python-poetry.org \| python3 -` | same as macOS | `poetry --version` |
| **temporal** *(Temporal only)* | `brew install temporal` | [official guide](https://docs.temporal.io/cli#install) | `temporal --version` |

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

Once prerequisites and Python dependencies are installed, the sandbox is three commands away:

```bash
# 1. Activate the Poetry virtual environment (from <repo-root>/python)
source .venv/bin/activate

# 2. Create the sandbox (~10–15 min on first run)
ma sandbox create

# 3. Verify everything works by running the demo pipeline
ma sandbox demo pipeline
```

> **Tip:** If you prefer not to activate the venv, you can prefix each command with `poetry run` (e.g., `poetry run ma sandbox create`).

### Choosing a workflow engine

`ma sandbox create` defaults to **Cadence**, which is the recommended choice for most users — it's the most-tested path and matches the examples in this guide. Pass `--workflow temporal` only if you specifically want to develop or test against Temporal (for example, if your team is migrating to it). The two engines are interchangeable from a workflow-author perspective; the choice mainly affects which web UI and CLI you use.

### Verifying success

When `ma sandbox create` completes, all Michelangelo services start in your k3d cluster. Verify with:

```bash
kubectl get pods
```

You should see roughly 10–15 pods (the exact count depends on which engine you chose and any `--exclude` flags). All pods should reach `Running` status within 2–3 minutes; some pods may briefly show `ContainerCreating` or `Init` while images pull.

Then open the **Michelangelo UI at [http://localhost:8090](http://localhost:8090)** — if the dashboard loads, your sandbox is healthy. See [Sandbox Ports and Endpoints](./ma-sandbox-ports-and-endpoints.md) for the full list of services and their URLs.

---

## Sandbox commands

The `ma sandbox` command manages your local Kubernetes development environment.

> For a complete command reference, see the [CLI Reference - Sandbox Commands](../user-guides/reference/cli.md#sandbox-commands).

### Lifecycle

The typical sandbox workflow:

```
create → (develop) → stop → start → (develop) → delete
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
- **Image doesn't exist**: Verify the image tag matches what's available in the registry

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
- **Explore example projects** -- Try [California Housing XGBoost](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/california_housing_xgb), [BERT Text Classification](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/bert_cola), or [GPT Fine-tuning](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/gpt_oss_20b_finetune)
- **Learn the CLI** -- See the [CLI Reference](../user-guides/reference/cli.md) for managing pipelines and projects
