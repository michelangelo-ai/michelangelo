**Purpose**: Run a Uniflow pipeline that schedules Ray jobs on the compute Kubernetes cluster `michelangelo-compute-0`.

### Prerequisites
- **Repository**: Local checkout with `$REPOROOT` pointing to the repo root
- **Tooling**: `poetry`, `docker`, `k3d`
- **Storage**: Access to `s3://default` (or update the `--storage-url`)

### Procedure
1) Change to the Python workspace:

```bash
cd $REPOROOT/python
```

2) Create the Michelangelo sandbox and compute cluster:

```bash
poetry run ma sandbox create --create-compute-cluster
```

Note: This provisions a local k3d cluster named `michelangelo-compute-0`.

3) Build the example image:

```bash
docker build -t examples:latest -f ./examples/Dockerfile .
```

4) Import the image into the k3d cluster:

```bash
k3d image import examples:latest -c michelangelo-compute-0
```

5) Launch the pipeline (remote run) against the compute cluster:

```bash
PYTHONPATH=. poetry run python ./examples/bert_cola/bert_cola.py remote-run \
  --image docker.io/library/examples:latest \
  --storage-url s3://default \
  --yes
```

**Outcome**:
- Sandbox and compute K8s cluster are created
- The `examples:latest` image is available in `michelangelo-compute-0`
- The `bert_cola` pipeline runs Ray jobs on the compute cluster
