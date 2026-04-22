# Sandbox Setup

Set up a local Michelangelo environment on your machine. This gives you a fully functional cluster with the API server, controller manager, workflow engine, object storage, and all supporting services.

**Time estimate**: ~20 minutes (assuming prerequisites are installed).

## Prerequisites

Before you begin, make sure you have the following installed. Run each verification command to confirm:

| Tool | Install | Verify |
|------|---------|--------|
| **Docker** | [Get Docker](https://docs.docker.com/get-started/get-docker) or [Colima](https://github.com/abiosoft/colima) | `docker --version` |
| **kubectl** | `brew install kubectl` or [official guide](https://kubernetes.io/docs/tasks/tools/#kubectl) | `kubectl version --client` |
| **k3d** | `brew install k3d` | `k3d --version` |
| **Helm** | `brew install helm` or [official guide](https://helm.sh/docs/intro/install/) | `helm version` |
| **Python 3.11+** | [python.org](https://www.python.org/downloads/) | `python3 --version` |
| **Poetry** | `curl -sSL https://install.python-poetry.org \| python3 -` | `poetry --version` |

### Colima resource requirements

If you are using Colima as your Docker runtime, the default VM resources are too limited for the sandbox. Start Colima with at least:

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

From the repository root, install the Michelangelo Python packages:

```bash
cd <repo-root>/python
poetry install
```

> **Tip**: Replace `<repo-root>` with the path where you cloned the Michelangelo repository (e.g., `~/michelangelo`).

---

## Quick start

The fastest way to get a working Michelangelo environment:

```bash
# 1. Install dependencies (from the repository root)
cd <repo-root>/python
poetry install
source .venv/bin/activate

# 2. Create the sandbox (~10-15 min on first run)
ma sandbox create

# 3. Verify everything works by running the demo pipeline
ma sandbox demo pipeline
```

When `ma sandbox create` completes successfully, you should see all Michelangelo services starting up in your K3d cluster. You can verify with:

```bash
kubectl get pods
```

All pods should show `Running` status. See [Sandbox Ports and Endpoints](./ma-sandbox-ports-and-endpoints.md) for the full list of services and their URLs.

---

## Sandbox commands

The `ma sandbox` command manages your local Kubernetes development environment.

> For a complete command reference, see the [CLI Reference - Sandbox Commands](../user-guides/cli.md#sandbox-commands).

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
| `--exclude [services]` | Exclude services: `apiserver`, `controllermgr`, `ui`, `worker` | none |
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

## What's next?

- **Run your first pipeline**: Follow the [Getting Started with ML Pipelines](../user-guides/ml-pipelines/getting-started.md) guide to define tasks, build a workflow, and run it on your sandbox
- **Understand the concepts**: Read [Core Concepts and Key Terms](./core-concepts-and-key-terms.md) to learn how projects, workflows, and deployments fit together
- **Explore the services**: See [Sandbox Ports and Endpoints](./ma-sandbox-ports-and-endpoints.md) for a full list of local URLs for the UI, Ray Dashboard, MinIO, and more

## Running your first workflow

Once your sandbox is running, you can run Uniflow workflows locally or remotely.

### Local execution

Local execution runs workflows directly in your Python environment -- great for rapid development and debugging.

```bash
cd <repo-root>/python
poetry install --extras example
PYTHONPATH=. poetry run python ./examples/bert_cola/bert_cola.py
```

> **Note**: Local execution doesn't support caching, retries, or resource constraints. Use remote execution for production-like behavior.

### Remote execution

Remote execution deploys workflows to your sandbox's Kubernetes cluster, with full caching, retries, and resource management.

**Setup:**

1. Build a Docker image with your workflow code:
   ```bash
   cd <repo-root>/python
   docker build -t examples:latest -f ./examples/Dockerfile .
   ```

2. Import the image into your K3d cluster:
   ```bash
   k3d image import examples:latest -c michelangelo-sandbox
   ```

3. Set up MinIO storage (object storage for workflow artifacts):
   - Open the MinIO Console at http://localhost:9090
   - Log in with username `minioadmin` and password `minioadmin` (these are default sandbox credentials, not for production use)
   - Click "Create Bucket" and create a bucket named `default`

4. Set up the Cadence workflow domain (if using Cadence):
   ```bash
   brew install cadence-workflow
   cadence --do default d re
   ```

5. Run your workflow:
   ```bash
   PYTHONPATH=. poetry run python ./examples/bert_cola/bert_cola.py \
     remote-run \
     --image docker.io/library/examples:latest \
     --storage-url s3://default \
     --yes
   ```

**Monitoring your workflow:**

| Service | URL | What to check |
|---------|-----|---------------|
| Cadence Web UI | http://localhost:8088/domains/default/workflows | Workflow status and history |
| MinIO Console | http://localhost:9090/browser/default | Stored artifacts and data |
| Ray Dashboard | http://localhost:8265 | Ray task execution (requires port-forward, see below) |

To access the Ray Dashboard for tasks running in the cluster:

1. Find the Ray head service: `kubectl get svc | grep ray`
2. Port-forward it: `kubectl port-forward svc/<ray-head-svc-name> 8265:8265 -n default`

For more details on execution modes, see [Pipeline Running Modes](../user-guides/ml-pipelines/pipeline-running-modes.md).

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

### Pods stuck in `CrashLoopBackOff`

A service is starting but immediately crashing. Check its logs:

```bash
kubectl logs <pod-name>
```

To restart a single service (e.g., MinIO):

```bash
kubectl delete pod minio
kubectl apply -f <repo-root>/python/michelangelo/cli/sandbox/resources/minio.yaml
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

- **Build your first pipeline** -- Follow [Getting Started with ML Pipelines](../user-guides/ml-pipelines/getting-started.md) to create a training workflow (~30 min)
- **Explore example projects** -- Try [Boston Housing XGBoost](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/boston_housing_xgb), [BERT Text Classification](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/bert_cola), or [GPT Fine-tuning](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/gpt_oss_20b_finetune)
- **Learn the CLI** -- See the [CLI Reference](../user-guides/cli.md) for managing pipelines and projects
