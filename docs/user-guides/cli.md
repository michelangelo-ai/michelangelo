# CLI Reference

The Michelangelo CLI (`ma`) provides a unified way to manage resources using standard Kubernetes-style commands. This guide covers all supported commands for managing Michelangelo API entities.

## Command summary

All resource types support `get`, `apply`, and `delete` (see [supported resource types](#supported-resource-types) below). Additional commands:

| Command | Description |
|---------|-------------|
| `ma pipeline run` | Execute a registered pipeline |
| `ma pipeline dev-run` | Run a pipeline without registering it |
| `ma pipeline_run kill` | Terminate a running pipeline run |
| `ma trigger_run kill` | Terminate a running trigger |
| `ma sandbox create` | Set up a local development environment |
| `ma sandbox delete` | Tear down the local environment |

### Supported resource types

| Resource Type | CLI Name | Description | Supported Operations |
|---------------|----------|-------------|----------------------|
| Project | `project` | Namespace and team ownership for ML resources | get, apply, delete |
| Pipeline | `pipeline` | Registered workflow with configuration and scheduling | get, apply, delete, run, dev-run |
| PipelineRun | `pipeline_run` | Single execution instance of a pipeline | get, apply, delete, kill |
| TriggerRun | `trigger_run` | Scheduled or on-demand pipeline execution trigger | get, apply, delete, kill |
| Model | `model` | Trained model artifact with versioning | get, apply, delete |
| ModelFamily | `model_family` | Group of related model versions | get, apply, delete |
| Deployment | `deployment` | Model serving deployment configuration | get, apply, delete |
| InferenceServer | `inference_server` | Runtime server for model inference | get, apply, delete |
| Revision | `revision` | Versioned snapshot of a resource | get, apply, delete |
| Cluster | `cluster` | Kubernetes cluster configuration | get, apply, delete |
| RayCluster | `ray_cluster` | Ray distributed compute cluster | get, apply, delete |
| RayJob | `ray_job` | Job submitted to a Ray cluster | get, apply, delete |
| SparkJob | `spark_job` | Job submitted to a Spark cluster | get, apply, delete |
| CachedOutput | `cached_output` | Cached task output for pipeline resume | get, apply, delete |

## Prerequisites

1. **Install [Python >= 3.9](https://www.python.org/downloads/)** and **[Poetry](https://python-poetry.org/docs/#installation)**.

2. **Install dependencies:**

   ```bash
   cd python/
   poetry install
   ```

3. **Start the sandbox environment.** Follow the [Sandbox Setup Guide](../getting-started/sandbox-setup.md) to install the required software (Docker, kubectl, k3d) and create a local development environment:

   ```bash
   ma sandbox create
   ```

   This starts all required services, including the API server (`localhost:15566`), database, workflow engine, and object storage. See [Sandbox commands](#sandbox-commands) for the full command reference.

4. **(Optional) Configure a custom API server address:**

   ```bash
   export MACTL_ADDRESS="127.0.0.1:15566"
   ```

   The default address (`127.0.0.1:15566`) works automatically with the sandbox. Only set this if you are connecting to a different API server instance.

## Usage

All Michelangelo API entities support the following standard operations -- GET, APPLY, and DELETE

### General syntax

```bash
cd $REPO_ROOT/python/
ma <RESOURCE_TYPE> <COMMAND> [ARGS]
```

We will abstract this part like `ma <RESOURCE_TYPE> <COMMAND>` in below.

### GET - Retrieve resource

Retrieve information about an existing resource by namespace and name. If you don't specify the `--name` field, it lists all resources under the specified namespace.

Syntax:

```bash
ma <RESOURCE_TYPE> get --namespace="<namespace>" [--name="<name>"]
# Short form: -n for --namespace
ma <RESOURCE_TYPE> get -n "<namespace>" [--name="<name>"]
```

Examples:

```bash
# List all projects
ma project get --namespace="ma-dev-test"

# List all pipelines in a namespace
ma pipeline get --namespace="ma-dev-test"

# Get a specific pipeline
ma pipeline get --namespace="ma-dev-test" --name="bert-cola-test"

# Get a specific project
ma project get --namespace="ma-dev-test" --name="my-project"

# Get a pipeline run
ma pipeline_run get --namespace="ma-dev-test" --name="run-001"
```

#### Arguments

The following argument is available for list operations (get command without `--name`):

- `--limit [n]` - maximum number of results to return (default: 100)

### APPLY - Create or update a resource from YAML

Apply (create or update) a resource from a YAML configuration file. The `apply` command works as an upsert: it creates the resource if it doesn't exist, or updates it if it does. The resource type is automatically detected from the `apiVersion` and `kind` fields in the YAML.

Syntax:

```bash
ma <RESOURCE_TYPE> apply --file="<YAML_FILE_PATH>"
# Short form: -f for --file
ma <RESOURCE_TYPE> apply -f "<YAML_FILE_PATH>"
```

Examples:

```bash
# Apply a pipeline configuration
ma pipeline apply --file="./examples/bert_cola/pipeline.yaml"

# Apply a project configuration
ma project apply --file="./project.yaml"
```

### DELETE - Remove a resource

Delete a specific resource by namespace and name.

Syntax:

```bash
ma <RESOURCE_TYPE> delete --namespace="<namespace>" --name="<name>"
# Short form: -n for --namespace
ma <RESOURCE_TYPE> delete -n "<namespace>" --name="<name>"
```

Examples:

```bash
# Delete a pipeline
ma pipeline delete --namespace="ma-dev-test" --name="bert-cola-test"

# Delete a project
ma project delete --namespace="ma-dev-test" --name="my-project"

# Delete a pipeline run
ma pipeline_run delete --namespace="ma-dev-test" --name="run-001"
```

## Type-specific commands

Some resource types support additional commands beyond GET, APPLY, and DELETE.

### Pipeline

#### RUN - Execute a pipeline

The RUN command is specifically available for pipelines to create and execute pipeline runs. To run a pipeline, you need to register your pipeline first using `ma pipeline apply -f <pipeline_conf.yaml>`.

Syntax:

```bash
ma pipeline run --namespace="<namespace>" --name="<pipeline_name>"
# Short form: -n for --namespace
ma pipeline run -n "<namespace>" --name="<pipeline_name>"
```

Example:

```bash
# Run a registered pipeline
ma pipeline run --namespace="ma-dev-test" --name="bert-cola-test"
```

##### Arguments

- `--resume_from` - create resumed pipeline run from specified pipeline run (specifying resume_from step is optional)

##### Resume_From Argument

The RUN command also can have a `--resume_from` argument that allows a new pipeline run to be resumed from a previous pipeline line run. If a pipeline run step is not specified in the resume_from argument, the resumed pipeline will automatically resume from the last failed step of the previous pipeline.

Syntax:

```bash
ma pipeline run --namespace="<namespace>" --name="<pipeline_name>" --resume_from=<pipeline_run_name>:<pipeline_run_step_name>
```

Example:

```bash
ma pipeline run --namespace="ma-dev-test" --name="bert-cola-test" --resume_from=run-1759873504-b93b7f612:train
```

#### DEV RUN - Execute a pipeline in DEV mode

The DEV RUN command is used to run a pipeline without registering it. This command is to allow users to quickly iterate on their pipelines. The dev-run command supports an `--env` flag for passing environment variables, which are injected into the pipeline's execution environment.

Syntax:

```bash
ma pipeline dev-run --file=<YAML_FILE_PATH> --env=<ENV_VAR>=<ENV_VAL>
# Short form: -f for --file
ma pipeline dev-run -f <YAML_FILE_PATH> --env=<ENV_VAR>=<ENV_VAL>
```

##### Arguments

- `--file` / `-f` - path to the pipeline YAML configuration file (required)
- `--env` - environment variable to inject (repeatable for multiple variables)
- `--file-sync` - sync uncommitted local file changes to the remote container
- `--storage-url` - custom storage URL for file-sync tarballs (e.g., `s3://bucket/path`)
- `--resume_from` - resume from a previous pipeline run, optionally specifying a step (`<run_name>:<step_name>`)

Example:

```bash
# Run a pipeline in dev mode
ma pipeline dev-run -f "./examples/bert_cola/pipeline.yaml" --env=foo=bar

# To pass in multiple environment variables:
ma pipeline dev-run -f "./examples/bert_cola/pipeline.yaml" --env=foo=bar --env=lorem=ipsum --env=key=val
```

##### Dev-run command with local file sync

Adding `--file-sync` to the dev-run command enables testing of uncommitted code changes without needing to commit or rebuild Docker images.

```bash
# Run a pipeline in dev mode with file sync
ma pipeline dev-run -f "./examples/bert_cola/pipeline.yaml" --env=foo=bar --file-sync

# With custom storage URL
ma pipeline dev-run -f "./examples/bert_cola/pipeline.yaml" --file-sync --storage-url=s3://my-bucket/workflows
```

##### Differences between dev-run and remote-run

**1. dev-run: Test Pipeline from Local File**

`pipeline dev-run` command runs a pipeline directly from your committed git snapshot. Pipeline run will be controlled by Michelangelo API server and controller. This command creates a PipelineRun entity but no Pipeline entity, so you will not see the pipeline entity information in MA Studio.

**remote-run** (invoked via `python my_workflow.py remote-run`, not an `ma` command) bypasses the Michelangelo API server and submits your workflow directly to Cadence/Temporal. No Michelangelo entities are created, and pipeline status is not visible in MA Studio.

**2. dev-run --file-sync: Test Pipeline + Uncommitted Changes**

Adding `--file-sync` to the `pipeline dev-run` command enables testing of uncommitted code changes without needing to commit or rebuild Docker images.

**dev-run --file-sync**: Creates two tarballs: a workflow tarball (from committed code) and a file-sync tarball (containing only files changed via `git diff`). When the container starts, `sitecustomize.py` downloads the file-sync tarball and overlays changed files on top of the base code. The file-sync URL is passed via the `UF_FILE_SYNC_TARBALL_URL` environment variable.

**remote-run**: Creates a workflow tarball from committed code and sends it straight to the workflow engine without creating any Michelangelo entities.

**remote-run --file-sync**: Creates two tarballs: a workflow tarball (base64-encoded in the Cadence CLI input) and a file-sync tarball (uploaded to S3). The S3 URL is passed as an environment variable to the container.

### Pipeline_run

#### Kill - Terminate a pipeline run

The KILL command is used to cleanly terminate a running pipeline. It sets the PipelineRun status to "killed" and aborts the pipeline execution in Cadence/Temporal. The command will prompt for confirmation unless the `--yes` flag is provided.

Syntax:

```bash
ma pipeline_run kill --namespace=<NAMESPACE> --name=<NAME> [--yes]
```

Parameters:

- `--namespace`: Kubernetes namespace where the pipeline run exists
- `--name`: Name of the pipeline run to kill
- `--yes`: (Optional) Skip confirmation prompt and kill immediately

Example:

```bash
# Kill a pipeline run with confirmation prompt
ma pipeline_run kill --namespace=ma-dev-test --name=pipeline-run-20251118-194500-8cdb1538

# Kill a pipeline run without confirmation prompt
ma pipeline_run kill --namespace=ma-dev-test --name=pipeline-run-20251118-194500-8cdb1538 --yes
```

### Trigger_run

#### Kill - Terminate a running trigger

The KILL command is used to cleanly terminate a running trigger_run resource. This command sets the trigger's kill flag, which triggers proper Cadence workflow termination. The command will prompt for confirmation unless the --yes flag is provided.

Syntax:

```bash
ma trigger_run kill --namespace=<NAMESPACE> --name=<NAME> [--yes]
```

Example:

```bash
# Kill a trigger run with confirmation prompt
ma trigger_run kill --namespace=ma-dev-test --name=training-pipeline-cron-trigger

# Kill a trigger run without confirmation prompt
ma trigger_run kill --namespace=ma-dev-test --name=training-pipeline-cron-trigger --yes
```

## Sandbox commands

The `ma sandbox` commands manage a local K3d development environment. For prerequisites, setup walkthrough, and detailed options, see the [Sandbox Setup Guide](../getting-started/sandbox-setup.md).

| Command | Description |
|---------|-------------|
| `ma sandbox create` | Create a K3d cluster with all Michelangelo services |
| `ma sandbox create --workflow temporal` | Create with Temporal instead of Cadence |
| `ma sandbox create --exclude ui` | Create without specific services |
| `ma sandbox create --create-compute-cluster` | Create with a Ray compute cluster |
| `ma sandbox delete` | Tear down the cluster and all resources |
| `ma sandbox start` | Start a stopped cluster |
| `ma sandbox stop` | Stop the cluster (preserves state) |
| `ma sandbox demo pipeline` | Create demo pipeline resources |
| `ma sandbox demo inference` | Create demo inference server resources |

## YAML Resource Examples

### Pipeline YAML

```yaml
apiVersion: michelangelo.api/v2
kind: Pipeline
metadata:
  namespace: "ma-dev-test"
  name: "my-pipeline"
spec:
  type: "PIPELINE_TYPE_TRAIN"
  manifest:
    filePath: examples.bert_cola.bert_cola
```

### Project YAML

```yaml
apiVersion: michelangelo.api/v2
kind: Project
metadata:
  name: ma-dev-test
  namespace: ma-dev-test
spec:
  description: My ML Project
  owner:
    owningTeam: "michelangelo"
    owners:
      - craig.marker
  tier: 4
  gitRepo: https://github.com/uber/michelangelo
  rootDir: python/michelangelo/cli/sandbox/crds
```

### PipelineRun YAML

```yaml
apiVersion: michelangelo.api/v2
kind: PipelineRun
metadata:
  name: run-training-pipeline
  namespace: ma-dev-test
spec:
  pipeline:
    name: training-pipeline
    namespace: ma-dev-test
```

## Configuration

The `ma` CLI uses a layered configuration system. Settings are resolved in the following priority order (highest to lowest):

1. **Environment variables** (highest priority)
2. **TOML config file** (`~/.ma/config.toml`)
3. **Default values** (lowest priority)

### Configuration file

The configuration file is located at `~/.ma/config.toml` and uses TOML format.

#### Example configuration

```toml
[ma]
address = "127.0.0.1:15566"
use_tls = false

[minio]
access_key_id = "minioadmin"
secret_access_key = "minioadmin"
endpoint_url = "http://localhost:9091"

[metadata]
rpc-caller = "grpcurl"
rpc-service = "ma-apiserver"
rpc-encoding = "proto"

[plugin]
dirs = []  # Add custom plugin directories here
```

### Configurable fields

#### API server

API server configuration is placed under the `[ma]` section.

- `address` - Address of the API server (default: `127.0.0.1:15566`)
- `use_tls` - Whether the client uses TLS credentials (default: `false`)

#### MinIO credentials

MinIO credentials for object storage are placed under the `[minio]` section.

- `access_key_id` - MinIO user name (example: `minioadmin`)
- `secret_access_key` - MinIO password (example: `minioadmin`)
- `endpoint_url` - MinIO endpoint URL (example: `http://localhost:9091`)

#### Custom gRPC metadata

Custom gRPC metadata headers are placed under the `[metadata]` section.

- `rpc-caller` - Identifies the calling client (example: `grpcurl`)
- `rpc-service` - Target service name (example: `ma-apiserver`)
- `rpc-encoding` - Protocol encoding format (example: `proto`)

#### Custom plugins

The `ma` CLI supports custom plugins to extend entity-specific commands and behavior. Plugin configuration is placed under the `[plugin]` section.

**Built-in plugins**: The CLI includes built-in plugins located at `python/michelangelo/cli/mactl/plugins/entity/` that provide core functionality for entities like `pipeline`, `pipeline_run`, and `trigger_run`.

**Custom plugin directories**: You can add additional plugin directories by specifying them in the configuration file:

```toml
[plugin]
dirs = [
    "/path/to/your/custom/plugins",
    "/another/plugin/directory"
]
```

**Plugin directory structure**: Each plugin directory should follow this structure:

```
your-plugin-directory/
└── entity/
    └── {entity_type}/
        └── main.py
```

For example, to create a custom pipeline plugin:

```
my-plugins/
└── entity/
    └── pipeline/
        ├── __init__.py
        └── main.py
```

**Required plugin functions**: Plugin modules should implement one or both of these functions:

- `apply_plugins(crd: CRD, channel: Channel)` - Adds custom command signatures to the entity
- `apply_plugin_command(crd: CRD, target_command: str, crds: dict[str, CRD], channel: Channel)` - Applies logic for specific commands (e.g., `apply`, `create`)

**Note**: Support for per-module plugin configuration via `plugin.modules` is coming soon.

### Environment variables

The following environment variables override config file settings:

- `MACTL_ADDRESS` - Override the API server address
- `MACTL_USE_TLS` - Override the TLS setting (accepts: `true`, `1`, `yes`, `y`)
- `AWS_ACCESS_KEY_ID` - Override MinIO/S3 access key
- `AWS_SECRET_ACCESS_KEY` - Override MinIO/S3 secret key
- `AWS_ENDPOINT_URL` - Override MinIO/S3 endpoint URL

## Troubleshooting

### Common Issues

1. Connection refused: Ensure the API server is running and accessible
2. Resource not found: Verify namespace and resource name are correct
3. YAML parsing errors: Check YAML syntax and required fields
4. Permission denied: Ensure proper authentication/authorization setup

## Tips and Best Practices

1. YAML files must include apiVersion, kind, and metadata sections
2. Resource names are case-sensitive and use snake_case in commands (e.g., pipeline_run not PipelineRun)
3. Check API server connectivity if commands fail with gRPC connection errors

### Debug Mode

Enable debug logging by setting the environment variable:

```bash
export LOG_LEVEL=DEBUG
```

This will provide detailed information about gRPC calls and internal operations.
