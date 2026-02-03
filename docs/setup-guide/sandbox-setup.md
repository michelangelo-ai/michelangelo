# Sandbox Setup

## Prerequisites

### Required Software

This guide assumes you have the following software installed and configured on your system. Please follow the instructions below for each prerequisite.

* [Docker](https://docs.docker.com/get-started/get-docker)
* [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
* [k3d](https://k3d.io)

#### 1. Docker

Please follow the official Docker installation guide for your operating system: [Official Docker Documentation](https://docs.docker.com/get-started/get-docker)

Alternatively, we can use [Colima](https://github.com/abiosoft/colima) for starting the docker runtime.

**Important Configuration: Accessing Your Host from Docker Containers (`host.docker.internal`)**

Docker often requires containers to communicate with services running directly on your host machine (your laptop or development server). To facilitate this, Docker provides a special hostname: `host.docker.internal`. This name resolves to your host's internal IP address (typically `127.0.0.1`).

**Verification and Configuration:**

1.  **Open your system's `hosts` file**: Open your terminal and run: `sudo nano /etc/hosts` (or use your preferred text editor).

2.  **Check for the entry:** Look for a line similar to:

    ```
    127.0.0.1 host.docker.internal
    ```

3.  **Add the entry if missing:** If you don't find this line, add it to the end of the file.

**Why is this important?**

Ensuring this entry exists allows containers managed by Docker (including the Kubernetes nodes created by `k3d`) to easily connect back to services running on your local development machine using the consistent `host.docker.internal` address.

#### 2. kubectl

`kubectl` is the command-line tool for interacting with Kubernetes clusters. You will use it to manage and inspect your `k3d` cluster.

**Installation:**

Follow the official Kubernetes documentation for installing `kubectl`: [Install kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)

```bash
brew install kubectl
```

#### 3. k3d

`k3d` is a lightweight tool to run local Kubernetes clusters in Docker. It simplifies the process of setting up a Kubernetes environment for development and testing.

**Installation:**

```bash
brew install k3d
```

### GitHub Personal Access Token

Michelangelo is not publicly available yet, so we keep Michelangelo's Docker containers in the private GitHub Container
Registry, which requires a [GitHub personal access token (classic)](https://github.com/settings/tokens) for authentication.

To enable authentication for the sandbox, please create a GitHub personal access token (classic) with the
"read:packages" scope and save it to the `CR_PAT` environment variable. For example, you can add the following line to
your shell configuration file (such as `.bashrc` or `.zshrc`, depending on the shell you use):

```bash
$ export CR_PAT=your_token_...
$ echo 'export CR_PAT=your_token_...' >> ~/.zshrc
$ source ~/.zshrc

# login before running ma sandbox so that MA docker image can be pulled
$ docker login ghcr.io -u [your github id] -p [CR_PAT]
```

For a more detailed guide, please refer
to https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry#authenticating-with-a-personal-access-token-classic.

> Be aware that `CR_PAT` environment variable is required while Michelangelo is NOT publicly accessible. Once we become
> public, the token will no longer be necessary, and this section will be removed.

### Python Environment

This project requires Python version 3.9 or higher to run certain scripts and tools.

**Installation:**

Please download and install Python 3.9+ from the [official Python downloads page](https://www.python.org/downloads/).

**Verification:**

Open your terminal or command prompt and verify the installed Python version. The command might vary slightly depending on your system:

```bash
python3 --version
# or
python --version
```
The output should display a version number that starts with 3.9 or a higher minor or patch version (e.g., Python 3.9.x, Python 3.10.y).

#### Poetry - Python Dependency Management

Poetry is used to manage the project's Python dependencies, ensuring that you have the correct versions of all necessary libraries for development and running Python-based tools.

**Installation:**

Follow the official Poetry installation guide for your operating system: [Poetry Installation](https://python-poetry.org/docs/#installing-with-the-official-installer). Michelangelo recommends running the official installer script:

```bash
curl -sSL https://install.python-poetry.org | python3 -
```

Make sure to follow the instructions provided during the installation process, which might include adding Poetry's bin directory (e.g., ~/.local/bin on Linux/macOS) to your system's PATH environment variable so you can run the poetry command globally.

**Verification:**

Open a new terminal or command prompt and check the installed Poetry version:

```bash
poetry --version
```

This command should output the installed Poetry version number.

**Install dependencies**

```bash
poetry install
```

This command should install all the dependencies from pyproject.toml.

```bash
cd $REPO_ROOT/python
poetry install -E ma
```

if you see the following error when setting up sandbox

```python
Traceback (most recent call last):
  File "<string>", line 1, in <module>
  File "/Users/frank.chen.cst/.pyenv/versions/3.9.22/lib/python3.9/importlib/__init__.py", line 127, in import_module
    return _bootstrap._gcd_import(name[level:], package, level)
  File "<frozen importlib._bootstrap>", line 1030, in _gcd_import
  File "<frozen importlib._bootstrap>", line 1007, in _find_and_load
  File "<frozen importlib._bootstrap>", line 986, in _find_and_load_unlocked
  File "<frozen importlib._bootstrap>", line 680, in _load_unlocked
  File "<frozen importlib._bootstrap_external>", line 850, in exec_module
  File "<frozen importlib._bootstrap>", line 228, in _call_with_frames_removed
  File "/Users/frank.chen.cst/Desktop/michelangelo/python/michelangelo/cli/cli.py", line 5, in <module>
    from michelangelo.cli.ma import ma
  File "/Users/frank.chen.cst/Desktop/michelangelo/python/michelangelo/cli/ma/ma.py", line 39, in <module>
    from grpc_reflection.v1alpha import reflection_pb2, reflection_pb2_grpc
ModuleNotFoundError: No module named 'grpc_reflection'
```

## Running Michelangelo's API sandbox environment

```bash
cd $REPO_ROOT/python
poetry run ma sandbox --help
poetry run ma sandbox create
```

For creating for Temporal Workflow Engine

```bash
poetry run ma sandbox create --workflow temporal
```

or

```bash
cd $REPO_ROOT/python
poetry install
source .venv/bin/activate
ma sandbox --help
ma sandbox create
```

For creating for Temporal Workflow Engine

```bash
ma sandbox create --workflow temporal
```

### Debugging container issue in Sandbox

Test docker pull issues

```bash
# Check pod status
kubectl get pods
kubectl get pods -n ray-system
kubectl logs michelangelo-worker
kubectl describe pod michelangelo-worker

# Delete and start for the partial pod failure
# Example for minio
kubectl delete pod minio
kubectl apply -f michelangelo/cli/sandbox/resources/minio.yaml

# Test docker pull
docker login ghcr.io -u [your id] -p [CR_PAT]
docker pull ghcr.io/michelangelo-ai/worker:sha-6161efe@sha256:aae98f00b82d744e453432a9008027fc74d44b78cc1731cb995c7ee654a8225d

# Debugging container starting issue
docker images
docker exec -it k3d-michelangelo-sandbox-server-0 crictl images
```

## Running Michelangelo Uniflow

**Environment Setup: Mac**

* Create Python virtual environment and install packages

```bash
cd $REPO_ROOT/python
poetry install
```

This will create a .venv directory if it doesn't already exist. This directory contains a Python virtual environment with all the dependencies installed. You can activate this virtual environment and use it like any other Python virtual environment, or you can run commands via the Poetry CLI, e.g., poetry run python, poetry run pytest, etc.

### Execution Modes

Uniflow supports two primary modes of execution: **Local Execution** and **Remote Execution**. Each is suited for different stages of development and deployment.

#### Local Execution

Local execution runs workflows directly in a standard Python environment, making it ideal for rapid iteration and debugging.

##### Pros

* Fast feedback loop for development
* Simple to run and test locally

##### Limitations

* **No Caching or Retries**
  Features like caching, retries, and `apply_local_diff` are not supported.
* **No Resource Constraints**
  Configurations for CPU, GPU, memory, and worker instances are ignored.
* **No Authentication Support**
  If your tasks depend on external cloud services (e.g., S3, HDFS, Kubernetes APIs), local mode does not support automatic authentication. Test these interactions in remote environments.

##### Example

```bash
python your_workflow_script.py
```

#### Remote Execution

Remote execution deploys workflows to a **Kubernetes** cluster for production-scale workloads, fault tolerance, and reproducibility.

##### Benefits

* Full support for resource constraints (CPU/GPU)
* Caching and retry mechanisms enabled
* Handles large datasets and distributed execution
* Secure cloud access (via service accounts, mounted credentials, etc.)

##### Running a Workflow Remotely

```bash
PYTHONPATH=. poetry run python ./examples/bert_cola/bert_cola.py remote-run \
  --image docker.io/library/my_image:latest \
  --storage-url s3://<my_bucket_name> \
  --yes
```

Sample Output:

```sh
.com/cadence-workflow/starlark-worker/cadstar.(*Service).Run
--execution_timeout 315360000
--workflow_id examples.bert_cola.bert_cola.train_workflow.97lal

Started Workflow Id: examples.bert_cola.bert_cola.train_workflow.97lal
Run Id: 56f90eb2-c570-4926-a1fe-993816cd1baf
```

### Run example: bert cola

* Go to python repo: `cd $REPO_ROOT/python`
* Install dependencies: `poetry install -E example`
* README: [bert cola README.md](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/README.md)

**Local runs**

* Run examples: `PYTHONPATH=. poetry run python ./examples/bert_cola/bert_cola.py`

**Remote runs**

Install Cadence for command-line interaction with cadence workflow

```bash
brew install cadence-workflow
```

Running workflows in the remote mode requires a docker container that contains code of the workflow tasks. Build a new revision of the project's container, or use an existing revision if you didn't change task code.

* Build docker image: `docker build -t examples:latest -f ./examples/Dockerfile .`
  * Note: you may experience an error with poetry that installed with brew, please uninstall in brew and install with curl above and docker build with option `--no-cache`

In order for Kubernetes to pull the image, push it to a registry that the cluster has access to. For example, push it to

* Push images to registry `k3d image import examples:latest -c michelangelo-sandbox`
* Create default bucket http://localhost:9090/buckets, login as minioadmin and password as minioadmin, click "Create Bucket" and create a bucket with the name default.
* Create default domain http://localhost:8088/domains/default/ if not exists in Cadence, `cadence --do default d re`
* Run example: `PYTHONPATH=. poetry run python ./examples/bert_cola/bert_cola.py  remote-run --image docker.io/library/examples:latest --storage-url s3://default --yes`

Debugging workflow running in the cluster

* Cadence for workflow status - http://localhost:8088/domains/default/workflows
* Minio for object storage - http://localhost:9090/browser/default
* Ray cluster dashboard - http://localhost:8265
  * to access the Ray cluster dashboard for the failed ray task, you need to port forward the Kubernetes service.
    1. Set the breakpoint = True in task to keep run ray cluster.
    ```python
    # example/bert_cola/data.py or train.py
    @uniflow.task(config=RayTask(
       ...
       breakpoint=True,
    ))
    ```
    2. Run `kubectl get svc` to get the service name and copy it
    ```sh
    NAME                     TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)                                         AGE
    ...
    uf-ray-td7pb-head-svc    ClusterIP   None            <none>        10001/TCP,8265/TCP,8080/TCP,6379/TCP,8000/TCP   62s
    ```
    3. Run `kubectl port-forward svc/<service name> 8265:8265 -n default`
