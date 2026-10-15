"""Tests for RawModelBackend, create_raw_backend and load_model_schema."""

from __future__ import annotations

import numpy as np
import pytest
import torch
import yaml

from michelangelo.lib.inference import (
    BackendContext,
    RawModelBackend,
    build_backend,
    create_raw_backend,
    resolve_backend,
)
from michelangelo.lib.inference.raw_backend import DEFAULT_OUTPUT_NAME
from michelangelo.lib.inference.tests.fixtures import (
    DictModel,
    DictOutputModel,
    EchoModel,
    NamedTupleModel,
    Scale,
    SumPredictor,
    TupleModel,
)
from michelangelo.lib.model_manager._private.schema.common.serde import schema_to_yaml
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.lib.model_manager.serde.model import load_model_schema
from michelangelo.lib.shared.utils.model_fuser.fused_model import FusedModel

_FIX = "michelangelo.lib.inference.tests.fixtures"


def _schema(inputs=("a", "b"), outputs=("y",)) -> ModelSchema:
    return ModelSchema(
        input_schema=[ModelSchemaItem(n, DataType.FLOAT, [1]) for n in inputs],
        output_schema=[ModelSchemaItem(n, DataType.FLOAT, [1]) for n in outputs],
    )


def _batch():
    return {
        "a": np.array([1.0, 2.0], dtype=np.float32),
        "b": np.array([10.0, 20.0], dtype=np.float32),
    }


def test_single_tensor_output_named_by_schema():
    """One tensor output takes the schema's output name."""
    backend = RawModelBackend(DictModel(), schema=_schema())
    out = backend.predict(_batch())
    assert list(out) == ["y"]
    np.testing.assert_allclose(out["y"], [12.0, 24.0])


def test_single_tensor_output_default_name_without_schema():
    """Without a schema the output is called ``prediction``."""
    backend = RawModelBackend(DictModel())
    assert backend.input_names is None
    assert backend.output_names is None
    assert list(backend.predict(_batch())) == [DEFAULT_OUTPUT_NAME]


def test_inputs_are_fed_by_name_not_position():
    """Reordering the input dict does not change the result."""
    backend = RawModelBackend(DictModel(), schema=_schema())
    forward = backend.predict(_batch())
    reordered = backend.predict({"b": _batch()["b"], "a": _batch()["a"]})
    np.testing.assert_array_equal(forward["y"], reordered["y"])


def test_names_come_from_schema():
    """input_names and output_names mirror the schema order."""
    backend = RawModelBackend(DictModel(), schema=_schema(("b", "a"), ("y",)))
    assert backend.input_names == ["b", "a"]
    assert backend.output_names == ["y"]


def test_empty_schema_sections_mean_unknown_names():
    """A schema with empty sections reports unknown names."""
    backend = RawModelBackend(DictModel(), schema=ModelSchema())
    assert backend.input_names is None
    assert backend.output_names is None


def test_tuple_outputs_zip_with_schema_names():
    """A tuple output is named positionally from the schema."""
    backend = RawModelBackend(TupleModel(), schema=_schema(outputs=("up", "down")))
    out = backend.predict(_batch())
    np.testing.assert_allclose(out["up"], [2.0, 3.0])
    np.testing.assert_allclose(out["down"], [0.0, 1.0])


def test_tuple_outputs_with_wrong_name_count_raises():
    """A name count that does not match the output count is an error."""
    backend = RawModelBackend(TupleModel(), schema=_schema(outputs=("only",)))
    with pytest.raises(ValueError, match="2 output"):
        backend.predict(_batch())


def test_named_tuple_uses_field_names_without_schema():
    """A NamedTuple names its own outputs when there is no schema."""
    out = RawModelBackend(NamedTupleModel()).predict(_batch())
    assert set(out) == {"low", "high"}


def test_dict_outputs_keep_their_keys():
    """A dict output is converted key by key."""
    out = RawModelBackend(DictOutputModel(), schema=_schema()).predict(_batch())
    np.testing.assert_allclose(out["double"], [2.0, 4.0])


def test_custom_model_is_called_with_numpy_dict():
    """A custom Model gets numpy in and returns numpy out."""
    backend = RawModelBackend(EchoModel(), schema=_schema(("a",), ("response",)))
    out = backend.predict(_batch())
    np.testing.assert_array_equal(out["response"], _batch()["a"])


def test_rejects_unsupported_model_type():
    """Anything that is not a Model or nn.Module is refused."""
    with pytest.raises(TypeError, match="str"):
        RawModelBackend("not a model")


def test_module_is_moved_to_device_and_set_to_eval():
    """The module is on the requested device in eval mode."""
    module = DictModel()
    module.train()
    RawModelBackend(module, device="cpu")
    assert not module.training


def test_fused_model_runs_transform_then_predictor():
    """A transform fused with a predictor scores raw inputs by name."""
    fused = FusedModel(
        transform_module=Scale(),
        predictor_module=SumPredictor(),
        transform_input_keys=["a"],
        predictor_input_keys=["a", "b"],
        predictor_takes_dict=True,
    )
    out = RawModelBackend(fused, schema=_schema()).predict(_batch())
    np.testing.assert_allclose(out["y"], [12.0, 24.0])


# --- real packages on disk -------------------------------------------------


def _write_torch_package(root, schema: ModelSchema | None):
    for d in ("model", "defs", "metadata"):
        (root / d).mkdir()
    torch.save(DictModel().state_dict(), root / "model" / "model.pt")
    (root / "defs" / "model_class.txt").write_text(f"{_FIX}.DictModel")
    (root / "metadata" / "type.yaml").write_text("type: torch\n")
    if schema is not None:
        (root / "metadata" / "schema.yaml").write_text(schema_to_yaml(schema))
    return str(root)


def test_create_raw_backend_from_torch_package(tmp_path):
    """A torch package loads with its stored schema and scores a batch."""
    path = _write_torch_package(tmp_path, _schema())
    backend = build_backend(resolve_backend(path), BackendContext(path))
    assert backend.input_names == ["a", "b"]
    out = backend.predict(_batch())
    np.testing.assert_allclose(out["y"], [12.0, 24.0])


def test_create_raw_backend_prefers_context_schema(tmp_path):
    """A schema passed in the context wins over the stored one."""
    path = _write_torch_package(tmp_path, _schema())
    ctx = BackendContext(path, schema=_schema(outputs=("override",)))
    assert create_raw_backend(ctx).output_names == ["override"]


def test_create_raw_backend_without_stored_schema(tmp_path):
    """A package with no schema file still loads, with unknown names."""
    path = _write_torch_package(tmp_path, None)
    assert create_raw_backend(BackendContext(path)).output_names is None


def test_create_raw_backend_from_custom_package(tmp_path):
    """A custom-python raw package loads and scores a batch."""
    from michelangelo.lib.model_manager.packager.custom_triton import (
        CustomTritonPackager,
    )

    src = tmp_path / "model"
    dest = tmp_path / "package"
    src.mkdir()
    dest.mkdir()
    (src / "weights.txt").write_text("w")
    schema = ModelSchema(
        input_schema=[ModelSchemaItem("input", DataType.INT, [1])],
        output_schema=[ModelSchemaItem("response", DataType.INT, [1])],
    )
    package = CustomTritonPackager().create_raw_model_package(
        model_path=str(src),
        model_class=(
            "michelangelo.lib.model_manager.packager.custom_triton.tests."
            "fixtures.predict.Predict"
        ),
        model_schema=schema,
        sample_data=[{"input": np.array([1])}],
        dest_model_path=str(dest),
        include_import_prefixes=["michelangelo"],
    )
    backend = build_backend(resolve_backend(package), BackendContext(package))
    assert backend.output_names == ["response"]
    out = backend.predict({"input": np.array([3])})
    np.testing.assert_array_equal(out["response"], [3])


# --- load_model_schema -----------------------------------------------------


def test_load_model_schema_round_trip(tmp_path):
    """A written schema reads back with names, types and shapes."""
    (tmp_path / "metadata").mkdir()
    (tmp_path / "metadata" / "schema.yaml").write_text(schema_to_yaml(_schema()))
    loaded = load_model_schema(str(tmp_path))
    assert [i.name for i in loaded.input_schema] == ["a", "b"]
    assert loaded.output_schema[0].data_type is DataType.FLOAT
    assert loaded.output_schema[0].shape == [1]


def test_load_model_schema_missing_file_is_none(tmp_path):
    """A package with no schema file yields ``None``."""
    assert load_model_schema(str(tmp_path)) is None


def test_load_model_schema_defaults_missing_sections(tmp_path):
    """A schema file that omits sections still loads."""
    (tmp_path / "metadata").mkdir()
    (tmp_path / "metadata" / "schema.yaml").write_text(
        yaml.safe_dump({"input_schema": [{"name": "a", "data_type": "float"}]})
    )
    loaded = load_model_schema(str(tmp_path))
    assert loaded.output_schema == []
    assert loaded.feature_store_features_schema == []


def test_load_model_schema_rejects_non_mapping(tmp_path):
    """A schema file that is not a mapping is an error."""
    (tmp_path / "metadata").mkdir()
    (tmp_path / "metadata" / "schema.yaml").write_text("- just\n- a list\n")
    with pytest.raises(ValueError, match="mapping"):
        load_model_schema(str(tmp_path))


def test_read_only_inputs_are_scored_without_modifying_or_warning():
    """Read-only (Arrow-backed) arrays are copied for torch, not shared."""
    import warnings

    batch = _batch()
    for arr in batch.values():
        arr.setflags(write=False)
    backend = RawModelBackend(DictModel(), schema=_schema())
    with warnings.catch_warnings():
        warnings.simplefilter("error")
        out = backend.predict(batch)
    np.testing.assert_allclose(out["y"], [12.0, 24.0])
    assert not batch["a"].flags.writeable
