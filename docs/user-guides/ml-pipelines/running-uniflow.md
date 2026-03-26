# Running Uniflow pipelines

This guide covers how to run Uniflow pipelines locally and remotely.

## What you'll learn

* How to set up your environment for local and remote execution
* The differences between local and remote execution modes
* How to debug workflows and container issues

## Environment setup

Create Python virtual environment and install packages:

```bash
cd $REPO_ROOT/python
poetry install
```

This will create a `.venv` directory if it doesn't already exist. This directory contains a Python virtual environment with all the dependencies installed. You can activate this virtual environment and use it like any other Python virtual environment, or you can run commands via the Poetry CLI, e.g., `poetry run python`, `poetry run pytest`, etc.

## Execution modes

Uniflow supports two primary modes of execution: **Local Execution** and **Remote Execution**. Each is suited for different stages of development and deployment.

### Local Execution

Local execution runs workflows directly in a standard Python environment, making it ideal for rapid iteration and debugging.

#### Pros
- Fast feedback loop for development
- Simple to run and test locally

#### Limitations
- **No caching or retries**: Features like caching, retries, and `apply_local_diff` are not supported.
- **No resource constraints**: Configurations for CPU, GPU, memory, and worker instances are ignored.
- **No authentication support**: If your tasks depend on external cloud services (e.g., S3, HDFS, Kubernetes APIs), local mode does not support automatic authentication. Test these interactions in remote environments.

#### Example
```bash
python your_workflow_script.py
```

### Remote Execution

Remote execution deploys workflows to a **Kubernetes** cluster for production-scale workloads, fault tolerance, and reproducibility.

#### Benefits
- Full support for resource constraints (CPU/GPU)
- Caching and retry mechanisms enabled
- Handles large datasets and distributed execution
- Secure cloud access (via service accounts, mounted credentials, etc.)

#### Running a workflow remotely

```bash
PYTHONPATH=. poetry run python ./examples/bert_cola/bert_cola.py remote-run \
  --image docker.io/library/my_image:latest \
  --storage-url s3://<my_bucket_name> \
  --yes
```

Sample Output:

```
Started Workflow Id: examples.bert_cola.bert_cola.train_workflow.97lal
Run Id: 56f90eb2-c570-4926-a1fe-993816cd1baf
```

## Example: BERT-COLA

1. Go to python repo: `cd $REPO_ROOT/python`
2. Install dependencies: `poetry install -E example`
3. See the [BERT-COLA README](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/README.md)

### Local runs

```bash
PYTHONPATH=. poetry run python ./examples/bert_cola/bert_cola.py
```

### Remote runs

Install Cadence for command-line interaction with Cadence workflow:
```bash
brew install cadence-workflow
```

Running workflows in remote mode requires a docker container that contains code of the workflow tasks:

1. Build docker image:
   ```bash
   docker build -t examples:latest -f ./examples/Dockerfile .
   ```
   > Note: you may experience an error with poetry installed via brew. Uninstall from brew and install with curl, then docker build with `--no-cache`

2. Push images to registry:
   ```bash
   k3d image import examples:latest -c michelangelo-sandbox
   ```

3. Create default bucket at http://localhost:9090/buckets (login: minioadmin/minioadmin)

4. Create default domain if not exists:
   ```bash
   cadence --do default d re
   ```

5. Run example:
   ```bash
   PYTHONPATH=. poetry run python ./examples/bert_cola/bert_cola.py remote-run \
     --image docker.io/library/examples:latest \
     --storage-url s3://default \
     --yes
   ```

## Debugging workflows

### Useful URLs
- **Cadence** (workflow status): http://localhost:8088/domains/default/workflows
- **MinIO** (object storage): http://localhost:9090/browser/default
- **Ray Dashboard**: http://localhost:8265

### Accessing Ray Dashboard for failed tasks

1. Set `breakpoint=True` in task to keep Ray cluster running:
   ```python
   @uniflow.task(config=RayTask(
       ...
       breakpoint=True,
   ))
   ```

2. Get the service name:
   ```bash
   kubectl get svc
   ```

3. Port forward to access dashboard:
   ```bash
   kubectl port-forward svc/<service-name> 8265:8265 -n default
   ```

## Debugging container issues

```bash
# Check pod status
kubectl get pods
kubectl get pods -n ray-system
kubectl logs michelangelo-worker
kubectl describe pod michelangelo-worker

# Delete and restart for partial pod failure
kubectl delete pod minio
kubectl apply -f michelangelo/cli/sandbox/resources/minio.yaml

# Test docker pull
docker login ghcr.io -u [your-id] -p [CR_PAT]
docker pull ghcr.io/michelangelo-ai/worker:latest

# Debug container starting issues
docker images
docker exec -it k3d-michelangelo-sandbox-server-0 crictl images
```
