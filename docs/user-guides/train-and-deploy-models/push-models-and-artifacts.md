---
sidebar_position: 2.5
sidebar_label: "Push Models and Artifacts"
last_verified: 2026-10-22
---

# Push Models and Artifacts

After a model is packaged (see the [Model Registry Guide](./model-registry-guide.md)), something has to upload its files to storage and record it in a registry. In Michelangelo AI that step is `push()`. The same call also writes datasets and evaluation reports to their destinations.

This page has two halves:

- **[Using `push()`](#using-push)**: how to call it from a `@uniflow.task` with the three built-in plugins.
- **[Extending `push()`](#extending-push)**: the five interfaces you implement to push somewhere Michelangelo AI doesn't support out of the box. They are a pusher plugin, a model registry client, a storage backend, a dataset sink and an evaluation report sink.

All paths below are relative to `python/michelangelo/`.

:::note No config switches
None of these extension points is selected by a config key, Helm value or environment variable. You pass objects into `push()` from your own task code. The examples below show exactly where each object goes.
:::

## Using `push()`

### Signature

[`workflow/tasks/pusher/task.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/workflow/tasks/pusher/task.py):

```python
def push(
    config: PusherConfig,
    artifacts: dict[str, Any],
    *,
    storage_backend: StorageBackend | None = None,
    registry_client: ModelRegistryClient | None = None,
    registry: PluginRegistry | None = None,
    fail_fast: bool = True,
    on_error: Callable[[str, str, Exception], None] | None = None,
) -> list[PusherResult]:
```

`push()` walks `config.items` in order. For each item it looks up the artifact in `artifacts` by name and checks the artifact's type. It then builds the matching plugin and calls the plugin's `execute()`. You get back one `PusherResult` (`name`, `plugin`, `success`, `value`, `error`) per item processed.

### Configuration

`PusherConfig(items=[...])` holds one `PusherPluginConfig` per artifact ([`workflow/schema/pusher.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/workflow/schema/pusher.py)). Each item has:

- `name`, which must match a key in the `artifacts` dict. Duplicate names in one `PusherConfig` raise `ConfigurationError`.
- **Exactly one** of the following:
  - a built-in typed field: `model_plugin`, `dataset_plugin` or `eval_report_plugin`
  - the extension pair `plugin_name` + `plugin_config`, for a [custom plugin](#1-pusher-plugins-pusherpluginbase-and-pluginregistry)

  Setting none, or more than one, raises `ConfigurationError`.

The three built-in plugins:

| Field | Plugin | Config class | Expected artifact type |
| --- | --- | --- | --- |
| `model_plugin` | `ModelPusherPlugin` | `ModelPluginConfig` | `AssembledModel` |
| `dataset_plugin` | `DatasetPusherPlugin` | `DatasetPluginConfig` | `DatasetVariable` |
| `eval_report_plugin` | `EvalReportPusherPlugin` | `EvalReportPluginConfig` | `EvaluationReport` |

`push()` checks `isinstance(artifact, <expected type>)` before it runs the plugin. A mismatch raises `ConfigurationError`.

### Example

This example follows [`python/examples/bert_cola/push.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/examples/bert_cola/push.py), cut down to local storage. That file shows the full version, which switches to `MinioStorageBackend` when `AWS_ENDPOINT_URL` is set. It also switches to `APIRegistryClient` when `REGISTRY_ENDPOINT` is set.

```python
import tempfile

import michelangelo.uniflow.core as uniflow
from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend
from michelangelo.lib.model_manager.registry import APIRegistryClient
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.workflow.schema.pusher import (
    ModelPluginConfig,
    PusherConfig,
    PusherPluginConfig,
)
from michelangelo.workflow.tasks.pusher import push
from michelangelo.workflow.variables.types import AssembledModel, PusherResult


@uniflow.task(config=RayTask(head_cpu=1, head_memory="1Gi", worker_instances=0))
def push_step(assembled: AssembledModel) -> list[PusherResult]:
    # Live objects are built inside the task body (see "Gotchas" below).
    storage_backend = LocalStorageBackend(tempfile.mkdtemp())
    registry_client = APIRegistryClient(namespace="my-project")

    config = PusherConfig(
        items=[
            PusherPluginConfig(
                name="model",
                model_plugin=ModelPluginConfig(
                    model_name="bert-cola",
                    description="BERT fine-tuned for CoLA",
                    labels={"framework": "transformers"},
                ),
            ),
        ]
    )
    return push(
        config=config,
        artifacts={"model": assembled},
        storage_backend=storage_backend,
        registry_client=registry_client,
    )
```

`APIRegistryClient(namespace=...)` with no `svc` argument uses `APIClient.ModelService`, which needs the `MA_API_SERVER` environment variable. It records the model as a `Model` resource in that namespace. For tests and quick local runs, use `InMemoryRegistryClient()` from the same package instead. It keeps nothing after the process exits.

### Error handling

- `fail_fast=True` (the default): the first plugin failure raises `PusherPluginError`, with the original exception as its `__cause__`. Later items don't run.
- `fail_fast=False`: every item runs. A failed item's `PusherResult` has `success=False` and the message in `.error`.
- `on_error(artifact_name, plugin_name, exc)`: optional. It's called on **every** plugin failure in either mode. If the callback itself raises, the exception is logged and suppressed.
- Some errors abort `push()` straight away in **both** modes, because they happen before `execute()` runs:
  - a name in `config.items` that's missing from `artifacts` (`ArtifactNotFoundError`);
  - an unknown plugin name or an artifact type mismatch (`ConfigurationError`);
  - any exception from a plugin's constructor, such as `ModelPusherPlugin` rejecting a missing registry client (raised as-is, not wrapped and not passed to `on_error`).

### Gotchas

- **`storage_backend` is required.** The signature defaults it to `None`, but `push()` raises `ConfigurationError("storage_backend is required. ...")` when it's missing. It checks this before it looks at any item, including items whose plugin never uploads anything.
- **Build live objects inside the `@task` body.** Registry clients, storage backends and sinks are live Python objects that the Uniflow codec can't serialize. Don't pass a `PusherConfig` that contains them as a task argument across the workflow boundary. Construct it, and the backends, inside the task that calls `push()`.
- **Model registry clients are wired one way or the other.** Pass either `push(registry_client=...)` or `ModelPluginConfig(registry_clients=[...])`, not both; see [Wiring](#wiring).

---

## Extending `push()`

| Interface | What it lets you change | Wired through |
| --- | --- | --- |
| [`PusherPluginBase`](#1-pusher-plugins-pusherpluginbase-and-pluginregistry) | Push a new kind of artifact, or replace a built-in plugin | `PluginRegistry`, passed as `push(registry=...)` |
| [`ModelRegistryClient`](#2-model-registry-clients-modelregistryclient) | Register models in a different registry | `push(registry_client=...)` or `ModelPluginConfig(registry_clients=[...])` |
| [`StorageBackend`](#3-storage-backends-storagebackend) | Upload model files somewhere else | `push(storage_backend=...)` |
| [`DataSink`](#4-dataset-sinks-datasink) | Write datasets to a new destination | `DatasetPluginConfig(sinks=[...])` |
| [`EvalReportSink`](#5-evaluation-report-sinks-evalreportsink) | Send evaluation reports to a new destination | `EvalReportPluginConfig(sinks=[...])` |

Start with the narrowest interface that does the job. To register models in MLflow, for example, write a `ModelRegistryClient`. You don't need a new pusher plugin, because the built-in `ModelPusherPlugin` already does the uploading and calls your client.

---

### 1. Pusher plugins: `PusherPluginBase` and `PluginRegistry`

**Purpose.** A plugin turns one artifact into a push. Write one when the artifact type, or the whole push flow, isn't covered by the built-in plugins.

**Signature.** [`workflow/tasks/pusher/plugins/base.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/workflow/tasks/pusher/plugins/base.py):

```python
class PusherPluginBase(ABC):
    def __init__(
        self,
        config: Any,
        artifact: Any = None,
        storage_backend: StorageBackend | None = None,
        registry_client: ModelRegistryClient | None = None,
    ) -> None: ...

    @abstractmethod
    def execute(self) -> dict[str, Any]: ...
```

The constructor stores its arguments as `self._config`, `self._artifact`, `self._storage_backend` and `self._registry_client`. `execute()` is the only abstract method, and the dict it returns becomes `PusherResult.value`. `push()` creates a new plugin instance for each artifact on each call.

**How it's registered and selected.** [`workflow/tasks/pusher/registry.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/workflow/tasks/pusher/registry.py) maps plugin names to classes:

- `PluginRegistry.register(name, plugin_class, artifact_type=None)`:
  - registers a name in this registry;
  - raises `ValueError` if the name is already registered in the same instance;
  - when `artifact_type` (a type or a tuple of types) is given, `push()` checks the artifact against it.
- `PluginRegistry.get(name)`: looks the name up here, then in the parent registry. It raises `ConfigurationError` listing the registered names if neither has it.
- `PluginRegistry.extend()`: returns a child registry that falls through to this one. Registering a built-in name (for example `"model_plugin"`) in a child **overrides** that built-in for anyone using the child, without changing `default_registry`.
- `default_registry`: holds the three built-ins. `push()` uses it when you don't pass `registry=`.

`PluginRegistry` and `default_registry` are both exported from `michelangelo.workflow.tasks.pusher`.

To use a custom plugin, set the item's extension pair **and** pass your registry to `push()`:

```python
from michelangelo.workflow.tasks.pusher import (
    PusherConfig,
    PusherPluginConfig,
    default_registry,
    push,
)
from michelangelo.workflow.variables.types import AssembledModel

registry = default_registry.extend()  # keeps the built-ins visible
registry.register("manifest_plugin", ManifestPusherPlugin, AssembledModel)

config = PusherConfig(
    items=[
        PusherPluginConfig(
            name="model",
            plugin_name="manifest_plugin",
            plugin_config={"key": "manifests/bert-cola.json"},
        ),
    ]
)
results = push(
    config=config,
    artifacts={"model": assembled},
    storage_backend=storage_backend,
    registry=registry,
)
```

If you leave out `registry=registry`, the lookup goes to `default_registry` and fails with `ConfigurationError: No plugin registered under name 'manifest_plugin'. ...`.

**Minimal implementation.** The plugin receives `plugin_config` as a **raw dict**, exactly as you wrote it, so it has to validate the dict itself. The built-in plugins raise `ConfigurationError` from `__init__`, and you should do the same. A constructor error stops `push()` even with `fail_fast=False`, so bad configuration fails loudly:

```python
import json
import os
import tempfile
from typing import Any

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.tasks.pusher.plugins.base import PusherPluginBase


class ManifestPusherPlugin(PusherPluginBase):
    """Uploads a small JSON manifest describing an AssembledModel."""

    def __init__(self, config, artifact=None, storage_backend=None, registry_client=None):
        super().__init__(config, artifact, storage_backend, registry_client)
        if "key" not in config:
            raise ConfigurationError("manifest_plugin needs plugin_config['key'].")

    def execute(self) -> dict[str, Any]:
        manifest = {"raw_model_path": self._artifact.raw_model.path}
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "manifest.json")
            with open(path, "w") as f:
                json.dump(manifest, f)
            uri = self._storage_backend.upload(path, self._config["key"])
        return {"manifest_uri": uri}
```

For a complete real plugin to copy, see `DatasetPusherPlugin` in [`workflow/tasks/pusher/plugins/dataset_plugin.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/workflow/tasks/pusher/plugins/dataset_plugin.py). It validates its inputs in `__init__` and does the work in `execute`.

**How to test it.** `workflow/tasks/pusher/tests/test_push_dispatch.py` defines a fake `PusherPluginBase` subclass and registers it on a fresh `PluginRegistry()`. It then passes that registry to `push(registry=...)` along with a temporary `LocalStorageBackend`. `workflow/tasks/pusher/tests/test_registry.py` covers registration, fall-through and overrides.

**Gotchas.**

- A bare `PluginRegistry()` has **no** built-ins. Use `default_registry.extend()` if the same `push()` call also uses `model_plugin`, `dataset_plugin` or `eval_report_plugin`.
- You can also register a *new* name directly on `default_registry`. That changes process-wide state, though, and it can't override a built-in (same-instance re-registration raises `ValueError`). Prefer `extend()`.
- A `PluginRegistry` is a live object like the backends. Build it in the task body or at the task module's top level, never as a task argument.

---

### 2. Model registry clients: `ModelRegistryClient`

**Purpose.** `ModelPusherPlugin` uploads the model's files through the storage backend, then calls `register_model()` on a registry client. Write a client to register models somewhere other than the Michelangelo AI API server.

**Signature.** [`lib/model_manager/registry/client.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/lib/model_manager/registry/client.py). The class is exported from `michelangelo.lib.model_manager.registry`.

```python
class ModelRegistryClient(ABC):
    @abstractmethod
    def register_model(
        self,
        name: str,
        artifact_uri: str,
        deployable_artifact_uri: str | None = None,
        description: str | None = None,
        kind: str | None = None,
        schema: dict[str, Any] | None = None,
        labels: Mapping[str, str] | None = None,
        metadata: Mapping[str, Any] | None = None,
        source_pipeline_run: SourcePipelineRun | None = None,
    ) -> RegisteredModel: ...

    @abstractmethod
    def get_model(self, name: str, version: str | None = None) -> RegisteredModel: ...
```

`RegisteredModel` (same file) is a dataclass with these fields:

- `name`, `version` and `registry_uri`, which are required;
- `artifact_uri`, `deployable_artifact_uri`, `kind`, `labels` and `metadata`, which are optional.

The contract, from the docstrings:

- `artifact_uri` is the fully qualified URI the storage backend returned (`s3://...`, `gs://...` or an absolute path).
- A registry with no native field for `schema` or `source_pipeline_run` **must accept and silently ignore** the argument, never raise. The built-in `ModelPusherPlugin` never fills in `schema`. It fills in `source_pipeline_run` from the pipeline pod's environment, or passes `None` outside a pipeline.
- Registries with native run linkage (MLflow's `create_model_version(run_id=...)`, for example) should read `metadata["run_id"]` and pass it natively. `ModelPluginConfig.run_id` ends up there.
- Treat `labels` and `metadata` as read-only.
- `get_model()` raises `KeyError` when the name or version doesn't exist.

#### Wiring

There's no registry of clients, so pass the instance directly. Choose **one** of these:

- `push(registry_client=my_client)`, which goes to every plugin that takes a client.
- `ModelPluginConfig(registry_clients=[client_a, client_b])`, which registers the same uploaded files in each client in order (fan-out).

Passing both raises `ConfigurationError`, and so does passing neither when the config has a `model_plugin` item. In fan-out, registration stops at the first exception. If at least one earlier client has already succeeded, the plugin raises `PartialRegistrationError`, which carries `registrations_completed` and `failed_registry_type` so you can compensate. With a single client, the client's own exception propagates unchanged. Either way, `push()` then handles it like any other plugin failure (see [Error handling](#error-handling)).

**Minimal implementation.** The method bodies are illustrative (`...`); replace them with calls to your registry's API.

```python
from __future__ import annotations

from collections.abc import Mapping
from typing import TYPE_CHECKING, Any

from michelangelo.lib.model_manager.registry import ModelRegistryClient, RegisteredModel

if TYPE_CHECKING:
    from michelangelo.lib.shared.pipeline_run import SourcePipelineRun


class MyRegistryClient(ModelRegistryClient):
    def register_model(
        self,
        name: str,
        artifact_uri: str,
        deployable_artifact_uri: str | None = None,
        description: str | None = None,
        kind: str | None = None,
        schema: dict[str, Any] | None = None,              # accept and ignore if unsupported
        labels: Mapping[str, str] | None = None,
        metadata: Mapping[str, Any] | None = None,
        source_pipeline_run: SourcePipelineRun | None = None,  # accept and ignore if unsupported
    ) -> RegisteredModel:
        version = ...  # create the version in your registry
        return RegisteredModel(
            name=name,
            version=version,
            registry_uri=f"myregistry://{name}/{version}",
            artifact_uri=artifact_uri,
            deployable_artifact_uri=deployable_artifact_uri,
            kind=kind,
            labels=dict(labels or {}),
            metadata=dict(metadata or {}),
        )

    def get_model(self, name: str, version: str | None = None) -> RegisteredModel:
        ...  # raise KeyError when not found
```

To copy from:

- `InMemoryRegistryClient` in the same file is the smallest complete client.
- `APIRegistryClient` in [`lib/model_manager/registry/api_client.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/lib/model_manager/registry/api_client.py) is the production reference. It handles create-versus-update and retries.

**How to test it.** `lib/model_manager/registry/tests/client_test.py` exercises the `InMemoryRegistryClient` contract. Run the same assertions against your client.

:::warning Upgrading a custom registry client (0.8.0+ / #1687)
#1687 shipped in **0.8.0** and added `kind` to `register_model()`. Since then, `ModelPusherPlugin` **always** passes `kind=` as a keyword, along with `source_pipeline_run=`, which arrived later in **0.10.0** (#2034).

A client written against an older signature fails at push time with:

```text
TypeError: register_model() got an unexpected keyword argument 'kind'
```

With the default `fail_fast=True`, `push()` reports it as `PusherPluginError: Plugin 'model_plugin' failed for artifact '<name>'.`, with the `TypeError` as its cause. Fix the client by adding `kind: str | None = None` and `source_pipeline_run=None` to its signature, or by accepting `**kwargs`.

Setting `kind` on `ModelPluginConfig` is still **optional**. Use the `ModelKind` constants from `michelangelo.lib.model_manager.constants` (for example `ModelKind.REGRESSION`). If you leave it unset, the only effect is a blank "Type" column in the registry UI.
:::

To use MLflow as your registry, read [MLflow Model Registry vs Michelangelo AI Model Registry](../../operator-guides/integrations/mlflow.md#mlflow-model-registry-vs-michelangelo-ai-model-registry) first. Michelangelo AI doesn't ship an MLflow `ModelRegistryClient`, so you would write one against this interface.

---

### 3. Storage backends: `StorageBackend`

**Purpose.** A storage backend is where `ModelPusherPlugin` uploads model files, and where any plugin that uses `self._storage_backend` uploads. Write one for an object store that the built-ins don't cover.

This is a different thing from the fsspec "storage backends" in the [Type System reference](../reference/type-system.md), which Uniflow uses to read and write task data.

**Signature.** [`lib/artifact_manager/storage_backend.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/lib/artifact_manager/storage_backend.py):

```python
class StorageBackend(ABC):
    @abstractmethod
    def upload(self, local_path: str, destination_key: str) -> str: ...

    @abstractmethod
    def download(self, uri: str, local_path: str) -> None: ...

    def get_storage_location(self) -> str | None: ...  # optional; defaults to None
```

- `upload()` takes a local **file or directory**. It returns a URI that `download()` on the same backend accepts. Create parent "directories" under `destination_key` as needed.
- `download()` raises `ValueError` for a URI this backend doesn't recognize.
- `get_storage_location()` is used only for logging.

There is **no** `delete()` method, so cleanup is up to you.

**How it's selected.** Only by passing it in: `push(storage_backend=MyBackend(...))`. No config key or environment variable selects a backend.

The built-in backends:

- `LocalStorageBackend(base_dir)` returns absolute paths under `base_dir`.
- `MinioStorageBackend(endpoint, bucket, access_key, secret_key, ...)` is in [`lib/artifact_manager/minio_backend.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/lib/artifact_manager/minio_backend.py). It returns `s3://bucket/key` and rejects URIs that don't start with `s3://`.

**Minimal implementation.** The transfer logic is illustrative:

```python
from michelangelo.lib.artifact_manager.storage_backend import StorageBackend


class GCSStorageBackend(StorageBackend):
    def __init__(self, bucket: str) -> None:
        self._bucket = bucket

    def upload(self, local_path: str, destination_key: str) -> str:
        ...  # upload a file, or every file under a directory, to gs://<bucket>/<destination_key>
        return f"gs://{self._bucket}/{destination_key}"

    def download(self, uri: str, local_path: str) -> None:
        if not uri.startswith(f"gs://{self._bucket}/"):
            raise ValueError(f"URI {uri!r} is not managed by this backend.")
        ...  # download the object, or every object under the prefix, to local_path

    def get_storage_location(self) -> str:
        return f"gs://{self._bucket}"
```

**How to test it.** `lib/artifact_manager/tests/storage_backend_test.py` covers file and directory round trips, overwrites and nested keys for `LocalStorageBackend`. `minio_backend_test.py` in the same folder does the same for an object store.

**Gotchas.** Uploads are at-least-once. If a push fails after the first upload, orphan objects can be left behind. Because there's no `delete()`, use keys that are safe to overwrite: `ModelPusherPlugin` writes to fixed keys under `models/<model_name>/`, so a re-run with the same name overwrites the earlier files.

---

### 4. Dataset sinks: `DataSink`

**Purpose.** `DatasetPusherPlugin` hands the dataset to each configured sink. Write a sink for a destination the built-ins don't cover.

**Signature.** [`workflow/tasks/functions/sinks/base.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/workflow/tasks/functions/sinks/base.py):

```python
class DataSink(ABC):
    @abstractmethod
    def write(self, artifact: DatasetVariable) -> SinkResult: ...
```

`SinkResult(uri, num_records, extra={})` is defined in `workflow/schema/sinks/result.py` and exported from `michelangelo.workflow.schema.sinks`. `DatasetPusherPlugin` merges each sink's `extra` into that sink's entry in `PusherResult.value["sinks"]`.

`artifact.value` can be a pandas DataFrame, a Spark DataFrame or a Ray Dataset, depending on the task that produced it. Check the type and use the native writer. Don't call `toPandas()` on a Spark DataFrame, because it collects the whole dataset onto the driver.

**How it's selected.** Pass sinks in `DatasetPluginConfig(sinks=[...])`. They all receive the same `DatasetVariable`, in order.

If `sinks` is left as `None` and `destination_path` is set, a `LocalFileSink` is created for you. Passing `sinks=[]` turns that off.

The built-in sinks are in `workflow/tasks/functions/sinks/`: `LocalFileSink`, `InMemorySink`, `HiveSink` and `S3Sink`. Each built-in takes a typed config object from `michelangelo.workflow.schema.sinks`, not positional strings:

```python
from michelangelo.workflow.schema.pusher import DatasetFormat, DatasetPluginConfig
from michelangelo.workflow.schema.sinks import LocalFileSinkConfig
from michelangelo.workflow.tasks.functions.sinks import LocalFileSink

DatasetPluginConfig(
    sinks=[
        LocalFileSink(LocalFileSinkConfig("/tmp/out", format=DatasetFormat.CSV)),
        MyWarehouseSink("ml.eval_results"),  # defined below
    ]
)
```

**Minimal implementation.** The write itself is illustrative:

```python
from michelangelo.workflow.schema.sinks import SinkResult
from michelangelo.workflow.tasks.functions.sinks import DataSink
from michelangelo.workflow.variables import DatasetVariable


class MyWarehouseSink(DataSink):
    def __init__(self, table: str) -> None:
        self._table = table

    def write(self, artifact: DatasetVariable) -> SinkResult:
        import pandas as pd

        df = artifact.value
        if not isinstance(df, pd.DataFrame):
            raise TypeError(f"MyWarehouseSink needs pandas, got {type(df).__name__}.")
        ...  # write df to self._table
        return SinkResult(
            uri=f"warehouse://{self._table}",
            num_records=len(df),
            extra={"table": self._table},
        )
```

`InMemorySink` (`workflow/tasks/functions/sinks/memory.py`) is the smallest built-in to copy.

**How to test it.** The quickest end-to-end check drives your sink through the plugin. `test_sink_extra_metadata_appears_in_result` in `workflow/tasks/pusher/tests/plugins/test_dataset_plugin.py` does exactly this with a throwaway `DataSink` subclass. It builds `DatasetPusherPlugin(config=DatasetPluginConfig(sinks=[sink]), artifact=...)`, calls `execute()` and checks `result["sinks"]`.

**Gotchas.** Each sink writes independently. If the second sink raises, the first sink's data is already written.

---

### 5. Evaluation report sinks: `EvalReportSink`

**Purpose.** `EvalReportPusherPlugin` fills in the report's name and then sends the `EvaluationReport` proto to each configured sink. Write a sink to send reports to an experiment tracker, an object store or a queue.

**Signature.** [`workflow/tasks/functions/eval_report_sinks/base.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/workflow/tasks/functions/eval_report_sinks/base.py):

```python
class EvalReportSink(ABC):
    @abstractmethod
    def write(
        self,
        report: EvaluationReport,
        extra_fields: dict[str, Any] | None = None,
    ) -> EvalReportSinkResult: ...
```

`EvalReportSinkResult(name, namespace, output_path="", extra={})` is defined in `workflow/schema/eval_report_sinks/result.py` and exported from `michelangelo.workflow.schema.eval_report_sinks`. Treat `extra_fields` as read-only. Sinks that write files merge it into the output. API sinks ignore it.

**How it's selected.** Pass sinks in `EvalReportPluginConfig(sinks=[...])`:

- `sinks=None` (the default) creates a single `LocalFileEvalReportSink`.
- `sinks=[]` writes nothing, which is useful for a dry run.

The built-in sinks are `LocalFileEvalReportSink` and `APIClientEvalReportSink`, exported from `michelangelo.workflow.tasks.functions.eval_report_sinks`.

**Dispatch.** Sinks run synchronously, once each, in list order. They are **fail-fast**: if a sink raises, the exception propagates and the sinks after it aren't called. Sinks don't need to be thread-safe.

**Minimal implementation.** `flatten_report_to_metrics(report)` converts the report into a flat `{metric_name: value}` dict for key-value systems such as MLflow `log_metrics`. It skips charts with more than one series, and series with more than one data point, and logs a warning for each one it skips.

```python
from typing import Any

from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport
from michelangelo.workflow.schema.eval_report_sinks import EvalReportSinkResult
from michelangelo.workflow.tasks.functions.eval_report_sinks import (
    EvalReportSink,
    flatten_report_to_metrics,
)


class MLflowEvalReportSink(EvalReportSink):
    def write(
        self,
        report: EvaluationReport,
        extra_fields: dict[str, Any] | None = None,
    ) -> EvalReportSinkResult:
        import mlflow

        mlflow.log_metrics(flatten_report_to_metrics(report))
        return EvalReportSinkResult(
            name=report.metadata.name,
            namespace=report.metadata.namespace,
        )
```

**How to test it.** `workflow/tasks/functions/eval_report_sinks/tests/test_sinks.py` builds `EvaluationReport` fixtures and checks the built-in sinks against them.

**Gotchas.** If `report_name` is fixed in `EvalReportPluginConfig`, a retried pipeline run calls `write()` again with the same `metadata.name`. A sink that creates server-side resources, as `APIClientEvalReportSink` does, has to be idempotent or handle duplicates gracefully.

---

## Next steps

- [Deploy a Model](./deploy-a-model.md) to serve a model you've registered.
- [Model Registry (operator guide)](../../operator-guides/components/model-registry.md) for how registered models are stored and queried as `Model` resources.
- [Pipeline Management](../ml-pipelines/pipeline-management.md) for running a pusher step as part of a pipeline.
