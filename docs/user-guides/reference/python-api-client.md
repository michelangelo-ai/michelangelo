---
sidebar_position: 1
sidebar_label: "Python APIClient"
last_verified: 2026-11-18
---

# Python APIClient Guide

`APIClient` is the Python client for the Michelangelo AI API server. Use it from scripts, notebooks, and pipeline tasks to read and write resources such as projects, pipelines, pipeline runs, and models without hand-building gRPC stubs. This guide covers how to connect, how to call the most common services, and what the client does when a call fails.

For the full list of methods and fields, use the [gRPC Service Reference](../../api-reference/grpc/conventions.md). `APIClient` is a thin wrapper over those same RPCs.

## Prerequisites

- The `michelangelo` Python package installed in your environment.
- The address of a running Michelangelo AI API server in `host:port` form. In the local sandbox this is `localhost:15566`; see [Sandbox Ports and Endpoints](../../getting-started/ma-sandbox-ports-and-endpoints.md).

## Create a client

There are two ways to use `APIClient`. Both expose the same service attributes.

| Mode | Use it when | Channel owner |
| --- | --- | --- |
| **Per-instance** (`APIClient(...)` or `APIClient.from_env(...)`) | You want an isolated client, or more than one endpoint in a process | The instance |
| **Class-level singleton** (`APIClient.ModelService`, ...) | Existing pipeline code, or a single endpoint configured through the environment | The class, shared by the whole process |

### Per-instance client

```python
from michelangelo.api.v2 import APIClient

with APIClient(endpoint="localhost:15566", caller="my-script") as client:
    project = client.ProjectService.get_project(namespace="default", name="my-project")
```

- `endpoint` is the `host:port` address. Pass either `endpoint` or a pre-built `channel`, never both and never neither. Either mistake raises `ValueError`.
- `caller` names your application. The client sends it as the `rpc-caller` header on every call.
- The context manager closes the channel on exit. Outside a `with` block, call `client.close()` yourself. `close()` does nothing for a `channel` you passed in, so close that one yourself.

To read the address from the environment instead, use `from_env`:

```python
import os
os.environ["MA_API_SERVER"] = "localhost:15566"

from michelangelo.api.v2 import APIClient

with APIClient.from_env("my-script") as client:
    ...
```

`from_env(caller)` raises `ValueError` if `MA_API_SERVER` is unset or has no `:` in it. `APIClient.validate_env()` runs the same check, so you can call it at startup.

### Class-level singleton

```python
import os
os.environ["MA_API_SERVER"] = "localhost:15566"

from michelangelo.api.v2 import APIClient

APIClient.set_caller("my-pipeline")
model = APIClient.ModelService.get_model(namespace="my-project", name="my-model")
```

The singleton opens its channel on the first call, from `MA_API_SERVER`, and closes it at interpreter exit. `set_caller`, `set_channel`, and `set_header_provider` change shared state for every user of the class in the process. After `set_channel`, call `APIClient.init()` so the service stubs pick up the new channel.

:::warning
Set the caller before the first call. If no caller is set, the default header provider raises `ValueError("caller is not set")` when the call is made.
:::

## Configure the connection

| Setting | Where | Default | Effect |
| --- | --- | --- | --- |
| `MA_API_SERVER` | Environment variable | None, required for the singleton and `from_env` | API server address, `host:port` |
| `MA_API_SERVER_NAME` | Environment variable | `ma-apiserver` | Value of the `rpc-service` header, used for routing |
| `caller` | `APIClient(caller=...)` or `APIClient.set_caller(...)` | None, required before the first call | Value of the `rpc-caller` header |
| `timeout` | Argument on every service method | 60 seconds | Per-call deadline |
| `headers` | Argument on every service method | None | Extra headers for that call. `rpc-service` and `rpc-caller` you pass are not overwritten |

The client also sets `rpc-encoding: proto` on every call, and allows messages up to 1 GB in each direction.

### Security and authentication

By default the API server does not authenticate callers. With `apiserver.auth.mode` unset or `dummy`, which is the sandbox setup, every request is allowed. In that case you need no token, and a plaintext connection to `localhost:15566` is fine.

If your operator has set `apiserver.auth.mode: k8s-rbac`, every call must carry an `authorization: Bearer <id-token>` header. The token is an OIDC ID token from your deployment's identity provider. Michelangelo AI doesn't issue it, so get it from the same login flow or tooling your operator set up for the CLI and other clients. The server verifies the token and authorizes the request through Kubernetes RBAC. `APIClient` doesn't attach the header for you, so add it with a header provider. `k8s-rbac` mode runs only behind TLS, so pair the header with TLS credentials:

```python
import grpc
from michelangelo.api.v2 import APIClient
from michelangelo.api.v2.services.base import DefaultHeaderProvider


class BearerHeaderProvider(DefaultHeaderProvider):
    def __init__(self, id_token: str):
        super().__init__()
        self._id_token = id_token

    def get_headers(self, request_headers=None):
        headers = super().get_headers(request_headers)
        headers["authorization"] = f"Bearer {self._id_token}"
        return headers


client = APIClient(
    endpoint="api.example.com:443",
    caller="my-script",
    credentials=grpc.ssl_channel_credentials(),
    header_provider=BearerHeaderProvider(id_token),
)
```

- Use the lowercase key `authorization`, because gRPC metadata keys must be lowercase.
- `credentials` takes a `grpc.ChannelCredentials` and opens a TLS channel. Without it the channel is plaintext and the token would travel unencrypted.
- `header_provider` replaces the default one. Subclassing `DefaultHeaderProvider`, as above, keeps `caller` working. A custom provider without a `caller` attribute makes the `caller=` argument raise `TypeError`.
- ID tokens expire. For long-running processes, have `get_headers` fetch a fresh token each time instead of storing one.
- `interceptors` takes a list of gRPC client interceptors for tracing or other custom behavior. They are ignored when you pass your own `channel`.

For how the server is configured, see [Authentication](../../operator-guides/setup/authentication.md).

## Call the common services

Every service is an attribute of the client, such as `client.PipelineService`. Methods follow one pattern across services: `get_<resource>`, `list_<resource>`, `create_<resource>`, `update_<resource>`, `delete_<resource>`, and `delete_<resource>_collection`. Read and delete calls take `namespace` and `name` as keywords. Create and update calls take the resource object. Responses are protobuf messages, so you read them with attribute access.

The examples below use the `client` from a per-instance client. With the singleton, replace `client` with `APIClient`.

### Projects

```python
project = client.ProjectService.get_project(namespace="default", name="my-project")
print(project.spec.description)

projects = client.ProjectService.list_project(namespace="default")
for p in projects.items:
    print(p.metadata.name)
```

To create one, build a `Project` and pass it in:

```python
from michelangelo.gen.api.v2.project_pb2 import Project

project = Project()
project.metadata.namespace = "default"
project.metadata.name = "my-project"
project.spec.description = "My ML project"
client.ProjectService.create_project(project)
```

### Pipelines

```python
pipeline = client.PipelineService.get_pipeline(namespace="my-project", name="my-pipeline")

pipelines = client.PipelineService.list_pipeline(namespace="my-project")
for p in pipelines.items:
    print(p.metadata.name)
```

### Pipeline runs

Start a run by creating a `PipelineRun` that points at a pipeline, then read its state with `get_pipeline_run`:

```python
from michelangelo.gen.api.options_pb2 import ResourceIdentifier
from michelangelo.gen.api.v2.pipeline_run_pb2 import (
    PipelineRun,
    PipelineRunSpec,
    PipelineRunState,
)
from michelangelo.gen.k8s.io.apimachinery.pkg.apis.meta.v1.generated_pb2 import ObjectMeta

run = PipelineRun()
run.metadata.CopyFrom(ObjectMeta(name="my-run-001", namespace="my-project"))
run.spec.CopyFrom(
    PipelineRunSpec(pipeline=ResourceIdentifier(namespace="my-project", name="my-pipeline"))
)
created = client.PipelineRunService.create_pipeline_run(run)

current = client.PipelineRunService.get_pipeline_run(namespace="my-project", name="my-run-001")
print(PipelineRunState.Name(current.status.state))
if current.status.error_message:
    print(current.status.error_message)
```

`status.state` is one of `PIPELINE_RUN_STATE_PENDING`, `RUNNING`, `SUCCEEDED`, `KILLED`, `FAILED`, or `SKIPPED`, and `PIPELINE_RUN_STATE_INVALID` before the run is validated. You choose the run name, so make it unique within the namespace. `michelangelo.api.v2.generate_random_name("run")` returns a time-sortable unique name.

The SDK already wraps this flow in `michelangelo.uniflow.plugins.pipeline.run`, which provides `create_pipeline_run`, `poll_pipeline_run`, and `run_pipeline`, including input arguments. For one-off runs, `ma pipeline run` does the same from the command line; see [Pipeline Management](../ml-pipelines/pipeline-management.md).

### Models

```python
model = client.ModelService.get_model(namespace="my-project", name="my-model")
print(model.spec.description)

models = client.ModelService.list_model(namespace="my-project")
for m in models.items:
    print(m.metadata.name)
```

`Model` is an immutable resource kind. To publish a trained model from code, use the pusher rather than calling `create_model` yourself; see [Push Models and Artifacts](../train-and-deploy-models/push-models-and-artifacts.md).

### Limit and filter lists

`list_*` methods accept `list_options` and `list_options_ext`, each as a protobuf message or a plain dict:

```python
first_five = client.PipelineRunService.list_pipeline_run(
    namespace="my-project", list_options={"limit": 5}
).items
```

`list_options` is the Kubernetes `ListOptions` message, with fields such as `limit`, `label_selector`, and `field_selector`. `list_options_ext` is Michelangelo AI's `ListOptionsExt`, which adds `order_by`, `pagination`, a `returned_field_skip_list`, and a `CriterionOperation` filter under `operation`. See [How to Read the gRPC API Reference](../../api-reference/grpc/conventions.md) for the list conventions. Always pass a `namespace`; listing across namespaces isn't supported.

## Errors, retries, and timeouts

The client raises or passes through only what the code defines:

- **Configuration errors.** `ValueError` for a missing or malformed `MA_API_SERVER`, for a missing or doubled `endpoint`/`channel`, and for an unset caller. `TypeError` if a custom header provider has no `caller` attribute and you pass `caller=`.
- **Server errors.** The client doesn't wrap these. A failed RPC raises the underlying `grpc.RpcError`, so catch that and inspect `.code()`.
- **Retries.** The channel's service config retries a call up to 3 attempts in total, with exponential backoff from 0.1 s to 10 s, when the status is `INTERNAL`, `UNAVAILABLE`, or `UNKNOWN`. Other status codes fail on the first attempt.
- **Timeouts.** Each call has a 60-second deadline by default. Override it with `timeout=` on the method.

```python
import grpc

try:
    client.PipelineService.get_pipeline(namespace="my-project", name="missing")
except grpc.RpcError as err:
    print(err.code(), err.details())
```

## Services available in Python

The client exposes 12 services: `CachedOutputService`, `DeploymentService`, `EvaluationReportService`, `ModelFamilyService`, `ModelService`, `PipelineRunService`, `PipelineService`, `ProjectService`, `RayClusterService`, `RayJobService`, `SparkJobService`, and `TriggerRunService`. To list them at runtime:

```python
[s for s in dir(APIClient) if s.endswith("Service")]
```

The gRPC reference documents 15 services. `InferenceServerService`, `RevisionService`, and `ClusterService` have no generated Python wrapper at the moment, so call those over gRPC directly.

## Next steps

- [gRPC Service Reference](../../api-reference/grpc/conventions.md) for every RPC, request, and response field.
- [Python SDK Reference](../../api-reference/python-sdk/index.md) for the rest of the `michelangelo` package.
- [CLI Reference](./cli.md) for the same resources from the command line.
