Michelangelo SDK

## User Guide

Install core packges only

```
pip install michelangelo
```

Install with bundle plugins

```
pip install michelangelo[plugin]
```

## User Guide

### Michelangelo Python Client

1. Set Michelangelo API server address using environment variable `MICHELANGELO_API_SERVER`. e.g.

```bash
export MICHELANGELO_API_SERVER="localhost:12345"
```

2. Access API resources using Michelangelo Python client.

```python
from michelangelo.api.v2.client import APIClient
from michelangelo.gen.api.v2.project_pb2 import Project

APIClient.set_caller('my-client') # initialize client with caller name
# If not specified in Python code, the client will use the Michelangelo API server
# address from MICHELANGELO_API_SERVER environment variable.

# list existing projects
projects = APIClient.ProjectService.list_project(namespace='default')
print("Existing projects:")
print(projects)

# create a new project
proj = Project()
proj.metadata.namespace = "default"
proj.metadata.name = "demo-project"
proj.spec.tier = 2
proj.spec.description = "demo project"
proj.spec.owner.owning_team = "8D8AC610-566D-4EF0-9C22-186B2A5ED793"
proj.spec.git_repo = "https://github.com/michelangelo-ai/michelangelo"
proj.spec.root_dir = "/demo-project"
APIClient.ProjectService.create_project(proj)

# get the new project
project = APIClient.ProjectService.get_project(namespace='default', name='demo-project')
print(project)
```

## Developer Guide

### Preprequisites

- Python 3.9
- Poetry: https://python-poetry.org

### Cheat Sheet

- Install dependencies: `poetry install`
  - Install dependencies with plugins: `poetry install -E plugin`
- Add a new dependency: `poetry add <package-name>`
- Run tests: `poetry run pytest`
- Run examples:
  - Install dependencies for example (ML libs for BERT model): `poetry install -E example`
  - Run example: `poetry run python ./examples/bert_cola/bert_cola.py`
- Format code: `poetry run ruff format .`
- Run Michelangelo CLI: `poetry run ma --help`

### Environment Setup: Mac

- Install Python 3.9: `brew install python@3.9`
- Install Poetry: `curl -sSL https://install.python-poetry.org | python3.9 -`
- Create Python virtual environment and install packages: `poetry install`

The last step will create a `.venv` directory if it doesn't already exist.
This directory contains a Python virtual environment with all the dependencies installed.
You can activate this virtual environment and use it like any other Python virtual environment,
or you can run commands via the Poetry CLI, e.g., `poetry run python`, `poetry run pytest`, etc.

### Dockerfile

If you experience issues with the Python environment, you can use the Dockerfile to build a Docker image with the necessary dependencies.
We recommend to setup your local environment with the instructions above (with poetry), but you can use the Dockerfile as a fallback.

```bash
$ cd $REPO_ROOT/python
$ docker build -t $IMAGE_NAME -f ./examples/bert_cola/Dockerfile .
$ docker run $IMAGE_NAME
```

### Regenerate Python gRPC client code

After protobuf file changes, use the following script to regenerate the Python gRPC client code.
tools/gen-grpc-client.sh --clients python
