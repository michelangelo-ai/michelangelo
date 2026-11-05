"""Tests for the deployable-package backend.

Packages are built with the real packagers, so these tests also check that
offline scoring reads exactly what the assembler writes.
"""

from __future__ import annotations

import numpy as np
import pytest

from michelangelo.lib.inference import (
    BackendContext,
    DeployableModelBackend,
    build_backend,
    resolve_backend,
)
from michelangelo.lib.inference.deployable_backend import _onnx_input
from michelangelo.lib.inference.tests.fixtures import (
    DictModel,
    NamedTupleModel,
    PositionalModel,
    RecordModel,
    custom_deployable,
    torch_deployable,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem

_SCHEMA = ModelSchema(
    input_schema=[
        ModelSchemaItem("a", DataType.FLOAT, [1]),
        ModelSchemaItem("b", DataType.FLOAT, [1]),
    ],
    output_schema=[ModelSchemaItem("y", DataType.FLOAT, [1])],
)
_INPUTS = {
    "a": np.array([[1.0], [2.0]], dtype=np.float32),
    "b": np.array([[10.0], [10.0]], dtype=np.float32),
}


def _backend(path: str) -> DeployableModelBackend:
    return build_backend(resolve_backend(path), BackendContext(model_path=path))


@pytest.mark.parametrize(
    ("model", "backend"),
    [
        (PositionalModel(), None),
        (DictModel(), None),
        (PositionalModel(), "onnxruntime"),
        (PositionalModel(), "python"),
    ],
    ids=["torchscript-positional", "torchscript-dict", "onnx", "python-module"],
)
def test_scores_a_packaged_torch_model(tmp_path, model, backend):
    """Every torch packaging backend scores 2a + b under the schema's name."""
    backend_ = _backend(torch_deployable(tmp_path, model, _SCHEMA, backend=backend))

    assert isinstance(backend_, DeployableModelBackend)
    assert backend_.input_names == ["a", "b"]
    assert backend_.output_names == ["y"]
    out = backend_.predict(_INPUTS)
    assert list(out) == ["y"]
    np.testing.assert_allclose(out["y"].ravel(), [12.0, 14.0])


def test_torchscript_namedtuple_outputs_are_matched_by_field_name(tmp_path):
    """A NamedTuple is mapped by field name, not by schema position."""
    schema = ModelSchema(
        input_schema=[ModelSchemaItem("a", DataType.FLOAT, [1])],
        # Deliberately the reverse of the NamedTuple's field order.
        output_schema=[
            ModelSchemaItem("high", DataType.FLOAT, [1]),
            ModelSchemaItem("low", DataType.FLOAT, [1]),
        ],
    )
    out = _backend(torch_deployable(tmp_path, NamedTupleModel(), schema)).predict(
        {"a": np.array([[5.0]], dtype=np.float32)}
    )

    assert out["low"].ravel().tolist() == [4.0]
    assert out["high"].ravel().tolist() == [6.0]


def test_schema_comes_from_config_pbtxt_when_not_given(tmp_path):
    """A torch package has no schema.yaml, so config.pbtxt supplies the names."""
    path = torch_deployable(tmp_path, PositionalModel(), _SCHEMA)
    backend = build_backend(
        resolve_backend(path), BackendContext(model_path=path, schema=None)
    )

    assert backend.input_names == ["a", "b"]
    assert backend.output_names == ["y"]


@pytest.mark.parametrize(
    ("batch", "expected_calls"),
    [(False, [(1,), (1,), (1,)]), (True, [(3, 1)])],
    ids=["per-record", "batch"],
)
def test_custom_model_is_called_the_way_serving_calls_it(
    tmp_path, batch, expected_calls
):
    """Without custom batch processing predict gets one record at a time."""
    schema = ModelSchema(
        input_schema=[ModelSchemaItem("a", DataType.FLOAT, [1])],
        output_schema=[ModelSchemaItem("response", DataType.FLOAT, [1])],
    )
    backend = _backend(custom_deployable(tmp_path, schema, batch=batch))
    RecordModel.calls.clear()

    out = backend.predict({"a": np.array([[1.0], [2.0], [3.0]])})

    assert RecordModel.calls == expected_calls
    assert out["response"].shape == (3, 1)
    assert out["response"].ravel().tolist() == [2.0, 3.0, 4.0]


def test_names_are_unknown_without_a_schema(tmp_path):
    """Without a schema the predictor falls back to every column."""
    path = torch_deployable(tmp_path, PositionalModel(), _SCHEMA)
    from michelangelo.lib.model_manager.serde.model import load_deployable_model

    backend = DeployableModelBackend(load_deployable_model(path), schema=None)

    assert backend.input_names is None
    assert backend.output_names is None


def test_rejects_a_model_it_cannot_call():
    """Only the four loaded-model kinds are accepted."""
    with pytest.raises(TypeError, match="cannot score a str"):
        DeployableModelBackend("not a model")


def test_onnx_integer_inputs_turn_nan_into_the_padding_sentinel():
    """A NaN from a null list element becomes INT32_SENTINEL, not INT64_MIN."""
    out = _onnx_input(np.array([[1.0, np.nan]]), np.int64)

    assert out.dtype == np.int64
    assert out.tolist() == [[1, -(2**31)]]


def test_onnx_string_inputs_are_decoded():
    """Bytes from the predictor's encoding step reach ONNX Runtime as str."""
    out = _onnx_input(np.array([[b"x"], ["y"]], dtype=object), np.object_)

    assert out.tolist() == [["x"], ["y"]]


def test_onnx_inputs_already_in_the_right_dtype_are_not_cast():
    """Matching or undeclared dtypes are only made contiguous."""
    arr = np.arange(6, dtype=np.float32).reshape(2, 3)[:, ::2]

    for dtype in (np.float32, None):
        out = _onnx_input(arr, dtype)
        assert out.flags.c_contiguous
        assert out.dtype == np.float32
