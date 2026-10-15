"""Tests for Predictor."""

from __future__ import annotations

import numpy as np
import pyarrow as pa
import pytest

from michelangelo.lib._internal.utils.numpy_utils.sentinel import (
    sentinel_for_numpy_dtype,
)
from michelangelo.lib.inference import Predictor
from michelangelo.lib.inference.predictor import (
    MODEL_NAME_COLUMN,
    MODEL_VERSION_COLUMN,
)
from michelangelo.lib.inference.tests.fixtures import DictModel, EchoBackend
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem

_FIX = "michelangelo.lib.inference.tests.fixtures"


def _schema(shape=(1,), outputs=("y",)):
    return ModelSchema(
        input_schema=[ModelSchemaItem("a", DataType.FLOAT, [1])],
        output_schema=[
            ModelSchemaItem(n, DataType.FLOAT, list(shape)) for n in outputs
        ],
    )


def _predictor(backend, **kwargs):
    """Build a Predictor around a hand-made backend, skipping model loading."""
    p = Predictor(model=DictModel(), **kwargs)
    p._backend = backend
    return p


def _table(**columns):
    return pa.table(columns)


def test_requires_exactly_one_model_source():
    """Neither or both sources are rejected."""
    with pytest.raises(ValueError, match="exactly one"):
        Predictor()
    with pytest.raises(ValueError, match="exactly one"):
        Predictor(model_path="/x", model=DictModel())


def test_custom_backend_requires_model_path():
    """A backend factory cannot be combined with an in-memory model."""
    with pytest.raises(ValueError, match="model_path"):
        Predictor(model=DictModel(), backend=f"{_FIX}.echo_factory")


def test_scores_with_in_memory_model_and_scalar_squeeze():
    """A scalar-shaped output comes back as one value per row."""
    p = Predictor(model=DictModel(), schema=_schema())
    p._input_columns = ["a", "b"]
    out = p(_table(a=[1.0, 2.0], b=[10.0, 20.0], label=[0, 1]))
    assert out.column_names == ["a", "b", "label", "y"]
    assert out["y"].to_pylist() == [12.0, 24.0]
    assert pa.types.is_floating(out["y"].type)


def test_non_scalar_output_keeps_feature_axis():
    """Outputs whose schema shape is not ``[1]`` stay two-dimensional."""
    backend = EchoBackend({"y": "a"})
    p = _predictor(backend, schema=_schema(shape=(2,)))
    out = p(_table(a=[1.0, 2.0]))
    assert out["y"].to_pylist() == [[1.0], [2.0]]


def test_scalar_columns_reach_backend_with_feature_axis():
    """Scalar input columns are reshaped to ``(n, 1)`` before scoring."""
    backend = EchoBackend({"y": "a"}, input_names=["a"])
    _predictor(backend)(_table(a=[1.0, 2.0, 3.0]))
    assert backend.seen["a"].shape == (3, 1)


def test_input_columns_default_to_backend_names_then_all_columns():
    """Explicit columns win, then the backend's names, then every column."""
    seen = EchoBackend({"y": "a"}, input_names=["a"])
    _predictor(seen)(_table(a=[1.0], b=[2.0]))
    assert list(seen.seen) == ["a"]

    unnamed = EchoBackend({"y": "a"}, input_names=None)
    _predictor(unnamed)(_table(a=[1.0], b=[2.0]))
    assert list(unnamed.seen) == ["a", "b"]

    explicit = EchoBackend({"y": "b"}, input_names=["a"])
    _predictor(explicit, input_columns=["b"])(_table(a=[1.0], b=[2.0]))
    assert list(explicit.seen) == ["b"]


def test_ragged_ints_are_padded_with_the_dtype_sentinel_not_zero():
    """Unlisted columns pad with the sentinel for their dtype."""
    backend = EchoBackend({"y": "tokens"})
    _predictor(backend)(_table(tokens=[[1, 2, 3], [4]]))
    padded = backend.seen["tokens"]
    assert padded.shape == (2, 3)
    assert padded[1, 1] == sentinel_for_numpy_dtype(np.dtype(np.int64))
    assert padded[1, 1] != 0


def test_explicit_padding_value_wins():
    """A per-column pad value overrides the sentinel."""
    backend = EchoBackend({"y": "tokens"})
    _predictor(backend, column_padding_values={"tokens": -7})(
        _table(tokens=[[1, 2, 3], [4]])
    )
    assert backend.seen["tokens"][1].tolist() == [4, -7, -7]


def test_string_columns_round_trip_with_none_and_empty():
    """Strings reach the backend as bytes and come back as text, nulls intact."""
    backend = EchoBackend({"text_out": "text"})
    p = _predictor(backend, schema=_schema(outputs=("text_out",)))
    out = p(_table(text=["abc", "", None, "é"]))
    assert backend.seen["text"].dtype.kind in ("S", "O")
    assert out["text_out"].to_pylist() == ["abc", "", None, "é"]


def test_all_none_string_column_survives():
    """A column that is entirely null does not crash encoding."""
    backend = EchoBackend({"out": "text"})
    p = _predictor(backend, schema=_schema(outputs=("out",)))
    out = p(_table(text=pa.array([None, None], type=pa.string())))
    assert out["out"].to_pylist() == [None, None]


def test_columns_to_keep_drops_labels_but_keeps_predictions():
    """Listing only ``a`` drops ``label`` but never the prediction."""
    backend = EchoBackend({"y": "a"})
    out = _predictor(backend, columns_to_keep=["a"])(
        _table(a=[1.0], label=[5], other=[6])
    )
    assert out.column_names == ["a", "y"]


def test_columns_to_keep_ignores_unknown_names_and_keeps_listed_labels():
    """Unknown names are ignored; listed labels survive."""
    backend = EchoBackend({"y": "a"})
    out = _predictor(backend, columns_to_keep=["label", "nope"])(
        _table(a=[1.0], label=[5])
    )
    assert out.column_names == ["label", "y"]


def test_empty_columns_to_keep_keeps_only_predictions():
    """An empty list is not the same as ``None``."""
    out = _predictor(EchoBackend({"y": "a"}), columns_to_keep=[])(
        _table(a=[1.0], label=[5])
    )
    assert out.column_names == ["y"]


def test_none_columns_to_keep_keeps_everything():
    """``None`` keeps every input column."""
    out = _predictor(EchoBackend({"y": "a"}))(_table(a=[1.0], label=[5]))
    assert out.column_names == ["a", "label", "y"]


def test_model_columns_are_appended_last():
    """Name and version columns follow the predictions."""
    p = _predictor(EchoBackend({"y": "a"}), model_name="housing", model_version="3")
    out = p(_table(a=[1.0, 2.0]))
    assert out.column_names == ["a", "y", MODEL_NAME_COLUMN, MODEL_VERSION_COLUMN]
    assert out[MODEL_NAME_COLUMN].to_pylist() == ["housing", "housing"]
    assert out[MODEL_VERSION_COLUMN].to_pylist() == ["3", "3"]


def test_model_columns_survive_columns_to_keep():
    """Model columns are kept even when ``columns_to_keep`` is set."""
    p = _predictor(EchoBackend({"y": "a"}), model_name="m", columns_to_keep=["a"])
    assert p(_table(a=[1.0], b=[2.0])).column_names == ["a", "y", MODEL_NAME_COLUMN]


def test_prediction_named_like_input_column_is_rejected():
    """A prediction never silently overwrites an input column."""
    with pytest.raises(ValueError, match="already exist"):
        _predictor(EchoBackend({"label": "a"}))(_table(a=[1.0], label=[1]))


def test_prediction_named_like_model_column_is_rejected():
    """A prediction named ``model_name`` would clobber the metadata column."""
    p = _predictor(EchoBackend({MODEL_NAME_COLUMN: "a"}), model_name="m")
    with pytest.raises(ValueError, match="already exist"):
        p(_table(a=[1.0]))


def test_empty_batch_is_returned_unchanged():
    """No rows means nothing to score."""
    batch = pa.table({"a": pa.array([], type=pa.float64())})
    assert _predictor(EchoBackend({"y": "a"}))(batch) is batch


@pytest.mark.parametrize("num_cpus", [None, 1, 3])
def test_thread_pool_matches_serial_conversion(num_cpus):
    """The thread pool is only created above one CPU and changes nothing."""
    backend = EchoBackend({"y": "a", "z": "b"})
    p = Predictor(model=DictModel(), num_cpus=num_cpus)
    p._backend = backend
    assert (p._executor is not None) == bool(num_cpus and num_cpus > 1)
    out = p(_table(a=[1.0, 2.0], b=[3.0, 4.0]))
    assert out["z"].to_pylist() == [[3.0], [4.0]]


def test_backend_object_is_the_dispatch_point():
    """A backend's own ``predict`` is what runs, so subclass overrides take effect."""

    class Overriding(EchoBackend):
        def predict(self, inputs):
            return {"y": inputs["a"] * 0 + 42}

    out = _predictor(Overriding({"y": "a"}))(_table(a=[1.0, 2.0]))
    assert out["y"].to_pylist() == [[42.0], [42.0]]


def test_loads_model_package_and_stored_schema(tmp_path):
    """A package path loads the model and picks up its schema for squeezing."""
    import torch

    from michelangelo.lib.model_manager._private.schema.common.serde import (
        schema_to_yaml,
    )

    for d in ("model", "defs", "metadata"):
        (tmp_path / d).mkdir()
    torch.save(DictModel().state_dict(), tmp_path / "model" / "model.pt")
    (tmp_path / "defs" / "model_class.txt").write_text(f"{_FIX}.DictModel")
    (tmp_path / "metadata" / "type.yaml").write_text("type: torch\n")
    schema = ModelSchema(
        input_schema=[
            ModelSchemaItem("a", DataType.FLOAT, [1]),
            ModelSchemaItem("b", DataType.FLOAT, [1]),
        ],
        output_schema=[ModelSchemaItem("y", DataType.FLOAT, [1])],
    )
    (tmp_path / "metadata" / "schema.yaml").write_text(schema_to_yaml(schema))

    out = Predictor(model_path=str(tmp_path), device="cpu")(
        _table(a=[1.0], b=[1.0], label=[0])
    )
    assert out["y"].to_pylist() == [3.0]


def test_custom_backend_from_dotted_path(tmp_path):
    """A dotted-path factory replaces the built-in backend."""
    p = Predictor(model_path=str(tmp_path), backend=f"{_FIX}.echo_factory")
    assert p(_table(a=[1.0, 2.0]))["y"].to_pylist() == [[1.0], [2.0]]
