# Project Management

A **project** in Michelangelo is a top-level organizational unit that groups related ML pipelines, models, and resources under a single namespace. Each project typically maps to a business use case -- for example, a product recommendation system, a fraud detection pipeline, or a document classification service.

Projects provide:

- **Namespace isolation** -- each project runs in its own Kubernetes namespace (an isolated scope for resources)
- **Ownership and access control** -- define teams and individuals who manage the project
- **Pipeline grouping** -- all pipelines within a project share configuration and resources
- **Lifecycle tracking** -- projects progress through phases from development to production

A project must be created before you can register or run pipelines. See [Pipeline Management](./ml-pipelines/pipeline-management.md) for creating pipelines within a project.

## Prerequisites

Before creating a project, ensure:

1. **The Michelangelo CLI is installed.** See the [CLI Reference](./cli.md) for setup instructions.
2. **The sandbox environment is running.** Follow the [Sandbox Setup Guide](../setup-guide/sandbox-setup.md) to install the required software (Docker, kubectl, k3d) and create a local development environment:

   ```bash
   ma sandbox create
   ```

   This starts all required services in a local Kubernetes cluster, including the API server (`localhost:14566`), MinIO object storage (`localhost:9091`), and a workflow engine (Cadence by default).

3. **Dependencies are installed.** [Poetry](https://python-poetry.org/docs/#installation) must be installed first, then install the project dependencies:

   ```bash
   cd python/
   poetry install
   ```

4. **Verify the environment is ready:**

   ```bash
   ma project get --namespace="default"
   ```

   If the sandbox is running correctly, this command returns successfully (an empty list is expected for a new environment).

> **Alternative:** If you prefer to run only the API server without the full sandbox environment, see the [Sandbox Setup Guide](../setup-guide/sandbox-setup.md) for standalone setup options. Note that project creation requires a running database and workflow engine in addition to the API server.

## Create a project

To create a project, define a `project.yaml` configuration file and apply it with the `ma` CLI.

### Step 1: Create a project folder

Create a folder in your repository with a `config/` subdirectory:

```
my-project/
  config/
    project.yaml
```

### Step 2: Define the project configuration

Create a `project.yaml` file with the following structure:

```yaml
apiVersion: michelangelo.api/v2
kind: Project
metadata:
  name: my-ml-project          # Must follow Kubernetes naming conventions
  namespace: my-ml-project     # Must match the name field exactly
  annotations:
    michelangelo/worker_queue: "default"  # Optional: override the workflow execution queue
spec:
  description: "Product recommendation model training and serving"
  owner:
    owningTeam: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"  # Team identifier (UUID format)
    owners:
      - jane.smith
      - john.doe
    ownerGroups:
      - ml-platform-team
  tier: 3                      # Service criticality: 1 (highest/most critical) to 5 (inactive/soft deletion)
  gitRepo: https://github.com/your-org/your-ml-repo
  rootDir: path/to/ml/project
```

### Step 3: Apply the configuration

Run the following command to create the project:

```bash
ma project apply -f "./my-project/config/project.yaml"
```

The `apply` command is an **upsert** operation: it creates the project if it does not exist, or updates it if it does. You can use the same command to modify project settings later.

### Step 4: Verify the project was created

```bash
ma project get --namespace="my-ml-project" --name="my-ml-project"
```

This will display the project's metadata and spec if the project was created successfully.

## Project YAML reference

### Required fields

| Field | Type | Description |
| --- | --- | --- |
| `metadata.name` | string | Project name. Must follow [Kubernetes naming conventions](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/) (lowercase, alphanumeric, hyphens). |
| `metadata.namespace` | string | Kubernetes namespace. **Must match `metadata.name` exactly.** |
| `spec.description` | string | Human-readable description of the project. |
| `spec.owner.owningTeam` | string | Team identifier in UUID format (e.g., `a1b2c3d4-e5f6-7890-abcd-ef1234567890`). |
| `spec.owner.owners` | list | List of individual owner identifiers. |
| `spec.tier` | integer | Service criticality tier: 1 (most critical) to 4 (least critical), or 5 (inactive project, soft deletion before hard deletion in production). |
| `spec.gitRepo` | string | URL of the Git repository containing the project code. |
| `spec.rootDir` | string | Path within the repository to the project root directory. Used to locate workflow code and configuration files. |

### Optional fields

| Field | Type | Description |
| --- | --- | --- |
| `metadata.annotations` | map | Key-value metadata. Use `michelangelo/worker_queue` to override the default Cadence/Temporal (the workflow orchestration engine) worker queue for this project. |
| `spec.owner.ownerGroups` | list | List of group identifiers for group-based ownership. |
| `spec.commit` | object | Git commit information associated with the project. |
| `spec.supportingLinks` | map | Key-value map of related resource URLs (dashboards, documentation, etc.). |
| `spec.retentionConfig` | object | Retention policies for deployments, endpoints, and models. |
| `spec.typeInfo` | object | Project classification flags (`isCoreMl`, `isGenerativeAi`). |

## Manage projects

### List all projects in a namespace

```bash
ma project get --namespace="my-ml-project"
```

Use `--limit` to control the number of results (default: 100):

```bash
ma project get --namespace="my-ml-project" --limit=10
```

### Get a specific project

```bash
ma project get --namespace="my-ml-project" --name="my-ml-project"
```

### Update a project

Modify your `project.yaml` file and re-run `apply`:

```bash
ma project apply -f "./my-project/config/project.yaml"
```

The CLI detects that the project already exists and performs an update.

### Delete a project

```bash
ma project delete --namespace="my-ml-project" --name="my-ml-project"
```

See the [CLI Reference](./cli.md) for the full list of supported commands and flags.

## Project-pipeline relationship

Projects are the top-level container for ML pipelines. Every pipeline must belong to a project, and the pipeline's namespace must match the project's namespace.

```
Project (my-ml-project)
  ├── Pipeline: training-pipeline
  ├── Pipeline: evaluation-pipeline
  └── Pipeline: serving-pipeline
```

To create a pipeline within a project, register it with the same namespace:

```yaml
apiVersion: michelangelo.api/v2
kind: Pipeline
metadata:
  namespace: "my-ml-project"   # Must match the project namespace
  name: "training-pipeline"
spec:
  type: "PIPELINE_TYPE_TRAIN"
  manifest:
    filePath: my_project.training_workflow
```

```bash
ma pipeline apply -f "./pipeline.yaml"
```

See [Pipeline Management](./ml-pipelines/pipeline-management.md) for details on creating and managing pipelines.

## Project lifecycle

### States

A project transitions through the following states after creation:

| State | Description |
| --- | --- |
| `PROVISIONING` | Project resources are being set up. |
| `PROVISION_PENDING` | Provisioning is queued and waiting to start. |
| `READY` | Project is fully provisioned and available for use. |
| `ERROR` | Provisioning failed. Check the project status for error details. |

### Phases

Projects can be tagged with a lifecycle phase. Phases are organizational labels. Only `DECOMMISSION` has a functional effect -- it blocks new pipeline and pipeline run creation.

| Phase | Description |
| --- | --- |
| `DEVELOPMENT` | Active development and experimentation. |
| `STAGING` | Pre-production validation. |
| `PRODUCTION` | Serving live traffic. |
| `DECOMMISSION` | Marked for retirement. New pipelines and pipeline runs cannot be created in a decommissioned project. |

## Validation rules

The API enforces the following constraints when creating or updating projects:

1. **Name must match namespace.** The `metadata.name` and `metadata.namespace` fields must be identical. A mismatch will cause a validation error.
2. **Reserved namespaces are forbidden.** Projects cannot be created in the `default` or `kube-*` namespaces.
3. **`owningTeam` must be a UUID.** The `spec.owner.owningTeam` field must be a valid UUID string (e.g., `a1b2c3d4-e5f6-7890-abcd-ef1234567890`). Plain text names will be rejected.
4. **Tier must be 1-5.** The `spec.tier` field must be an integer between 1 and 5 inclusive. Tier 5 indicates an inactive project (soft deletion) before hard deletion in production.
5. **Kubernetes naming conventions apply.** The project name must be lowercase, alphanumeric, and may include hyphens. No underscores or uppercase characters.

## Next steps

- [Pipeline Management](./ml-pipelines/pipeline-management.md) -- Create and manage pipelines within your project
- [Pipeline Running Modes](./ml-pipelines/pipeline-running-modes.md) -- Understand local, remote, dev, and production run modes
- [CLI Reference](./cli.md) -- Full command reference for the `ma` CLI
- [Set Up Triggers](./set-up-triggers.md) -- Schedule and automate pipeline execution
- [ML Pipelines Overview](./ml-pipelines/index.md) -- End-to-end guide to building ML workflows
