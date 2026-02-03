# CLI Tools

ma (Michelangelo CLI interface) provides a unified way to manage Michelangelo resources using standard Kubernetes-style commands. This guide covers all supported commands for managing Custom Resource Definitions (CRDs).

## Prerequisites

Before using ma, ensure:

1. **API Server is running**:
   ```bash
   # Start the Michelangelo API server (from repository root)
   $ bazel run //go/cmd/apiserver:apiserver
   ```

2. **Dependencies are installed**:
   ```bash
   # Set repo root and install dependencies
   $ export REPO_ROOT="/Users/{username}/michelangelo"
   $ cd $REPO_ROOT/python/
   $ poetry install -E ma
   ```

3. **Configure API server address** (optional):
   ```bash
   # Override default API server address
   $ export ma_ADDRESS="127.0.0.1:14566"  # e.g., for Michelangelo Api Server
   ```



## Usage

All CRDs support the following standard operations -- GET, APPLY, and DELETE

### How to run the ma in michelangelo repository

**Usage:**
```bash
$ cd $REPO_ROOT/python/ 
$ poetry run python -m michelangelo.cli.ma.ma <COMMAND> <RESOURCE_TYPE> [ARGS]
```

We will abstract this part like `$ ma <RESOURCE_TYPE> <COMMAND> ` in below.

### GET - Retrieve resource

Retrieve information about an existing resource by namespace and name. If you don't specify the `--name` field, it would list all resources under the specified namespace.

**Syntax:**
```bash
$ ma <RESOURCE_TYPE> get  --namespace="<namespace>" [--name="<name>"]
```

**Examples:**
```bash
# List all projects
$ poetry run python -m michelangelo.cli.ma.ma project get --namespace="ma-dev-test"

# List all pipelines in a namespace
$ poetry run python -m michelangelo.cli.ma.ma pipeline get --namespace="ma-dev-test"

# Get a specific pipeline
$ poetry run python -m michelangelo.cli.ma.ma pipeline get --namespace="ma-dev-test" --name="bert-cola-test"

# Get a specific project
$ poetry run python -m michelangelo.cli.ma.ma project get --namespace="ma-dev-test" --name="my-project"

# Get a pipeline run
$ poetry run python -m michelangelo.cli.ma.ma pipeline_run get --namespace="ma-dev-test" --name="run-001"

# Get a prompt template
$ poetry run python -m michelangelo.cli.ma.ma prompt_template get --namespace="ma-dev-test" --name="classification-prompt"
```

### APPLY - Create or update a resource from YAML

Apply (create or update) a resource from a YAML configuration file. ma automatically detects the resource type from the `apiVersion` and `kind` fields in the YAML.

Note: Currently, we support `create` command for creating a new CRD by using ma. Creating a new CRD with `apply` command would fail. This will be fixed soon.

**Syntax:**
```bash
$ ma pipeline apply --file="<YAML_FILE_PATH>"
```

**Examples:**
```bash
# Apply a pipeline configuration
$ poetry run python -m michelangelo.cli.ma.ma pipeline apply --file="./examples/bert_cola/pipeline.yaml"

# Apply a project configuration  
$ poetry run python -m michelangelo.cli.ma.ma project apply --file="./project.yaml"

# Apply a prompt template
$ poetry run python -m michelangelo.cli.ma.ma prompt_template apply --file="./prompt_template.yaml"
```

### DELETE - Remove a resource

Delete a specific resource by namespace and name.

**Syntax:**
```bash
$ poetry run python -m michelangelo.cli.ma.ma <resource_type> delete --namespace="<namespace>" --name="<name>"
```

**Examples:**
```bash
# Delete a pipeline
$ poetry run python -m michelangelo.cli.ma.ma pipeline delete --namespace="ma-dev-test" --name="bert-cola-test"

# Delete a project
$ poetry run python -m michelangelo.cli.ma.ma project delete --namespace="ma-dev-test" --name="my-project"

# Delete a pipeline run
$ poetry run python -m michelangelo.cli.ma.ma pipeline_run delete --namespace="ma-dev-test" --name="run-001"

# Delete a prompt template
$ poetry run python -m michelangelo.cli.ma.ma prompt_template delete --namespace="ma-dev-test" --name="classification-prompt"
```

## Default Plugin commands

ma support the default plugin commands for users for specific CRDs.

### Pipeline

#### RUN - Execute a pipeline

The RUN command is used to create and execute pipeline runs. To run a pipeline, you need to register your pipeline by using `ma apply <pipeline_conf.yaml PATH` command first.

**Syntax:**
```bash
$ poetry run python -m michelangelo.cli.ma.ma pipeline run --namespace="<namespace>" --name="<pipeline_name>"
```

**Example:**
```bash
# Run a registered pipeline
$ poetry run python -m michelangelo.cli.ma.ma pipeline run --namespace="ma-dev-test" --name="bert-cola-test"
```

#### DEV RUN - Execute a pipeline in DEV mode

The DEV RUN command is used to run a pipeline without registering it. This command is to allow users to quickly iterate on their pipelines. The dev-run command supports an `--env` flag for passing environment variables, which are injected into the pipeline's execution environment.

**Syntax:**
```bash
$ poetry run python -m michelangelo.cli.ma.ma pipeline dev-run --file=<YAML_FILE_PATH> --env=<ENV_VAR>=<ENV_VAL>
```

**Example:**
```bash
# Run a pipeline in dev mode
$ poetry run python -m michelangelo.cli.ma.ma pipeline dev-run  --file="./examples/bert_cola/pipeline.yaml" --env=foo=bar
```

## YAML Resource Examples

### Pipeline YAML
```yaml
apiVersion: michelangelo.uber.com/v2beta1
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

### PromptTemplate YAML
```yaml
apiVersion: michelangelo.uber.com/v2beta1
kind: PromptTemplate
metadata:
  name: ma-pt-test-001
  namespace: ma-integration-test
spec:
  features:
    - name: user_name
      value: albert greenberg
  instruction: |-
    Classify the following user's name: '{{user_name}}'
  messages:
    - content: Please classify user's name..
      role: user
  model: azure-openai-gpt4
  model_params:
    max_tokens: 1024
    temperature: 0.01
  traffic_type: PROMPT_TEMPLATE_TRAFFIC_TYPE_PRODUCTION
  type: PROMPT_TEMPLATE_TYPE_LLM_CHAT_COMPLETION
```

## Tips and Best Practices

1. **Always specify both namespace and name** for GET and DELETE operations
2. **Use APPLY instead of CREATE** - APPLY handles both creation and updates
3. **YAML files must include apiVersion, kind, and metadata** sections
4. **Resource names are case-sensitive** and use snake_case in commands (e.g., `prompttemplate` not `PromptTemplate`)
5. **Check API server connectivity** if commands fail with connection errors

## Configuration

ma supports configuration through a `config.yaml` file located at `michelangelo/cli/ma/config.yaml`. This can include:

- MinIO/S3 credentials for object storage
- API server endpoints
- Default namespaces

Example config:
```yaml
minio:
  access_key_id: "your-access-key"
  secret_access_key: "your-secret-key"
  endpoint_url: "http://localhost:9000"
```



## Troubleshooting

### Common Issues

1. **Connection refused**: Ensure the API server is running and accessible
2. **Resource not found**: Verify namespace and resource name are correct
3. **YAML parsing errors**: Check YAML syntax and required fields
4. **Permission denied**: Ensure proper authentication/authorization setup

### Debug Mode

Enable debug logging by setting the environment variable:
```bash
$ export LOG_LEVEL=DEBUG
```

This will provide detailed information about gRPC calls and internal operations.

