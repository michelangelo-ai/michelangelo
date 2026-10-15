"""Tests for batch_model_inference."""

from __future__ import annotations

import dataclasses
from unittest.mock import MagicMock, patch

import pytest

from michelangelo.lib.inference import ModelSource, batch_model_inference
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.workflow.schema.inference import (
    BatchOptions,
    OfflineInferenceConfig,
    PaddingOptions,
)


def _dataset():
    ds = MagicMock(name="dataset")
    ds.limit.return_value.take.return_value = [{"a": 1}]
    ds.map_batches.return_value = MagicMock(name="scored")
    return ds


def _call(config):
    ds = _dataset()
    with patch("ray.data.ActorPoolStrategy") as pool:
        batch_model_inference({"train": ds}, [(config, ModelSource(model_path="/m"))])
    return ds.map_batches.call_args, pool


# One row per BatchOptions field: how a value reaches the map_batches call.
# Adding a field to BatchOptions without a row here fails the coverage test below.
_FORWARDING = {
    "batch_size": lambda call, pool, v: call.kwargs["batch_size"] == v,
    "num_gpus": lambda call, pool, v: call.kwargs["num_gpus"] == v,
    "num_cpus": lambda call, pool, v: (
        call.kwargs["num_cpus"] == v
        and call.kwargs["fn_constructor_kwargs"]["num_cpus"] == v
    ),
    "concurrency": lambda call, pool, v: pool.call_args.kwargs["size"] == v,
    "max_tasks_in_flight_per_actor": lambda call, pool, v: (
        pool.call_args.kwargs["max_tasks_in_flight_per_actor"] == v
    ),
    "max_concurrency": lambda call, pool, v: call.kwargs["max_concurrency"] == v,
    "max_restarts": lambda call, pool, v: call.kwargs["max_restarts"] == v,
    "max_task_retries": lambda call, pool, v: call.kwargs["max_task_retries"] == v,
}


def test_every_batch_option_has_a_forwarding_check():
    """New BatchOptions fields must be wired through and listed above."""
    assert {f.name for f in dataclasses.fields(BatchOptions)} == set(_FORWARDING)


@pytest.mark.parametrize("field", sorted(_FORWARDING))
def test_each_batch_option_reaches_map_batches(field):
    """Every option reaches the Ray call (guards against dropped kwargs)."""
    value = 0.5 if field == "num_gpus" else 3
    config = OfflineInferenceConfig(batch_options=BatchOptions(**{field: value}))
    call, pool = _call(config)
    assert _FORWARDING[field](call, pool, value)


def test_unset_actor_options_never_reach_ray():
    """None must not override Ray's own defaults."""
    call, pool = _call(OfflineInferenceConfig())
    for key in ("max_concurrency", "max_restarts", "max_task_retries"):
        assert key not in call.kwargs
    # An explicit pool is always passed; with nothing set it carries no options.
    pool.assert_called_once_with()
    assert call.kwargs["compute"] is pool.return_value


def test_zero_is_forwarded_not_treated_as_unset():
    """``max_restarts=0`` disables restarts and must reach Ray."""
    config = OfflineInferenceConfig(batch_options=BatchOptions(max_restarts=0))
    call, _ = _call(config)
    assert call.kwargs["max_restarts"] == 0


def test_constructor_kwargs_carry_model_and_config():
    """The actor receives the model source and the per-model config."""
    config = OfflineInferenceConfig(
        input_columns=["a"],
        columns_to_keep=["label"],
        padding_options=PaddingOptions(column_padding_values={"a": -1}),
        backend="pkg.factory",
    )
    ds = _dataset()
    source = ModelSource(model_path="/m", name="housing", version="2")
    batch_model_inference({"train": ds}, [(config, source)])
    kwargs = ds.map_batches.call_args.kwargs["fn_constructor_kwargs"]
    assert kwargs["input_columns"] == ["a"]
    assert kwargs["model_path"] == "/m"
    assert kwargs["model_uri"] is None
    assert kwargs["storage_backend"] is None
    assert kwargs["model"] is None
    assert kwargs["model_name"] == "housing"
    assert kwargs["model_version"] == "2"
    assert kwargs["columns_to_keep"] == ["label"]
    assert kwargs["column_padding_values"] == {"a": -1}
    assert kwargs["backend"] == "pkg.factory"


def test_empty_models_returns_datasets_unchanged():
    """No models means a passthrough."""
    datasets = {"train": _dataset()}
    assert batch_model_inference(datasets, []) is datasets


def test_none_and_empty_datasets_are_passed_through():
    """``None`` and empty datasets are not scored."""
    empty = _dataset()
    empty.limit.return_value.take.return_value = []
    out = batch_model_inference(
        {"none": None, "empty": empty},
        [(OfflineInferenceConfig(), ModelSource(model_path="/m"))],
    )
    assert out == {"none": None, "empty": empty}
    empty.map_batches.assert_not_called()


def test_multiple_models_are_unioned():
    """Several models produce the union of their scored datasets."""
    ds = _dataset()
    first, second = MagicMock(name="first"), MagicMock(name="second")
    ds.map_batches.side_effect = [first, second]
    models = [
        (OfflineInferenceConfig(), ModelSource(model_path="/m1")),
        (OfflineInferenceConfig(), ModelSource(model_path="/m2")),
    ]
    out = batch_model_inference({"train": ds}, models)
    first.union.assert_called_once_with(second)
    assert out["train"] is first.union.return_value


def _schema(name):
    return ModelSchema(output_schema=[ModelSchemaItem(name, DataType.FLOAT, [1])])


def test_models_with_different_output_columns_are_rejected():
    """Differently shaped outputs cannot be combined."""
    models = [
        (OfflineInferenceConfig(), ModelSource(model_path="/m1", schema=_schema("p"))),
        (OfflineInferenceConfig(), ModelSource(model_path="/m2", schema=_schema("q"))),
    ]
    with pytest.raises(ValueError, match="same output columns"):
        batch_model_inference({"train": _dataset()}, models)


def test_in_memory_model_is_shared_through_the_object_store():
    """A loaded model goes through ``ray.put`` once and is passed as a ref."""
    ds = _dataset()
    with patch("ray.put", return_value="REF") as put:
        batch_model_inference(
            {"train": ds},
            [(OfflineInferenceConfig(), ModelSource(model="MODEL"))],
        )
    put.assert_called_once_with("MODEL")
    assert ds.map_batches.call_args.kwargs["fn_constructor_kwargs"]["model"] == "REF"


# --- real Ray -------------------------------------------------------------


@pytest.fixture(scope="module")
def ray_session():
    """One shared local Ray session for the module."""
    import ray

    ray.init(num_cpus=2, include_dashboard=False, ignore_reinit_error=True)
    yield
    ray.shutdown()


def test_end_to_end_on_a_real_ray_dataset(ray_session):
    """Scores a real dataset with an in-memory model through actors."""
    import ray.data

    from michelangelo.lib.inference.tests.fixtures import DictModel

    schema = ModelSchema(
        input_schema=[
            ModelSchemaItem("a", DataType.FLOAT, [1]),
            ModelSchemaItem("b", DataType.FLOAT, [1]),
        ],
        output_schema=[ModelSchemaItem("y", DataType.FLOAT, [1])],
    )
    ds = ray.data.from_items([{"a": float(i), "b": 10.0, "label": i} for i in range(6)])
    config = OfflineInferenceConfig(
        batch_options=BatchOptions(batch_size=3, concurrency=1, num_cpus=2),
        columns_to_keep=["label"],
    )
    out = batch_model_inference(
        {"train": ds},
        [
            (
                config,
                ModelSource(model=DictModel(), schema=schema, name="m", version="1"),
            )
        ],
    )["train"]
    rows = sorted(out.take_all(), key=lambda r: r["label"])
    assert [r["y"] for r in rows] == [10.0 + 2 * i for i in range(6)]
    assert set(rows[0]) == {"label", "y", "model_name", "model_version"}
    assert rows[0]["model_name"] == "m"


def test_remote_source_forwards_uri_and_backend_to_the_worker():
    """A URI source is downloaded by the worker, never on the driver."""
    ds = _dataset()
    backend = object()
    batch_model_inference(
        {"train": ds},
        [
            (
                OfflineInferenceConfig(),
                ModelSource(uri="s3://bucket/model", storage_backend=backend),
            )
        ],
    )
    kwargs = ds.map_batches.call_args.kwargs["fn_constructor_kwargs"]
    assert kwargs["model_uri"] == "s3://bucket/model"
    assert kwargs["storage_backend"] is backend
    assert kwargs["model_path"] is None


def test_shared_in_memory_model_is_put_once_across_datasets_and_configs():
    """One model object costs one object-store copy, not one per pair."""
    datasets = {"train": _dataset(), "validation": _dataset(), "test": _dataset()}
    source = ModelSource(model="MODEL")
    with patch("ray.put", return_value="REF") as put:
        batch_model_inference(
            datasets,
            [(OfflineInferenceConfig(), source), (OfflineInferenceConfig(), source)],
        )
    put.assert_called_once_with("MODEL")
    for ds in datasets.values():
        for call in ds.map_batches.call_args_list:
            assert call.kwargs["fn_constructor_kwargs"]["model"] == "REF"


def test_output_column_order_does_not_matter_when_combining_models():
    """The same output columns in a different order can still be combined."""
    first = ModelSchema(
        output_schema=[
            ModelSchemaItem("p", DataType.FLOAT, [1]),
            ModelSchemaItem("q", DataType.FLOAT, [1]),
        ]
    )
    second = ModelSchema(
        output_schema=[
            ModelSchemaItem("q", DataType.FLOAT, [1]),
            ModelSchemaItem("p", DataType.FLOAT, [1]),
        ]
    )
    models = [
        (OfflineInferenceConfig(), ModelSource(model_path="/a", schema=first)),
        (OfflineInferenceConfig(), ModelSource(model_path="/b", schema=second)),
    ]
    batch_model_inference({"train": _dataset()}, models)
