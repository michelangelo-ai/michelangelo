"""Tests for the tabular_inference task."""

from __future__ import annotations

import shutil
from unittest import TestCase
from unittest.mock import patch

import pytest

import michelangelo.workflow.tasks.tabular_inference as tabular_inference_package
from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend
from michelangelo.lib.model_manager.registry.client import InMemoryRegistryClient
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.workflow.schema.evaluator import (
    ColumnMapping,
    EvaluatorConfig,
    MetricConfig,
)
from michelangelo.workflow.schema.inference import (
    BatchOptions,
    ModelIdentifier,
    OfflineInferenceConfig,
    RayDataContextConfig,
    SamplingConfig,
    TabularInferenceConfig,
    WriteConfig,
)
from michelangelo.workflow.tasks.evaluator.task import evaluate
from michelangelo.workflow.tasks.tabular_inference.task import tabular_inference
from michelangelo.workflow.tasks.tabular_inference.tests.fixtures import (
    PositionalModel,
    custom_raw,
    torch_deployable,
)
from michelangelo.workflow.variables import DatasetVariable
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
_TORCH_LABELS = {"training_framework": TRAINING_FRAMEWORK_PYTORCH}


class TabularInferenceTaskTest(TestCase):
    """The task is a thin wrapper over ``tabular_inference_impl``."""

    @patch("michelangelo.workflow.tasks.tabular_inference.task.tabular_inference_impl")
    def test_tabular_inference_delegates_to_impl(self, mock_impl):
        """Every argument is forwarded to the implementation."""
        config = TabularInferenceConfig(
            local_models=[OfflineInferenceConfig(input_columns=["f1"])]
        )
        assembled = AssembledModel(raw_model=ModelArtifact(path="raw"))
        datasets = {"train": DatasetVariable.create(None)}
        backend, registry = LocalStorageBackend("/tmp"), InMemoryRegistryClient()
        mock_impl.return_value = {"train": DatasetVariable.create(None)}

        result = tabular_inference(
            config,
            datasets,
            assembled,
            storage_backend=backend,
            registry_client=registry,
        )

        mock_impl.assert_called_once_with(
            config,
            datasets,
            assembled,
            storage_backend=backend,
            registry_client=registry,
        )
        self.assertIs(result, mock_impl.return_value)

    def test_package_exports_the_task(self):
        """The package exports the task function and its config types."""
        self.assertIs(tabular_inference_package.tabular_inference, tabular_inference)
        self.assertIs(tabular_inference_package.ModelIdentifier, ModelIdentifier)


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
        deployable_model=ModelArtifact(path=deployable_path),
    )


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


def test_scores_with_assembled_model_deployable(ray_session, tmp_path, out_dir):
    """A PyTorch AssembledModel is scored from its deployable package."""
    assembled = _assembled(_package(tmp_path / "pkg"))

    result = tabular_inference(
        _config(), {"train": _dataset_var(tmp_path)}, assembled_model=assembled
    )

    _assert_scored(result["train"])


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


def test_scores_with_registry_resolved_model(ray_session, tmp_path, out_dir):
    """A registry reference is looked up, downloaded and scored."""
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
        _config(model=ModelIdentifier(name="housing")),
        {"train": _dataset_var(tmp_path)},
        storage_backend=storage,
        registry_client=registry,
    )

    _assert_scored(result["train"])


def test_registry_deployable_pushed_as_a_tar_archive(ray_session, tmp_path, out_dir):
    """A deployable uploaded as model.tar is unpacked on the worker and scored."""
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
        _config(model=ModelIdentifier(name="housing")),
        {"train": _dataset_var(tmp_path)},
        storage_backend=storage,
        registry_client=registry,
    )

    _assert_scored(result["train"])


def test_remote_assembled_package_is_downloaded(ray_session, tmp_path, out_dir):
    """A deployable package recorded as a storage URI is fetched by the storage."""
    package = _package(tmp_path / "pkg")

    class FakeRemote(LocalStorageBackend):
        def download(self, uri, local_path):
            assert uri == "fake://bucket/deployable"
            shutil.copytree(package, local_path, dirs_exist_ok=True)

    result = tabular_inference(
        _config(),
        {"train": _dataset_var(tmp_path)},
        assembled_model=_assembled("fake://bucket/deployable"),
        storage_backend=FakeRemote(base_dir=str(tmp_path)),
    )

    _assert_scored(result["train"])


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


def test_none_dataset_is_left_out(ray_session, tmp_path, out_dir):
    """A None dataset is not scored and the others are."""
    assembled = _assembled(_package(tmp_path / "pkg"))
    result = tabular_inference(
        _config(),
        {"train": _dataset_var(tmp_path), "test": None},
        assembled_model=assembled,
    )
    assert "test" not in result
    _assert_scored(result["train"])


def test_no_models_saves_the_input_datasets(ray_session, tmp_path, out_dir):
    """With no local_models the datasets are returned as saved results."""
    result = tabular_inference(
        TabularInferenceConfig(), {"train": _dataset_var(tmp_path)}
    )

    assert len(_rows(result["train"])) == 6


def test_write_config_is_applied(ray_session, tmp_path, out_dir):
    """write_config options reach the dataset write."""
    assembled = _assembled(_package(tmp_path / "pkg"))
    config = _config()
    config.write_config = WriteConfig(max_rows_per_file=2)

    result = tabular_inference(
        config, {"train": _dataset_var(tmp_path)}, assembled_model=assembled
    )

    assert len(list(out_dir.rglob("*.parquet"))) >= 3
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
        "michelangelo.workflow.tasks.tabular_inference.offline.task_impl."
        "set_ray_data_context"
    ) as set_context:
        tabular_inference(config, {})

    assert set_context.call_args.kwargs["max_errored_blocks"] == 7
    assert set_context.call_args.kwargs["preserve_order"] is True
