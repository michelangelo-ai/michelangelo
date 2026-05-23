# How to Write a Task in `michelangelo` (Open Source)

> LLM-readable reference. Synthesises learnings from the CanvasFlex Pusher Phase 1
> migration (PR3). Read this before generating code for any new workflow task.

---

## Folder Taxonomy

```
workflow/
    schema/                  Contracts — ABCs, config dataclasses, enums, exceptions
        sinks/               Typed sink configs + SinkResult (data contracts)
            result.py        SinkResult dataclass
            hive.py          HiveSinkConfig
            local.py         LocalFileSinkConfig
            memory.py        InMemorySinkConfig
    variables/               Typed data slots that persist across task boundaries
        _private/
            base.py          Variable ABC (path, value, _save/_load_value_using_io)
            dataset.py       DatasetVariable(Variable) — pandas/spark/ray
        types.py             ModelArtifact, AssembledModel, PusherResult
    tasks/
        functions/           Shared helpers called by task plugins (not tasks themselves)
            sinks/           DataSink ABC + implementations (HiveSink, LocalFileSink, InMemorySink)
        pusher/              One specific task — plugins, config, registry, dispatch
            plugins/
                base.py      PusherPluginBase ABC
                dataset_plugin.py
                model_plugin.py
                eval_report_plugin.py
```

**Key rules:**
- `schema/` = contracts only. No implementation logic.
- `variables/` = data types that cross task boundaries (like function arguments in a DAG).
- `tasks/functions/` = shared utilities used by multiple task plugins. Not tasks themselves.
- `tasks/<task_name>/` = the actual task orchestration (plugins, registry, dispatch function).
- Never put shared helpers inside a specific task folder.

---

## Config/Implementation Separation

Every sink (and by extension, every configurable component) follows a two-layer pattern
learned from the internal `schema/v2alpha1/data_sink.py`:

```
schema/sinks/<name>.py       — typed config dataclass (validated at definition time)
tasks/functions/sinks/<name>.py — implementation that accepts the config
```

```python
# Config (schema layer — validated before any I/O)
@dataclass
class HiveSinkConfig:
    database: str
    table: str
    mode: str = "overwrite"
    partition_by: list[str] = field(default_factory=list)

    def __post_init__(self) -> None:
        """Validate mode on construction."""
        if self.mode not in {"overwrite", "append", "ignore", "error"}:
            raise ValueError(f"Invalid mode {self.mode!r}.")

# Implementation (tasks/functions layer — I/O happens here)
class HiveSink(DataSink):
    def __init__(self, config: HiveSinkConfig) -> None:
        """Initialise with the typed Hive sink config."""
        self._config = config

    def write(self, artifact: DatasetVariable) -> SinkResult:
        ...
```

**Why:** Validation at config-construction time (not at write time) catches mistakes
before the pipeline runs. Config objects are also serialisable and inspectable by
workflow engines.

---

## Variable Pattern (`Variable` base class)

Data that flows between tasks subclasses `Variable` from `_private/base.py`:

```python
class DatasetVariable(Variable):
    def __init__(self, value=None, path=None, metadata=None):
        if path is None:
            path = f"{os.environ.get('UF_STORAGE_URL', 'memory://storage')}/{uuid.uuid4().hex}"
        super().__init__(path=path, metadata=metadata)
        self._value = value

    def save(self):
        # dispatch to save_pandas_dataframe / save_spark_dataframe / save_ray_dataset
        ...

    def save_pandas_dataframe(self):
        self._save_value_using_io(PandasIO)   # inherited from Variable

    def load_pandas_dataframe(self):
        self._load_value_using_io(PandasIO)   # inherited from Variable
```

**"Variable" vs "Artifact":**
- `Variable` = typed slot in the pipeline DAG. Mutable path + value. Persisted across tasks.
- `Artifact` = immutable output (MLflow/Kubeflow terminology). Do not use for pipeline-internal data.

---

## `_private/` Convention

Implementation details that users should not import directly go in `_private/`:

```python
# Users import from the package __init__:
from michelangelo.workflow.variables import DatasetVariable

# __init__.py re-exports unconditionally:
from michelangelo.workflow.variables._private.dataset import DatasetVariable
__all__ = ["DatasetVariable", ...]
```

Use `contextlib.suppress(ImportError)` in `__init__` **only** when the class requires
an optional dependency (e.g. `RayTask` requires `ray`):

```python
with contextlib.suppress(ImportError):
    from michelangelo.uniflow.plugins.ray.task import RayTask
```

For OS-only code with no optional deps, use unconditional import + `__all__`.

---

## IO Pattern (uniflow plugins)

Persistence uses the IO registry pattern. Each IO class handles one backend:

```python
class PandasIO(IO[pd.DataFrame]):
    def write(self, url: str, value: pd.DataFrame) -> dict[str, Any]:
        # writes part-*.parquet directory via PyArrow + fsspec
        ...
    def read(self, url: str, metadata: dict[str, Any]) -> pd.DataFrame:
        ...
```

The `Variable` base class calls IO via `_save_value_using_io(IOClass)` and
`_load_value_using_io(IOClass)`. Never call IO classes directly from plugin code.

IO classes live in `uniflow/plugins/<backend>/io.py`:
- `uniflow/plugins/pandas/io.py` — `PandasIO`
- `uniflow/plugins/spark/io.py` — `SparkIO`
- `uniflow/plugins/ray/io.py` — `RayDatasetIO`

---

## DataSink ABC

```python
# workflow/tasks/functions/sinks/base.py
class DataSink(ABC):
    @abstractmethod
    def write(self, artifact: DatasetVariable) -> SinkResult: ...
```

```python
# workflow/schema/sinks/result.py
@dataclass(frozen=True)
class SinkResult:
    uri: str
    num_records: int
    extra: dict[str, Any] = field(default_factory=dict)
```

**Where each piece lives:**
- `DataSink` ABC → `tasks/functions/sinks/base.py` (with implementations — it IS an implementation interface)
- `SinkResult` → `schema/sinks/result.py` (it IS a data contract — the return type spec)
- Config dataclasses → `schema/sinks/`
- Implementations → `tasks/functions/sinks/`

---

## Imports

Always use absolute imports:

```python
from michelangelo.workflow.schema.sinks import HiveSinkConfig, SinkResult
from michelangelo.workflow.tasks.functions.sinks import DataSink, HiveSink
from michelangelo.workflow.variables import DatasetVariable
```

Use `TYPE_CHECKING` guard for annotation-only imports (avoids circular imports
and satisfies TC001):

```python
from __future__ import annotations
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from michelangelo.workflow.schema.sinks.hive import HiveSinkConfig
    from michelangelo.workflow.variables import DatasetVariable
```

Only use `TYPE_CHECKING` when the import is **annotation-only**. If the class is
instantiated at runtime (e.g. `SinkResult(...)`), it must be a top-level import.

---

## Logging

```python
_logger = logging.getLogger(__name__)   # underscore prefix, module-level
```

Never use `log = ...` (conflicts with stdlib `math.log`). Never use bare `print`.

---

## Testing Conventions

- Framework: `unittest.TestCase` (not pytest fixtures)
- Naming: `test_*.py` in `workflow/schema/tests/` and `tasks/pusher/tests/`
- Method docstrings: `"""It raises TypeError when ..."""` (third-person "It")
- Mocking: `unittest.mock.patch` / `MagicMock`
- Optional deps (pyspark, ray): mock via `patch.dict(sys.modules, {...})`
- Tests live alongside the code they test, not in a separate top-level `tests/` folder

```python
class TestHiveSink(TestCase):
    def test_writes_to_hive_table(self):
        """It calls spark_df.write.mode(mode).saveAsTable(database.table)."""
        mock_sql, spark_df = self._make_spark_df()
        sink = HiveSink(HiveSinkConfig(database="ml", table="features"))
        with patch.dict(sys.modules, self._pyspark_mods(mock_sql)):
            sink.write(artifact)
        spark_df.write.mode.assert_called_once_with("overwrite")
```

---

## Common Pitfalls

| Pitfall | Fix |
|---|---|
| `IO[Dataset]` at class definition time | `IO[Any]` — `Dataset` type not available without ray installed |
| Optional dep import at module level | Lazy import inside method body, or `contextlib.suppress(ImportError)` |
| `sum(result, [])` to flatten lists | `[item for chunk in result for item in chunk]` — RUF017 (quadratic) |
| `# flake8: noqa:F401` on imports | Use `__all__` — ruff does not recognise flake8 noqa directives |
| `DataSink` in `schema/` | `DataSink` is an implementation interface → `tasks/functions/sinks/base.py` |
| Sink implementation as top-level `workflow/sinks/` | Sinks are only used by tasks → `workflow/tasks/functions/sinks/` |
| Config validation inside `write()` | Validate in `Config.__post_init__` — fail at definition, not at execution |
| `sinks: list[DataSink] = []` default | Use `None` sentinel — `[]` prevents detecting "not provided" vs "explicitly empty" |
