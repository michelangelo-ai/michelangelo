"""Tests for the shared calling conventions."""

from __future__ import annotations

import numpy as np
import pytest
import torch

from michelangelo.lib.inference._invoke import (
    _accepts_dict,
    call_torchscript,
    name_outputs,
    read_batch_inference,
    to_torch_inputs,
)
from michelangelo.lib.inference.tests.fixtures import (
    DictModel,
    PositionalModel,
    UnionDictModel,
)

_TENSORS = {"a": torch.tensor([[1.0], [2.0]]), "b": torch.tensor([[10.0], [10.0]])}


@pytest.mark.parametrize(
    ("model", "inputs", "order"),
    [
        (DictModel(), _TENSORS, ["a", "b"]),
        (UnionDictModel(), _TENSORS, ["a", "b"]),
        (PositionalModel(), _TENSORS, ["a", "b"]),
        # Column names that match no argument are bound by position.
        (PositionalModel(), {"x": _TENSORS["a"], "y": _TENSORS["b"]}, ["x", "y"]),
    ],
    ids=["dict", "union-with-dict", "by-name", "by-position"],
)
def test_call_torchscript_binds_inputs_like_serving(model, inputs, order):
    """Each TorchScript signature gets 2a + b."""
    out = call_torchscript(torch.jit.script(model), inputs, order)

    assert out.ravel().tolist() == [12.0, 14.0]


def test_positional_binding_follows_schema_order():
    """Swapping the schema order swaps the arguments, as Triton does."""
    module = torch.jit.script(PositionalModel())
    out = call_torchscript(module, {"x": _TENSORS["a"], "y": _TENSORS["b"]}, ["y", "x"])

    assert out.ravel().tolist() == [21.0, 22.0]


@pytest.mark.parametrize(
    ("arg_type", "expected"),
    [
        (torch._C.TensorType.get(), False),
        (torch._C.OptionalType(torch._C.TensorType.get()), False),
        (
            torch._C.OptionalType(
                torch._C.DictType(torch._C.StringType.get(), torch._C.TensorType.get())
            ),
            True,
        ),
    ],
    ids=["tensor", "optional-tensor", "optional-dict"],
)
def test_accepts_dict(arg_type, expected):
    """Only a Dict, or a Union or Optional holding one, takes the whole batch."""
    assert _accepts_dict(arg_type) is expected


def test_string_columns_become_lists_of_str():
    """A STRING input reaches torch as one str per row, as Triton passes it."""
    out = to_torch_inputs(
        {
            "encoded": np.array([[b"x"], [b"y"]], dtype=object),
            "plain": np.array(["p", "q"]),
            "n": np.array([1.0, 2.0]),
        },
        "cpu",
    )

    assert out["encoded"] == ["x", "y"]
    assert out["plain"] == ["p", "q"]
    assert out["n"].tolist() == [1.0, 2.0]


def test_read_only_numeric_columns_are_copied():
    """Arrow zero-copy arrays are read-only; torch gets a writable copy."""
    arr = np.array([1.0, 2.0])
    arr.flags.writeable = False

    assert to_torch_inputs({"a": arr}, "cpu")["a"].tolist() == [1.0, 2.0]


def test_name_outputs_passes_numpy_values_through():
    """A model that already returns numpy arrays is not converted again."""
    arr = np.array([1.0])
    assert name_outputs({"y": arr}, None)["y"] is arr


def test_name_outputs_reports_a_count_mismatch():
    """More outputs than schema names is an error that names both."""
    with pytest.raises(ValueError, match=r"returned 2 output\(s\) but the schema"):
        name_outputs((torch.ones(1), torch.ones(1)), ["only"])


@pytest.mark.parametrize(
    ("content", "expected"),
    [
        (None, False),
        ("type: custom-python\n", False),
        ("batch_inference: true\n", True),
    ],
    ids=["no-file", "no-key", "set"],
)
def test_read_batch_inference(tmp_path, content, expected):
    """A missing file or key means one record per predict call."""
    if content is not None:
        (tmp_path / "metadata").mkdir()
        (tmp_path / "metadata" / "type.yaml").write_text(content)

    assert read_batch_inference(str(tmp_path)) is expected
