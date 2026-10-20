# CLI Reference

The Michelangelo AI CLI (`ma`) provides a unified way to manage resources using standard Kubernetes-style commands. This guide covers all supported commands for managing Michelangelo AI API entities.

## Command summary

All resource types support `get`, `apply`, and `delete` (see [supported resource types](#supported-resource-types) below). Additional commands:

| Command | Description |
|---------|-------------|
| `ma pipeline run` | Execute a registered pipeline |
| `ma pipeline dev-run` | Run a pipeline without registering it |
| `ma pipeline delete` | Delete a pipeline (cascades to child runs by default) |
| `ma pipeline_run kill` | Terminate a running pipeline run |
| `ma trigger_run kill` | Terminate a running trigger |
| `ma trigger_run create` | Start a trigger run from a trigger defined on a pipeline |
| `ma sandbox create` | Set up a local development environment |
| `ma sandbox sync` | Redeploy services into an existing local environment |
| `ma sandbox snapshot create` / `restore` | Save or restore the local environment's Michelangelo AI resources |
| `ma sandbox delete` | Tear down the local environment |

### Supported resource types

| Resource Type | CLI Name | Description | Supported Operations |
|---------------|----------|-------------|----------------------|
| Project | `project` | Namespace and team ownership for ML resources | get, apply, delete |
| Pipeline | `pipeline` | Registered workflow with configuration and scheduling | get, apply, delete, run, dev-run |
| PipelineRun | `pipeline_run` | Single execution instance of a pipeline | get, apply, delete, kill |
| TriggerRun | `trigger_run` | Scheduled or on-demand pipeline execution trigger | get, apply, delete, kill, create |
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

> **Note:** In Michelangelo AI, a *project* is the workspace where your pipelines, models, and triggers live. In YAML files and CLI flags, your project is identified by the `namespace` field — these refer to the same thing. See the [Project Management guide](../getting-started/project-management-for-ml-pipelines.md) for details.

## Prerequisites

1. **Install [Python >= 3.9](https://www.python.org/downloads/)** and **[Poetry](https://python-poetry.org/docs/#installation)**.

2. **Install dependencies:**

   ```bash
   cd python/
   poetry install
   ```

3. **Start the sandbox environment.** Follow the [Sandbox Setup Guide](../../getting-started/sandbox-setup.md) to install the required software (Docker, kubectl, k3d) and create a local development environment:

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

All Michelangelo AI API entities support the following standard operations -- GET, APPLY, and DELETE

### General syntax

```bash
cd $REPO_ROOT/python/
ma <RESOURCE_TYPE> <COMMAND> [ARGS]
```

We will abstract this part like `ma <RESOURCE_TYPE> <COMMAND>` in below.

### GET - Retrieve resource

Retrieve information about an existing resource by project and name. Pass the name either as a positional argument or with `--name`. If you omit the name, `get` lists all resources in the specified project.

Syntax:

```bash
ma <RESOURCE_TYPE> get [<name>] --namespace="<namespace>" [--output=<table|yaml|json>]
ma <RESOURCE_TYPE> get --namespace="<namespace>" [--name="<name>"] [--limit=<n>] [--output=<table|yaml|json>]
ma <RESOURCE_TYPE> get --all-namespaces [--limit=<n>] [--output=<table|yaml|json>]
# Short form: -n for --namespace, -A for --all-namespaces, -o for --output
ma <RESOURCE_TYPE> get "<name>" -n "<namespace>" [-o <table|yaml|json>]
ma <RESOURCE_TYPE> get -A [-o <table|yaml|json>]
```

Examples:

```bash
# List all projects
ma project get --namespace="my-project"

# List all pipelines in a project
ma pipeline get --namespace="my-project"

# Get a specific pipeline
ma pipeline get --namespace="my-project" --name="bert-cola-test"

# Get a specific project
ma project get --namespace="my-project" --name="my-project"

# Get a pipeline run
ma pipeline_run get --namespace="my-project" --name="run-001"

# Get a pipeline, passing the name positionally
ma pipeline get bert-cola-test -n "my-project"

# List pipelines across every project
ma pipeline get --all-namespaces

# Print a pipeline as YAML instead of a table
ma pipeline get --namespace="my-project" --name="bert-cola-test" --output=yaml
```

#### Arguments

- `<name>` / `--name` — name of the resource to get. If you pass both, the positional name wins. Omit both to list.
- `--namespace` / `-n` — project to read from. Required unless you pass `--all-namespaces`.
- `--all-namespaces` / `-A` — list the resource type across all projects. `--namespace` is ignored, and you can't combine it with a name.
- `--output` / `-o` — output format: `table`, `yaml`, or `json` (default: `table`). Applies to single resources and lists.
- `--limit` — maximum number of results to return when listing (default: 100)

Lists come back newest first, ordered by `metadata.creation_timestamp`. Some resource types add their own list filters; see [Type-specific commands](#type-specific-commands). Those filters apply only when listing, not when you get a single resource by name.

### APPLY - Create or update a resource from YAML

Apply (create or update) a resource from a YAML configuration file. The `apply` command works as an upsert: it creates the resource if it doesn't exist, or updates it if it does. The resource type comes from `<RESOURCE_TYPE>` on the command line; the YAML's `apiVersion` and `kind` must still be present. With `-R`, files whose `kind` doesn't match `<RESOURCE_TYPE>` are skipped.

Syntax:

```bash
ma <RESOURCE_TYPE> apply --file="<YAML_FILE_PATH>" [--root="<ROOT_DIR>"] [--recursive] [--dry-run]
# Short form: -f for --file, -r for --root, -R for --recursive
ma <RESOURCE_TYPE> apply -f "<YAML_FILE_PATH>" [-r "<ROOT_DIR>"] [-R] [--dry-run]
```

Examples:

```bash
# Apply a pipeline configuration
ma pipeline apply --file="./examples/bert_cola/pipeline.yaml"

# Apply a project configuration
ma project apply --file="./project.yaml"

# Apply every Pipeline YAML under a directory, recursively
ma pipeline apply --file="./pipelines/" --recursive

# Validate a change on the server without saving it
ma pipeline apply --file="./examples/bert_cola/pipeline.yaml" --dry-run
```

#### Arguments

- `--file` / `-f` — path to the YAML file, or to a directory when used with `--recursive` (required)
- `--root` / `-r` — external workspace root. When set, it is prepended to `--file`.
- `--recursive` / `-R` — when `--file` is a directory, walk it recursively and apply every `.yaml` file whose `kind` matches `<RESOURCE_TYPE>`. Other files are skipped with a message, and an error in one file doesn't stop the walk. If any file fails, the command reports the failed files at the end and exits with an error.
- `--dry-run` — server-side dry run: the server validates the request and rolls it back, so nothing is saved

### DELETE - Remove a resource

Delete a specific resource by project and name.

Syntax:

```bash
ma <RESOURCE_TYPE> delete --namespace="<namespace>" --name="<name>"
# Short form: -n for --namespace
ma <RESOURCE_TYPE> delete -n "<namespace>" --name="<name>"
```

Examples:

```bash
# Delete a pipeline
ma pipeline delete --namespace="my-project" --name="bert-cola-test"

# Delete a project
ma project delete --namespace="my-project" --name="my-project"

# Delete a pipeline run
ma pipeline_run delete --namespace="my-project" --name="run-001"
```

#### Pipeline delete and cascade

`ma pipeline delete` removes a Pipeline **and cascades to its child PipelineRuns and TriggerRuns** (Kubernetes `foreground` propagation): in-flight runs are drained, their final state is retained in MySQL, then they are removed before the Pipeline. The command prompts for confirmation; pass `--yes` to skip it. This is **irreversible**.

```bash
ma pipeline delete --namespace="<namespace>" --name="<name>" [--yes]
```

- `--yes` — skip the confirmation prompt

```bash
# Prompts for confirmation
ma pipeline delete --namespace="my-project" --name="bert-cola-test"

# Skip confirmation (scripting)
ma pipeline delete --namespace="my-project" --name="bert-cola-test" --yes
```

To delete a Pipeline but **keep** its runs, use `kubectl delete pipeline <name> -n <namespace> --cascade=orphan`. For propagation policy, the RBAC caveat, and monitoring, see the [Cascade Delete operator guide](../../operator-guides/cascade-delete.md).

## Type-specific commands

Some resource types support additional commands beyond GET, APPLY, and DELETE.

### Pipeline

#### GET filters - Narrow a pipeline list

When listing pipelines, you can filter by owner and type. Pipeline lists also show `OWNER` and `TYPE` columns.

Syntax:

```bash
ma pipeline get --namespace="<namespace>" [--owner="<user>"] [--type=<PIPELINE_TYPE>]
```

Examples:

```bash
# List the training pipelines a user owns
ma pipeline get --namespace="my-project" --owner="alice" --type=TRAIN

# List evaluation pipelines across all projects
ma pipeline get --all-namespaces --type=EVAL
```

- `--owner` — list only pipelines owned by this user
- `--type` — list only pipelines of this type, e.g. `TRAIN`, `EVAL`, or `PREDICTION`. Case-insensitive, and the full enum name (`PIPELINE_TYPE_TRAIN`) also works. An unknown type fails with the list of valid values.

#### RUN - Execute a pipeline

The RUN command is specifically available for pipelines to create and execute pipeline runs. To run a pipeline, you need to register your pipeline first using `ma pipeline apply -f <pipeline_conf.yaml>`.

Syntax:

```bash
ma pipeline run --namespace="<namespace>" --name="<pipeline_name>" [--dry-run]
# Short form: -n for --namespace
ma pipeline run -n "<namespace>" --name="<pipeline_name>" [--dry-run]
```

Example:

```bash
# Run a registered pipeline
ma pipeline run --namespace="my-project" --name="bert-cola-test"

# Check that the run would be accepted, without launching it
ma pipeline run --namespace="my-project" --name="bert-cola-test" --dry-run
```

##### Arguments

- `--resume_from` - create resumed pipeline run from specified pipeline run (specifying resume_from step is optional)
- `--dry-run` — the server validates the pipeline and parameters, then rolls back. No run is launched.

##### Resume_From Argument

The RUN command also can have a `--resume_from` argument that allows a new pipeline run to be resumed from a previous pipeline line run. If a pipeline run step is not specified in the resume_from argument, the resumed pipeline will automatically resume from the last failed step of the previous pipeline.

Syntax:

```bash
ma pipeline run --namespace="<namespace>" --name="<pipeline_name>" --resume_from=<pipeline_run_name>:<pipeline_run_step_name>
```

Example:

```bash
ma pipeline run --namespace="my-project" --name="bert-cola-test" --resume_from=run-1759873504-b93b7f612:train
```

##### Notification Arguments

You can attach notification rules directly to a pipeline run so you're alerted when it reaches a terminal state. This is useful for one-off runs where you want a quick "ping me when it's done" without editing YAML specs.

- `--notify-slack` — Slack destination (channel or @user). Repeatable or comma-separated.
- `--notify-email` — Email address. Repeatable or comma-separated.
- `--notify-on` — Event type to trigger on: `SUCCEEDED`, `FAILED`, `KILLED`, `SKIPPED`, or `STARTED`. Repeatable or comma-separated. Defaults to the four terminal states (`SUCCEEDED`, `FAILED`, `KILLED`, `SKIPPED`) when omitted; `STARTED` is opt-in. Applies to all destinations (per-destination filtering is not yet supported — use YAML specs for that).

Syntax:

```bash
ma pipeline run -n "<namespace>" --name="<pipeline_name>" \
  --notify-slack "<channel_or_user>" \
  --notify-email "<email_address>" \
  --notify-on <EVENT_TYPE>
```

Example:

```bash
# Notify a Slack channel and two email addresses on failure or success
ma pipeline run -n "my-project" --name="bert-cola-test" \
  --notify-slack "#ml-alerts" \
  --notify-email alice@example.com,oncall@example.com \
  --notify-on FAILED,SUCCEEDED
```

For advanced notification configuration (per-destination event filtering, trigger run notifications, or standing notification rules), see [Pipeline Notifications](../ml-pipelines/notifications.md).

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

> For an architectural overview of how these two modes differ, see [Remote Run vs Pipeline Dev Run: what's actually different](../ml-pipelines/pipeline-running-modes.md#remote-run-vs-pipeline-dev-run-whats-actually-different).

**1. dev-run: Test Pipeline from Local File**

`pipeline dev-run` command runs a pipeline directly from your committed git snapshot. Pipeline run will be controlled by Michelangelo AI API server and controller. This command creates a PipelineRun entity but no Pipeline entity, so you will not see the pipeline entity information in MA Studio.

**remote-run** (invoked via `python my_workflow.py remote-run`, not an `ma` command) bypasses the Michelangelo AI API server and submits your workflow directly to Cadence/Temporal. No Michelangelo AI entities are created, and pipeline status is not visible in MA Studio.

**2. dev-run --file-sync: Test Pipeline + Uncommitted Changes**

Adding `--file-sync` to the `pipeline dev-run` command enables testing of uncommitted code changes without needing to commit or rebuild Docker images.

**dev-run --file-sync**: Creates two tarballs: a workflow tarball (from committed code) and a file-sync tarball (containing only files changed via `git diff`). When the container starts, `sitecustomize.py` downloads the file-sync tarball and overlays changed files on top of the base code. The file-sync URL is passed via the `UF_FILE_SYNC_TARBALL_URL` environment variable.

**remote-run**: Creates a workflow tarball from committed code and sends it straight to the workflow engine without creating any Michelangelo AI entities.

**remote-run --file-sync**: Creates two tarballs: a workflow tarball (base64-encoded in the Cadence CLI input) and a file-sync tarball (uploaded to S3). The S3 URL is passed as an environment variable to the container.

### Pipeline_run

#### GET filters - Narrow a pipeline run list

When listing pipeline runs, you can filter by who launched the run and by revision. Pipeline run lists also show `REVISION`, `USER`, `ENVIRONMENT`, and `STATE` columns.

Syntax:

```bash
ma pipeline_run get --namespace=<NAMESPACE> [--actor=<USER>] [--revision=<PATTERN>]
```

Examples:

```bash
# List runs launched by a user
ma pipeline_run get --namespace=my-project --actor=alice

# List runs whose revision name contains "bert-cola"
ma pipeline_run get --namespace=my-project --revision="%bert-cola%"
```

- `--actor` — list only runs launched by this user
- `--revision` — list only runs whose revision name matches this pattern. The server matches it with SQL `LIKE`, so use `%` as a wildcard.

#### Kill - Terminate a pipeline run

The KILL command is used to cleanly terminate a running pipeline. It sets the PipelineRun status to "killed" and aborts the pipeline execution in Cadence/Temporal. The command will prompt for confirmation unless the `--yes` flag is provided.

Syntax:

```bash
ma pipeline_run kill --namespace=<NAMESPACE> --name=<NAME> [--yes] [--dry-run]
```

Parameters:

- `--namespace`: Kubernetes namespace where the pipeline run exists
- `--name`: Name of the pipeline run to kill
- `--yes`: (Optional) Skip confirmation prompt and kill immediately
- `--dry-run`: (Optional) The server checks that you're allowed to kill the run, then rolls back. `spec.kill` is not set and the run keeps going. The confirmation prompt still appears unless you pass `--yes`.

Example:

```bash
# Kill a pipeline run with confirmation prompt
ma pipeline_run kill --namespace=my-project --name=pipeline-run-20251118-194500-8cdb1538

# Kill a pipeline run without confirmation prompt
ma pipeline_run kill --namespace=my-project --name=pipeline-run-20251118-194500-8cdb1538 --yes

# Check permission to kill a run without killing it
ma pipeline_run kill --namespace=my-project --name=pipeline-run-20251118-194500-8cdb1538 --dry-run --yes
```

### Revision

#### GET filters - Narrow a revision list

When listing revisions, you can filter by the kind of resource a revision belongs to (pipeline, model, or deployment) and by owner. Revision lists also show `TYPE`, `USER`, and `BASE_RESOURCE` columns.

Syntax:

```bash
ma revision get --namespace=<NAMESPACE> [--pipeline=<PATTERN> | --model=<PATTERN> | --deployment=<PATTERN>] [--owner=<USER>]
```

Examples:

```bash
# List all pipeline revisions in a project
ma revision get --namespace=my-project --pipeline=""

# List revisions of models whose name starts with "bert"
ma revision get --namespace=my-project --model="bert%"

# List a user's deployment revisions
ma revision get --namespace=my-project --deployment="" --owner=alice
```

- `--pipeline`, `--model`, `--deployment` — list only revisions of that resource kind. The value is matched against the resource's name with SQL `LIKE` (`%` is a wildcard). Pass an empty value (`--pipeline=""`) to match every revision of that kind; the flag always needs a value. These three flags are mutually exclusive.
- `--owner` — list only revisions owned by this user. Combines with any of the flags above.

### Trigger_run

#### Kill - Terminate a running trigger

The KILL command is used to cleanly terminate a running trigger_run resource. This command sets the trigger's kill flag, which triggers proper Cadence workflow termination. The command will prompt for confirmation unless the --yes flag is provided.

Syntax:

```bash
ma trigger_run kill --namespace=<NAMESPACE> --name=<NAME> [--yes] [--dry-run]
```

Parameters:

- `--namespace`: Project where the trigger run exists
- `--name`: Name of the trigger run to kill
- `--yes`: (Optional) Skip confirmation prompt and kill immediately
- `--dry-run`: (Optional) The server checks that you're allowed to kill the trigger run, then rolls back. `spec.kill` is not set and the trigger keeps running. The confirmation prompt still appears unless you pass `--yes`.

Example:

```bash
# Kill a trigger run with confirmation prompt
ma trigger_run kill --namespace=my-project --name=training-pipeline-cron-trigger

# Kill a trigger run without confirmation prompt
ma trigger_run kill --namespace=my-project --name=training-pipeline-cron-trigger --yes

# Check permission to kill a trigger run without killing it
ma trigger_run kill --namespace=my-project --name=training-pipeline-cron-trigger --dry-run --yes
```

#### Create - Start a trigger run from a pipeline's trigger

Create a TriggerRun from a pipeline's trigger configuration. The command looks up `<TRIGGER_NAME>` in the registered pipeline's `triggerMap` and creates a TriggerRun named `<TRIGGER_NAME>-<random suffix>`, with you as the actor. Use it to start a trigger that's already defined on the pipeline; use `ma trigger_run apply -f` instead when you want to create a TriggerRun from your own YAML.

Syntax:

```bash
ma trigger_run create --namespace=<NAMESPACE> --pipeline=<PIPELINE_NAME> --trigger-name=<TRIGGER_NAME> [--dry-run]
# Short form: -n for --namespace, -p for --pipeline, -t for --trigger-name
ma trigger_run create -n <NAMESPACE> -p <PIPELINE_NAME> -t <TRIGGER_NAME> [--dry-run]
```

Parameters:

- `--namespace` / `-n`: Project of the pipeline. The trigger run is created in the same project. (Required)
- `--pipeline` / `-p`: Name of the registered pipeline (Required)
- `--trigger-name` / `-t`: Key of the trigger in the pipeline's `triggerMap` (Required). If the pipeline has no triggers, or the name isn't found, the command fails and lists the available triggers.
- `--dry-run`: (Optional) The server validates the trigger configuration and rolls back without creating a TriggerRun.

Example:

```bash
# Start the pipeline's "daily" trigger
ma trigger_run create --namespace=my-project --pipeline=training-pipeline --trigger-name=daily

# Validate it first without creating anything
ma trigger_run create -n my-project -p training-pipeline -t daily --dry-run
```

## Sandbox commands

The `ma sandbox` commands manage a local K3d development environment. For prerequisites, setup walkthrough, and detailed options, see the [Sandbox Setup Guide](../../getting-started/sandbox-setup.md).

| Command | Description |
|---------|-------------|
| `ma sandbox create` | Create a K3d cluster with all Michelangelo AI services |
| `ma sandbox create --workflow temporal` | Create with Temporal instead of Cadence |
| `ma sandbox create --exclude ui` | Create without specific services |
| `ma sandbox create --create-compute-cluster` | Create with a Ray compute cluster |
| `ma sandbox create --compute-cluster-name <name>` | Name the compute cluster created by `--create-compute-cluster` (default: `michelangelo-compute-0`) |
| `ma sandbox create --wait-timeout <seconds>` | Seconds to wait for pods to become ready (default: 600) |
| `ma sandbox create --include-experimental <service>` | Also deploy experimental services (currently `mlflow`) |
| `ma sandbox create --set KEY=VALUE` | Pass a value through to `helm upgrade`/`helm install`. Repeatable. |
| `ma sandbox sync` | Redeploy services into an existing cluster, skipping cluster creation and image import. Falls back to a full `create` if the cluster doesn't exist. Accepts `--exclude`, `--workflow`, `--wait-timeout`, `--include-experimental`, and `--set`. |
| `ma sandbox delete` | Tear down the cluster and all resources |
| `ma sandbox delete --compute-cluster-name <name>` | Name of the compute cluster to delete along with the sandbox, if it exists (default: `michelangelo-compute-0`). Use it when you passed a custom name to `create --compute-cluster-name`. |
| `ma sandbox start` | Start a stopped cluster |
| `ma sandbox stop` | Stop the cluster (preserves state) |
| `ma sandbox demo pipeline` | Create demo pipeline resources |
| `ma sandbox demo inference` | Create demo inference server resources |
| `ma sandbox demo inference-multicluster` | Create a multi-cluster inference server demo |
| `ma sandbox demo kueue [--compute-cluster-name <name>]` | Install Kueue on a registered compute cluster, create the demo ClusterQueue and LocalQueue, and set the cluster's `scheduler_type`. Defaults to the sandbox cluster itself (`michelangelo-sandbox`). See the [Kueue scheduler backend guide](../../operator-guides/jobs/kueue-scheduler-backend.md). |
| `ma sandbox snapshot create` | Save all Michelangelo AI resources (CRDs) in the cluster to disk |
| `ma sandbox snapshot restore <timestamp>` | Restore a snapshot into the cluster. `<timestamp>` is required: the bare timestamp (e.g. `20260807-170000`) or the full path printed by `snapshot create`. |

## YAML Resource Examples

### Pipeline YAML

```yaml
apiVersion: michelangelo.api/v2
kind: Pipeline
metadata:
  namespace: "my-project"  # Your project name
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
  name: my-project
  namespace: my-project
spec:
  description: My ML Project
  owner:
    owningTeam: "michelangelo"
    owners: "sample name"
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
  namespace: my-project
spec:
  pipeline:
    name: training-pipeline
    namespace: my-project
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
access_key_id = "michelangeloadmin"
secret_access_key = "michelangeloadmin"
endpoint_url = "http://localhost:9091"

[metadata]
rpc-caller = "grpcurl"
rpc-service = "ma-apiserver"
rpc-encoding = "proto"

[plugin]
dirs = []  # Add custom plugin directories here
packages = []  # Add importable plugin packages here
```

### Configurable fields

#### API server

API server configuration is placed under the `[ma]` section.

- `address` - Address of the API server (default: `127.0.0.1:15566`)
- `use_tls` - Whether the client uses TLS credentials (default: `false`)

#### MinIO credentials

MinIO credentials for object storage are placed under the `[minio]` section.

- `access_key_id` - MinIO user name (example: `michelangeloadmin`)
- `secret_access_key` - MinIO password (example: `michelangeloadmin`)
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

- `apply_plugins(crd: CRD, channel: Channel, *args, **kwargs)` - Adds custom command signatures to the entity
- `apply_plugin_command(crd: CRD, target_command: str, crds: dict[str, CRD], channel: Channel, *args, **kwargs)` - Applies logic for specific commands (e.g., `apply`, `create`)

> **Note**: Always include `*args, **kwargs` in your plugin function signatures. This ensures your plugin remains compatible with future mactl versions that may pass additional context. If it's not used, you may use `*_, **__` as a convention to indicate unused parameters.

**Plugin packages**: Instead of a directory path, you can list importable Python package names. Each package must contain `entity/{entity_type}/main.py`, in the same layout as a plugin directory:

```toml
[plugin]
packages = [
    "my_company_plugins",
]
```

Prefer `packages` over `dirs` for plugins shipped inside a wheel or PEX. They're loaded through the import system, so they work under zipimport and relative imports inside the plugin work.

**Module overrides**: `[plugin.modules]` replaces individual functions anywhere in the CLI. Each key is the function to replace and each value is the replacement, both as dotted `module.function` paths:

```toml
[plugin.modules]
"michelangelo.cli.mactl.some_module.some_func" = "my_pkg.overrides.some_func"
```

Overrides are applied before plugin discovery, so they have the highest priority and plugins pick up the replaced functions. An override that fails to import is logged as an error and skipped; it doesn't stop the CLI.

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
2. Resource not found: Verify project name and resource name are correct
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

`ma -vv` appears in `ma -h` but currently has no effect; use `LOG_LEVEL=DEBUG`.
