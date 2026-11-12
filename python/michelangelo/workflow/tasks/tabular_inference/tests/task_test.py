"""Tests for the tabular_inference task."""

from __future__ import annotations

from unittest.mock import patch

import pytest

from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend
from michelangelo.lib.inference.tests.fixtures import (
    DictModel,
    PositionalModel,
    custom_raw,
    torch_deployable,
)
from michelangelo.lib.model_manager.registry.client import InMemoryRegistryClient
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.workflow.schema.evaluator import (
    ColumnMapping,
    EvaluatorConfig,
    MetricConfig,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.inference import (
    BatchOptions,
    ModelReference,
    OfflineInferenceConfig,
    RayDataContextConfig,
    SamplingConfig,
    TabularInferenceConfig,
)
from michelangelo.workflow.schema.ray_data_io import WriteConfig
from michelangelo.workflow.tasks.evaluator.task import evaluate
from michelangelo.workflow.tasks.tabular_inference import tabular_inference
from michelangelo.workflow.tasks.tabular_inference._private.model_source import (
    resolve_model_source,
)
from michelangelo.workflow.variables import DatasetVariable, ModelVariable
from michelangelo.workflow.variables._private.utils.serialization import save_object
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_CUSTOM,
    TRAINING_FRAMEWORK_PYTORCH,
    ModelMetadata,
)
from michelangelo.workflow.variables.types import AssembledModel, ModelArtifact

_SCHEMA = ModelSchema(
    input_schema=[
        ModelSchemaItem("a", DataType.FLOAT, [1]),
        ModelSchemaItem("b", DataType.FLOAT, [1]),
    ],
    output_schema=[ModelSchemaItem("y", DataType.FLOAT, [1])],
)
_ROWS = [{"a": float(i), "b": 10.0, "label": 10.0 + 2 * i} for i in range(6)]


@pytest.fixture(scope="module")
def ray_session():
    """One shared local Ray session for the module."""
    import ray

    ray.init(num_cpus=2, include_dashboard=False, ignore_reinit_error=True)
    yield
    ray.shutdown()


@pytest.fixture
def out_dir(tmp_path, monkeypatch):
    """Send task outputs to a temp directory."""
    path = tmp_path / "out"
    path.mkdir()
    monkeypatch.setenv("UF_STORAGE_URL", str(path))
    return path


def _dataset_var(tmp_path, name="in", rows=None):
    """A saved dataset that the task loads from disk, like a real pipeline."""
    import ray.data

    path = str(tmp_path / name)
    DatasetVariable.create(ray.data.from_items(rows or _ROWS), path=path).save()
    return DatasetVariable(path=path)


def _package(root):
    """A TorchScript deployable package that scores ``2a + b``.

    Built by the real packager, as the assembler does.
    """
    return torch_deployable(root, PositionalModel(), _SCHEMA)


def _assembled(
    deployable_path,
    raw_path="s3://bucket/raw-is-never-read",
    framework=TRAINING_FRAMEWORK_PYTORCH,
):
    """An assembler output, which records the framework on the raw model.

    The default raw path is a URI nothing can download, which proves a PyTorch
    model is not scored from its raw package.
    """
    return AssembledModel(
        raw_model=ModelArtifact(
            path=raw_path, metadata=ModelMetadata(training_framework=framework)
        ),
        deployable_model=(
            ModelArtifact(path=deployable_path) if deployable_path is not None else None
        ),
    )


_TORCH_LABELS = {"training_framework": TRAINING_FRAMEWORK_PYTORCH}


def _config(**model_kwargs):
    return TabularInferenceConfig(
        local_models=[
            OfflineInferenceConfig(
                batch_options=BatchOptions(batch_size=3, concurrency=1),
                **model_kwargs,
            )
        ]
    )


def _rows(var):
    var.load_ray_dataset()
    return sorted(var.value.take_all(), key=lambda r: r["label"])


def _assert_scored(var):
    rows = _rows(var)
    assert [r["y"] for r in rows] == [r["label"] for r in rows]
    assert {"a", "b", "label", "y"} <= set(rows[0])


def test_scores_with_in_process_model_variable(ray_session, tmp_path, out_dir):
    """A ModelVariable is scored in-process and its schema squeezes the output."""
    model = ModelVariable.create(DictModel())
    model.metadata._schema = save_object(_SCHEMA)

    result = tabular_inference(
        _config(), {"train": _dataset_var(tmp_path)}, assembled_model=model
    )

    _assert_scored(result["train"])


def test_scores_with_assembled_model_deployable(ray_session, tmp_path, out_dir):
    """A PyTorch AssembledModel is scored from its deployable package."""
    assembled = _assembled(_package(tmp_path / "pkg"))

    result = tabular_inference(
        _config(), {"train": _dataset_var(tmp_path)}, assembled_model=assembled
    )

    _assert_scored(result["train"])


def test_scores_with_registry_resolved_model(ray_session, tmp_path, out_dir):
    """A registry reference is looked up, downloaded and scored, with metadata."""
    storage = LocalStorageBackend(base_dir=str(tmp_path / "store"))
    (tmp_path / "store").mkdir()
    uri = storage.upload(_package(tmp_path / "pkg"), "models/housing")
    registry = InMemoryRegistryClient()
    registry.register_model(
        "housing",
        artifact_uri="s3://raw-is-never-read",
        deployable_artifact_uri=uri,
        labels=_TORCH_LABELS,
    )

    result = tabular_inference(
        _config(model=ModelReference(name="housing")),
        {"train": _dataset_var(tmp_path)},
        storage_backend=storage,
        registry_client=registry,
    )

    rows = _rows(result["train"])
    assert [r["y"] for r in rows] == [r["label"] for r in rows]
    assert {r["model_name"] for r in rows} == {"housing"}
    assert {r["model_version"] for r in rows} == {"1"}


def test_output_feeds_the_evaluator_unchanged(ray_session, tmp_path, out_dir):
    """The scored datasets go straight into evaluate(), prediction is a scalar."""
    assembled = _assembled(_package(tmp_path / "pkg"))
    result = tabular_inference(
        _config(), {"test": _dataset_var(tmp_path)}, assembled_model=assembled
    )
    config = EvaluatorConfig(
        metrics=[
            MetricConfig(
                name="mse",
                metric="torchmetrics.regression.MeanSquaredError",
                columns=ColumnMapping(prediction_col="y", target_col="label"),
            )
        ]
    )

    with patch("michelangelo.workflow.tasks.evaluator.task._save_summary") as save:
        evaluation = evaluate(config, result)

    assert list(evaluation.metrics) == ["test"]
    assert save.call_args.args[0]["mse"].iloc[0] == pytest.approx(0.0, abs=1e-9)


def test_none_dataset_is_passed_through(ray_session, tmp_path, out_dir):
    """A None dataset stays None and the others are scored."""
    assembled = _assembled(_package(tmp_path / "pkg"))
    result = tabular_inference(
        _config(),
        {"train": _dataset_var(tmp_path), "test": None},
        assembled_model=assembled,
    )
    assert result["test"] is None
    _assert_scored(result["train"])


def test_empty_dataset_is_returned_as_it_came(ray_session, tmp_path, out_dir):
    """An empty dataset is not scored or rewritten."""
    import pyarrow as pa
    import pyarrow.parquet as pq

    empty_dir = tmp_path / "e"
    empty_dir.mkdir()
    schema = pa.schema(
        [("a", pa.float64()), ("b", pa.float64()), ("label", pa.float64())]
    )
    pq.write_table(schema.empty_table(), str(empty_dir / "part-0.parquet"))
    empty_var = DatasetVariable(path=str(empty_dir))
    assembled = _assembled(_package(tmp_path / "pkg"))

    result = tabular_inference(
        _config(), {"test": empty_var}, assembled_model=assembled
    )

    assert result["test"] is empty_var


def test_no_models_returns_datasets_unchanged():
    """An empty local_models returns the same dict without touching Ray."""
    datasets = {"train": None}
    assert tabular_inference(TabularInferenceConfig(), datasets) is datasets
    assert (
        tabular_inference(TabularInferenceConfig(local_models=[]), datasets) is datasets
    )


def test_write_config_is_applied(ray_session, tmp_path, out_dir):
    """write_config options reach the dataset write."""
    assembled = _assembled(_package(tmp_path / "pkg"))
    config = _config()
    config.write_config = WriteConfig(max_rows_per_file=2)

    result = tabular_inference(
        config, {"train": _dataset_var(tmp_path)}, assembled_model=assembled
    )

    parquet_files = list((out_dir).rglob("*.parquet"))
    assert len(parquet_files) >= 3
    assert len(_rows(result["train"])) == 6


def test_sampling_scores_only_the_sampled_rows(ray_session, tmp_path, out_dir):
    """Only the sampled fraction of a dataset is scored and written."""
    rows = [{"a": float(i), "b": 10.0, "label": 10.0 + 2 * i} for i in range(200)]
    config = _config()
    config.sampling = SamplingConfig(fractions={"train": 0.25})

    result = tabular_inference(
        config,
        {"train": _dataset_var(tmp_path, rows=rows)},
        assembled_model=_assembled(_package(tmp_path / "pkg")),
    )

    scored = _rows(result["train"])
    assert 0 < len(scored) < 200
    assert [r["y"] for r in scored] == [r["label"] for r in scored]


def test_ray_data_context_options_reach_ray_data(ray_session):
    """Task-level and context-level options are applied before scoring."""
    config = _config()
    config.max_errored_blocks = 7
    config.ray_data_context = RayDataContextConfig(preserve_order=True)
    with patch(
        "michelangelo.workflow.tasks.tabular_inference.task.set_ray_data_context"
    ) as set_context:
        tabular_inference(
            config, {"train": None}, assembled_model=ModelVariable.create(DictModel())
        )

    assert set_context.call_args.kwargs["max_errored_blocks"] == 7
    assert set_context.call_args.kwargs["preserve_order"] is True


def test_bad_write_config_fails_before_scoring(tmp_path):
    """An unsupported write option is reported before any model is loaded."""
    config = _config()
    config.write_config = WriteConfig(max_rows_per_file=2)
    with (
        patch("michelangelo.workflow.tasks.functions.data.ray.write.ray") as mock_ray,
        patch(
            "michelangelo.workflow.tasks.tabular_inference.task.resolve_model_source"
        ) as resolve,
    ):
        mock_ray.__version__ = "2.40.0"
        with pytest.raises(ConfigurationError, match="max_rows_per_file"):
            tabular_inference(config, {"train": None}, assembled_model=object())
    resolve.assert_not_called()


# --- model resolution errors ------------------------------------------------


def test_no_model_source_names_both_ways_to_fix(tmp_path):
    """Without a model argument or a registry reference the error says what to set."""
    with pytest.raises(
        ConfigurationError, match=r"local_models\[0\]\.model.*assembled_model"
    ):
        tabular_inference(_config(), {"train": None})


def test_registry_reference_needs_registry_client(tmp_path):
    """A reference without a client names the missing argument."""
    with pytest.raises(ConfigurationError, match="registry_client"):
        tabular_inference(
            _config(model=ModelReference(name="m")),
            {"train": None},
            storage_backend=LocalStorageBackend(str(tmp_path)),
        )


def test_registry_reference_needs_storage_backend():
    """A reference without a backend names the missing argument."""
    with pytest.raises(ConfigurationError, match="storage_backend"):
        tabular_inference(
            _config(model=ModelReference(name="m")),
            {"train": None},
            registry_client=InMemoryRegistryClient(),
        )


def test_registry_entry_without_deployable_is_rejected(tmp_path):
    """A registered PyTorch model with only a raw package is not scored from it."""
    registry = InMemoryRegistryClient()
    registry.register_model("m", artifact_uri="s3://bucket/raw", labels=_TORCH_LABELS)
    with pytest.raises(ConfigurationError, match="no deployable_artifact_uri"):
        tabular_inference(
            _config(model=ModelReference(name="m")),
            {"train": None},
            storage_backend=LocalStorageBackend(str(tmp_path)),
            registry_client=registry,
        )


@pytest.mark.parametrize("deployable", [None, ""], ids=["missing", "empty-path"])
def test_model_without_deployable_is_rejected(tmp_path, deployable):
    """A PyTorch model with only a raw package is never silently scored from it."""
    assembled = _assembled(deployable, raw_path=str(tmp_path))
    with pytest.raises(ConfigurationError, match="no deployable package"):
        tabular_inference(_config(), {"train": None}, assembled_model=assembled)


def test_custom_assembled_model_is_scored_from_its_raw_package(
    ray_session, tmp_path, out_dir
):
    """A custom model uses the raw package, never the deployable."""
    schema = ModelSchema(
        input_schema=[ModelSchemaItem("a", DataType.FLOAT, [1])],
        output_schema=[ModelSchemaItem("response", DataType.FLOAT, [1])],
    )
    assembled = _assembled(
        "s3://bucket/deployable-is-never-read",
        raw_path=custom_raw(tmp_path / "custom", schema),
        framework=TRAINING_FRAMEWORK_CUSTOM,
    )

    result = tabular_inference(
        _config(), {"train": _dataset_var(tmp_path)}, assembled_model=assembled
    )

    rows = _rows(result["train"])
    assert [r["response"] for r in rows] == [r["a"] + 1 for r in rows]


@pytest.mark.parametrize("framework", [None, "xgboost"], ids=["missing", "unknown"])
def test_other_assembled_frameworks_use_the_raw_package(tmp_path, framework):
    """Only PyTorch and Lightning use the deployable package; the rest use raw."""
    raw = tmp_path / "raw"
    raw.mkdir()
    assembled = _assembled(
        "s3://bucket/deployable-is-never-used", raw_path=str(raw), framework=framework
    )

    source = resolve_model_source(
        OfflineInferenceConfig(),
        assembled,
        0,
        storage_backend=None,
        registry_client=None,
    )

    assert source.model_path == str(raw)


@pytest.mark.parametrize("framework", [None, "xgboost"], ids=["missing", "unknown"])
def test_non_custom_registry_models_use_the_deployable_artifact(tmp_path, framework):
    """Only a custom registered model uses the raw artifact."""
    registry = InMemoryRegistryClient()
    labels = {"training_framework": framework} if framework else {}
    registry.register_model(
        "m", artifact_uri="s3://r", deployable_artifact_uri="s3://d", labels=labels
    )

    source = resolve_model_source(
        OfflineInferenceConfig(model=ModelReference(name="m")),
        None,
        0,
        storage_backend=LocalStorageBackend(str(tmp_path)),
        registry_client=registry,
    )

    assert source.uri == "s3://d"


def test_registry_deployable_pushed_as_a_tar_archive(ray_session, tmp_path, out_dir):
    """A deployable uploaded as model.tar is unpacked on the worker and scored."""
    import shutil

    archive = shutil.make_archive(
        str(tmp_path / "model"), "tar", _package(tmp_path / "pkg")
    )
    storage = LocalStorageBackend(base_dir=str(tmp_path / "store"))
    (tmp_path / "store").mkdir()
    uri = storage.upload(archive, "models/housing/deployable/model.tar")
    registry = InMemoryRegistryClient()
    registry.register_model(
        "housing",
        artifact_uri="s3://raw-is-never-read",
        deployable_artifact_uri=uri,
        labels=_TORCH_LABELS,
    )

    result = tabular_inference(
        _config(model=ModelReference(name="housing")),
        {"train": _dataset_var(tmp_path)},
        storage_backend=storage,
        registry_client=registry,
    )

    _assert_scored(result["train"])


def test_unsupported_model_type_is_rejected():
    """Anything else passed as the model is a TypeError."""
    with pytest.raises(TypeError, match="ModelVariable or AssembledModel"):
        tabular_inference(_config(), {"train": None}, assembled_model="nope")


def test_remote_assembled_package_is_downloaded_by_the_workers(
    ray_session, tmp_path, out_dir
):
    """A deployable package recorded as a storage URI is fetched on the worker."""
    import shutil

    package = _package(tmp_path / "pkg")

    class FakeRemote(LocalStorageBackend):
        def download(self, uri, local_path):
            assert uri == "fake://bucket/deployable"
            shutil.copytree(package, local_path, dirs_exist_ok=True)

    assembled = _assembled("fake://bucket/deployable")
    result = tabular_inference(
        _config(),
        {"train": _dataset_var(tmp_path)},
        assembled_model=assembled,
        storage_backend=FakeRemote(base_dir=str(tmp_path)),
    )

    _assert_scored(result["train"])


def test_remote_sources_are_not_downloaded_on_the_driver(tmp_path):
    """Resolving a remote model records the URI and downloads nothing."""

    class Spy(LocalStorageBackend):
        def download(self, uri, local_path):
            raise AssertionError("downloaded on the driver")

    storage = Spy(base_dir=str(tmp_path))
    registry = InMemoryRegistryClient()
    registry.register_model(
        "m",
        artifact_uri="s3://bucket/raw",
        deployable_artifact_uri="s3://bucket/m",
        labels=_TORCH_LABELS,
    )
    registry.register_model(
        "c",
        artifact_uri="s3://bucket/c-raw",
        deployable_artifact_uri="s3://bucket/c",
        labels={"training_framework": TRAINING_FRAMEWORK_CUSTOM},
    )

    from_registry = resolve_model_source(
        OfflineInferenceConfig(model=ModelReference(name="m")),
        None,
        0,
        storage_backend=storage,
        registry_client=registry,
    )
    from_assembled = resolve_model_source(
        OfflineInferenceConfig(),
        _assembled("s3://bucket/deployable"),
        0,
        storage_backend=storage,
        registry_client=None,
    )

    assert (from_registry.uri, from_registry.name, from_registry.version) == (
        "s3://bucket/m",
        "m",
        "1",
    )
    assert from_registry.storage_backend is storage
    assert from_assembled.uri == "s3://bucket/deployable"
    assert from_assembled.model_path is None
    custom = resolve_model_source(
        OfflineInferenceConfig(model=ModelReference(name="c")),
        None,
        0,
        storage_backend=storage,
        registry_client=registry,
    )
    assert custom.uri == "s3://bucket/c-raw"


def test_dataset_already_loaded_as_pandas_is_rejected_clearly(tmp_path):
    """A non-Ray dataset variable names the dataset and the problem."""
    import pandas as pd

    var = DatasetVariable.create(pd.DataFrame({"a": [1.0]}), path=str(tmp_path / "x"))
    model = ModelVariable.create(DictModel())
    with pytest.raises(TypeError, match=r"datasets\['train'\].*DataFrame"):
        tabular_inference(_config(), {"train": var}, assembled_model=model)


def test_remote_assembled_package_needs_storage_backend():
    """A URI that is not a local directory names the missing argument."""
    assembled = _assembled("s3://bucket/deployable")
    with pytest.raises(ConfigurationError, match="storage_backend"):
        tabular_inference(_config(), {"train": None}, assembled_model=assembled)
