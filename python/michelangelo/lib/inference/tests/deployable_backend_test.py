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


def test_custom_deployable_is_rejected(tmp_path):
    """A custom model is scored from its raw package, never its deployable."""
    schema = ModelSchema(
        input_schema=[ModelSchemaItem("a", DataType.FLOAT, [1])],
        output_schema=[ModelSchemaItem("response", DataType.FLOAT, [1])],
    )
    with pytest.raises(ValueError, match="raw package"):
        _backend(custom_deployable(tmp_path, schema))


def test_torchscript_moves_to_cuda_with_the_fuser_disabled():
    """On CUDA the module is moved and the fusion workaround is applied."""
    from unittest.mock import MagicMock, patch

    import torch

    module = MagicMock(spec=torch.jit.ScriptModule)
    with patch("torch.jit.set_fusion_strategy") as fusion:
        backend = DeployableModelBackend(module, schema=_SCHEMA, device="cuda")

    module.to.assert_called_once_with("cuda")
    fusion.assert_called_once_with([("STATIC", 0), ("DYNAMIC", 0)])
    assert backend._model is module.to.return_value


def test_pytorch_module_gets_the_cuda_fusion_workaround_too():
    """The fuser setting applies to a plain module, not only TorchScript."""
    from unittest.mock import patch

    import torch

    with patch("torch.jit.set_fusion_strategy") as fusion:
        DeployableModelBackend(torch.nn.Identity(), device="cpu")
        fusion.assert_not_called()
        with patch.object(torch.nn.Identity, "to", lambda self, device: self):
            DeployableModelBackend(torch.nn.Identity(), device="cuda:0")
    fusion.assert_called_once_with([("STATIC", 0), ("DYNAMIC", 0)])


class _Meta:
    def __init__(self, name, type_="tensor(float)"):
        self.name = name
        self.type = type_


class _FakeSession:
    """Minimal ONNX Runtime session that records what it was fed."""

    def __init__(self, inputs, outputs):
        self._inputs, self._outputs = inputs, outputs
        self.feeds = None
        self.requested = None

    def get_inputs(self):
        return self._inputs

    def get_outputs(self):
        return self._outputs

    def run(self, names, feeds):
        self.requested, self.feeds = names, feeds
        return [feeds[next(iter(feeds))] for _ in names]


def test_onnx_feeds_schema_inputs_and_names_schema_outputs():
    """With a schema, inputs and outputs follow it, not the graph."""
    session = _FakeSession(
        [_Meta("a"), _Meta("b"), _Meta("unused")], [_Meta("onnx::out")]
    )
    backend = DeployableModelBackend(session, schema=_SCHEMA)

    out = backend.predict({**_INPUTS, "unused": _INPUTS["a"]})

    assert list(session.feeds) == ["a", "b"]
    assert session.requested == ["y"]
    assert list(out) == ["y"]


def test_onnx_without_a_schema_uses_the_graph_names():
    """No schema falls back to the graph's declared inputs and outputs."""
    session = _FakeSession([_Meta("a"), _Meta("b")], [_Meta("out")])

    out = DeployableModelBackend(session).predict({**_INPUTS, "extra": _INPUTS["a"]})

    assert list(session.feeds) == ["a", "b"]
    assert list(out) == ["out"]


def test_names_are_unknown_without_a_schema(tmp_path):
    """Without a schema the predictor falls back to every column."""
    path = torch_deployable(tmp_path, PositionalModel(), _SCHEMA)
    from michelangelo.lib.model_manager.serde.model import load_deployable_model

    backend = DeployableModelBackend(load_deployable_model(path), schema=None)

    assert backend.input_names is None
    assert backend.output_names is None


def test_rejects_a_model_it_cannot_call():
    """Only TorchScript, ONNX Runtime and torch modules are accepted."""
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


def test_onnx_read_only_inputs_are_copied():
    """Arrow-backed arrays are read-only; ONNX Runtime gets a writable copy."""
    arr = np.arange(4, dtype=np.float32)
    arr.flags.writeable = False

    for dtype in (np.float32, np.float64):
        out = _onnx_input(arr, dtype)
        assert out.flags.writeable
        assert out.flags.c_contiguous


def test_onnx_unsigned_integer_types_are_mapped():
    """Unsigned tensors are cast to their declared width."""
    from michelangelo.lib.inference.deployable_backend import _ONNX_DTYPES

    assert _ONNX_DTYPES["tensor(uint16)"] is np.uint16
    assert _ONNX_DTYPES["tensor(uint32)"] is np.uint32
    assert _ONNX_DTYPES["tensor(uint64)"] is np.uint64


def test_onnx_inputs_already_in_the_right_dtype_are_not_cast():
    """Matching or undeclared dtypes are only made contiguous."""
    arr = np.arange(6, dtype=np.float32).reshape(2, 3)[:, ::2]

    for dtype in (np.float32, None):
        out = _onnx_input(arr, dtype)
        assert out.flags.c_contiguous
        assert out.dtype == np.float32
