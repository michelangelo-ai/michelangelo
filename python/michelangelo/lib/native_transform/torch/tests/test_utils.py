"""Tests for :mod:`michelangelo.lib.native_transform.torch.utils`.

Covers dtype resolution helpers, layer-name generation, the sentinel lookup, and
the ``format_inputs`` / ``format_outputs`` dict-of-tensors I/O contract.
"""

from __future__ import annotations

import string

import numpy as np
import pytest

# These helpers operate on real torch tensors. Skip cleanly if torch is
# unavailable in a lightweight environment.
torch = pytest.importorskip("torch")

from michelangelo.lib.constants.sentinel import INT32_SENTINEL  # noqa: E402
from michelangelo.lib.native_transform.torch.utils import (  # noqa: E402
    format_inputs,
    format_outputs,
    generate_layer_name,
    id_generator,
    initialize_dtype,
    resolve_torch_dtype,
    sentinel_for_torch_dtype,
    to_snake_case,
)


class TestGenerateLayerName:
    """Name generation combines snake_case with a random suffix."""

    def test_prefix_is_snake_case(self) -> None:
        """The generated name starts with the snake_case base name."""
        output = generate_layer_name("TestLayer")
        assert output[:11] == "test_layer_"


class TestIdGenerator:
    """Random identifier generation."""

    def test_default_size_and_charset(self) -> None:
        """Default output is 10 chars from uppercase ASCII + digits."""
        result = id_generator()
        assert len(result) == 10
        assert all(c in string.ascii_uppercase + string.digits for c in result)

    def test_custom_size(self) -> None:
        """A custom size yields a string of that length."""
        assert len(id_generator(size=5)) == 5

    def test_custom_chars(self) -> None:
        """Only characters from a custom charset appear in the output."""
        custom_chars = "ABC123"
        result = id_generator(size=8, chars=custom_chars)
        assert len(result) == 8
        assert all(c in custom_chars for c in result)

    def test_randomness(self) -> None:
        """Successive calls produce different values."""
        assert id_generator() != id_generator()

    def test_size_zero(self) -> None:
        """A size of zero yields the empty string."""
        assert id_generator(size=0) == ""


class TestToSnakeCase:
    """CamelCase-to-snake_case conversion, including private-name handling."""

    @pytest.mark.parametrize(
        ("name", "expected"),
        [
            ("CamelCase", "camel_case"),
            ("SimpleCase", "simple_case"),
            ("Word", "word"),
            ("word", "word"),
            ("snake_case", "snake_case"),
            ("XMLHttpRequest", "xml_http_request"),
            ("HTTPResponse", "http_response"),
            ("CamelCase2Text", "camel_case2text"),
            ("Test123Case", "test123_case"),
            ("parseHTML5Content", "parse_htm_l5content"),
            ("_PrivateClass", "private__private_class"),
            ("_privateMethod", "private_private_method"),
            ("A", "a"),
            ("a", "a"),
        ],
    )
    def test_conversions(self, name: str, expected: str) -> None:
        """Known name/expected pairs convert correctly."""
        assert to_snake_case(name) == expected

    def test_empty_string_does_not_raise(self) -> None:
        """An empty string is returned unchanged instead of raising."""
        assert to_snake_case("") == ""


class TestResolveTorchDtype:
    """Resolution of dtype specs to concrete torch dtypes."""

    @pytest.mark.parametrize(
        ("spec", "expected"),
        [
            (torch.float32, torch.float32),
            (torch.int64, torch.int64),
            (torch.bool, torch.bool),
            ("float32", torch.float32),
            ("int64", torch.int64),
            ("bool", torch.bool),
            ("torch.float32", torch.float32),
            ("torch.int64", torch.int64),
        ],
    )
    def test_valid_specs(self, spec, expected) -> None:
        """Valid dtype objects and string aliases resolve as expected."""
        assert resolve_torch_dtype(spec) == expected

    def test_invalid_string(self) -> None:
        """An unrecognized string raises ``ValueError``."""
        with pytest.raises(ValueError, match="Unsupported dtype specification"):
            resolve_torch_dtype("invalid_dtype")

    @pytest.mark.parametrize("spec", [123, None])
    def test_invalid_types(self, spec) -> None:
        """Non-dtype, non-string inputs raise ``ValueError``."""
        with pytest.raises(ValueError):
            resolve_torch_dtype(spec)


class TestInitializeDtype:
    """Layer dtype-argument initialization with default fallback."""

    def test_torch_dtype_passthrough(self) -> None:
        """A ``torch.dtype`` is returned unchanged."""
        assert initialize_dtype(torch.float32, torch.int32) == torch.float32

    def test_valid_string(self) -> None:
        """A recognized string alias resolves to its dtype."""
        assert initialize_dtype("float32", torch.int32) == torch.float32
        assert initialize_dtype("int64", torch.int32) == torch.int64

    def test_torch_prefixed_string(self) -> None:
        """A ``torch.``-prefixed string alias resolves like the bare alias."""
        assert initialize_dtype("torch.float32", torch.int32) == torch.float32
        assert initialize_dtype("torch.int64", torch.int32) == torch.int64

    def test_agrees_with_resolve_torch_dtype(self) -> None:
        """Both string families resolve identically to ``resolve_torch_dtype``."""
        for spec in ("float32", "torch.float32", "int64", "torch.int64", "bool"):
            assert initialize_dtype(spec, torch.int32) == resolve_torch_dtype(spec)

    def test_invalid_string_raises(self) -> None:
        """An unrecognized string alias raises ``ValueError``."""
        with pytest.raises(ValueError, match="Unsupported dtype specification"):
            initialize_dtype("flaot32", torch.int32)

    def test_falls_back_to_default(self) -> None:
        """Non-dtype, non-string inputs fall back to the default dtype."""
        assert initialize_dtype(None, torch.int32) == torch.int32
        assert initialize_dtype(123, torch.float64) == torch.float64


class TestFormatInputsOutputs:
    """The stack / unbind I/O contract shared by every layer."""

    def test_format_inputs_selects_and_stacks(self) -> None:
        """``format_inputs`` stacks only the selected columns, in order."""
        inputs = {
            "col1": torch.tensor([1, 2, 3]),
            "col2": torch.tensor([4, 5, 6]),
            "col3": torch.tensor([7, 8, 9]),
        }
        result = format_inputs(["col1", "col3"], inputs)
        expected = torch.stack([inputs["col1"], inputs["col3"]])
        assert torch.equal(result, expected)

    def test_format_outputs_unbinds_to_dict(self) -> None:
        """``format_outputs`` maps each unbound slice to its column name."""
        t1 = torch.tensor([1, 2, 3])
        t2 = torch.tensor([4, 5, 6])
        outputs_tensor = torch.stack([t1, t2])
        result = format_outputs(["out1", "out2"], outputs_tensor)
        assert len(result) == 2
        assert torch.equal(result["out1"], t1)
        assert torch.equal(result["out2"], t2)

    def test_round_trip(self) -> None:
        """``format_outputs`` inverts ``format_inputs`` for matching columns."""
        cols = ["a", "b"]
        inputs = {"a": torch.tensor([1.0, 2.0]), "b": torch.tensor([3.0, 4.0])}
        stacked = format_inputs(cols, inputs)
        restored = format_outputs(cols, stacked)
        assert torch.equal(restored["a"], inputs["a"])
        assert torch.equal(restored["b"], inputs["b"])


class TestSentinelForTorchDtype:
    """Type-native sentinel lookup."""

    @pytest.mark.parametrize("dtype", [torch.float32, torch.float64])
    def test_float_returns_nan(self, dtype) -> None:
        """Floating-point dtypes map to a NaN sentinel."""
        assert np.isnan(sentinel_for_torch_dtype(dtype))

    @pytest.mark.parametrize("dtype", [torch.int32, torch.int64])
    def test_int_returns_int_sentinel(self, dtype) -> None:
        """Integer dtypes map to ``INT32_SENTINEL``."""
        assert sentinel_for_torch_dtype(dtype) == INT32_SENTINEL
        assert sentinel_for_torch_dtype(dtype) == -2147483648

    def test_invalid_dtype_raises(self) -> None:
        """An unsupported dtype raises ``ValueError``."""
        with pytest.raises(ValueError, match="No sentinel defined for dtype"):
            sentinel_for_torch_dtype(torch.bool)
