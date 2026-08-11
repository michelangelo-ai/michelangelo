"""Tests for :mod:`michelangelo.lib.native_transform.torch.base_transform_module`.

Covers ``TorchTransformModule``'s DAG execution (including its
TorchScript-round-trip contract) and ``get_transform_module``'s level-range
materialization from a ``TransformSpec``.
"""

from __future__ import annotations

import pytest

# These tests build real torch layers/modules and a real TransformSpec.
torch = pytest.importorskip("torch")
pytest.importorskip("pydantic")

from michelangelo.lib.native_transform.torch.base_layers import (  # noqa: E402
    Cast,
    Concatenate,
    TorchTransformBaseLayer,
)
from michelangelo.lib.native_transform.torch.base_transform_module import (  # noqa: E402
    TorchTransformModule,
    get_transform_module,
)
from michelangelo.lib.native_transform.torch.transform_spec import (  # noqa: E402
    TransformSpec,
)


class _RecordingLayer(TorchTransformBaseLayer):
    """A layer that records its inputs and returns a fixed output dict."""

    def __init__(
        self, name: str, input_cols: list[str], output_dict: dict[str, torch.Tensor]
    ) -> None:
        """Initialize the recording layer."""
        super().__init__(
            input_cols=input_cols, output_cols=list(output_dict), name=name
        )
        self.output_dict = output_dict
        self.call_args: dict[str, torch.Tensor] | None = None

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Record ``inputs`` and return the fixed ``output_dict``."""
        self.call_args = inputs
        return self.output_dict


class TestTorchTransformModule:
    """``TorchTransformModule``'s construction and forward-execution contract."""

    def test_init(self) -> None:
        """Constructor arguments are stored verbatim as attributes."""
        layers = torch.nn.ModuleList()
        module = TorchTransformModule(
            name="test_module",
            input_cols=["input1", "input2"],
            output_cols=["output1"],
            layers=layers,
        )
        assert module.name == "test_module"
        assert module.input_cols == ["input1", "input2"]
        assert module.output_cols == ["output1"]
        assert module.layers is layers

    def test_forward_chains_layers_in_order(self) -> None:
        """Each layer's output is threaded into the inputs of later layers."""
        layer1 = _RecordingLayer(
            name="layer1",
            input_cols=["input1"],
            output_dict={"intermediate": torch.tensor([1.0, 2.0])},
        )
        layer2 = _RecordingLayer(
            name="layer2",
            input_cols=["intermediate", "input2"],
            output_dict={
                "output1": torch.tensor([3.0, 4.0]),
                "output2": torch.tensor([5.0, 6.0]),
            },
        )
        module = TorchTransformModule(
            name="test_module",
            input_cols=["input1", "input2"],
            output_cols=["output1", "output2"],
            layers=torch.nn.ModuleList([layer1, layer2]),
        )
        inputs = {
            "input1": torch.tensor([10.0, 20.0]),
            "input2": torch.tensor([30.0, 40.0]),
        }

        result = module(inputs)

        assert torch.equal(result["output1"], torch.tensor([3.0, 4.0]))
        assert torch.equal(result["output2"], torch.tensor([5.0, 6.0]))
        assert torch.equal(layer1.call_args["input1"], torch.tensor([10.0, 20.0]))
        assert torch.equal(layer2.call_args["intermediate"], torch.tensor([1.0, 2.0]))
        assert torch.equal(layer2.call_args["input2"], torch.tensor([30.0, 40.0]))

    def test_forward_missing_input_column_raises(self) -> None:
        """A missing declared input column raises with the input name."""
        module = TorchTransformModule(
            name="test_module",
            input_cols=["input1", "input2"],
            output_cols=["output1"],
            layers=torch.nn.ModuleList(),
        )
        with pytest.raises(ValueError, match="Missing input name input2"):
            module({"input1": torch.tensor([1.0, 2.0])})

    def test_forward_missing_layer_input_raises(self) -> None:
        """A layer whose declared input isn't available yet raises."""
        layer = _RecordingLayer(
            name="test_layer", input_cols=["nonexistent_col"], output_dict={}
        )
        module = TorchTransformModule(
            name="test_module",
            input_cols=["input1"],
            output_cols=["input1"],
            layers=torch.nn.ModuleList([layer]),
        )
        with pytest.raises(
            ValueError, match="Missing input name nonexistent_col for layer test_layer"
        ):
            module({"input1": torch.tensor([1.0, 2.0])})

    def test_forward_with_no_layers_passes_through(self) -> None:
        """With no layers, an input column that is also an output passes through."""
        module = TorchTransformModule(
            name="test_module",
            input_cols=["input1"],
            output_cols=["input1"],
            layers=torch.nn.ModuleList(),
        )
        result = module({"input1": torch.tensor([1.0, 2.0])})
        assert torch.equal(result["input1"], torch.tensor([1.0, 2.0]))

    def test_rejects_non_transform_layer_modules(self) -> None:
        """A ``layers`` entry that isn't a ``TorchTransformBaseLayer`` raises."""
        invalid_layer = torch.nn.Linear(10, 10)
        invalid_layer.input_cols = ["input"]
        invalid_layer.name = "linear"
        with pytest.raises(
            AssertionError,
            match="All modules must be instances of TorchTransformBaseLayer",
        ):
            TorchTransformModule(
                name="test_module",
                input_cols=["input"],
                output_cols=["output"],
                layers=torch.nn.ModuleList([invalid_layer]),
            )


class TestTorchScriptRoundTrip:
    """The assembled DAG module must script, save/load, and match eager output."""

    def test_scripted_matches_eager(self, tmp_path) -> None:
        """Scripting (and reloading) reproduces the eager forward output."""
        concat = Concatenate(input_cols=["a", "b"], output_cols=["ab"])
        cast = Cast(input_cols=["ab"], output_cols=["ab_int"], dtype=torch.int64)
        module = TorchTransformModule(
            name="test_module",
            input_cols=["a", "b"],
            output_cols=["ab_int"],
            layers=torch.nn.ModuleList([concat, cast]),
        )
        module.eval()
        inputs = {"a": torch.tensor([[1.0, 2.0]]), "b": torch.tensor([[3.0]])}
        eager = module(inputs)

        scripted = torch.jit.script(module)
        scripted_out = scripted(inputs)
        assert set(scripted_out) == set(eager)
        for key in eager:
            torch.testing.assert_close(scripted_out[key], eager[key])

        model_path = tmp_path / "scripted_module.pt"
        scripted.save(str(model_path))
        loaded = torch.jit.load(str(model_path))
        loaded_out = loaded(inputs)
        for key in eager:
            torch.testing.assert_close(loaded_out[key], eager[key])


class TestGetTransformModule:
    """Level-range materialization of a ``TransformSpec`` into a runnable module."""

    def _spec(self) -> TransformSpec:
        """Build a two-level Cast -> Scale spec for the tests below."""
        return TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Cast",
                        "input_cols": ["a"],
                        "output_cols": ["a_cast"],
                        "dtype": "float32",
                    },
                    {
                        "transform_name": "Scale",
                        "input_cols": ["a_cast"],
                        "output_cols": ["a_scaled"],
                        "factor": 2.0,
                    },
                ]
            }
        )

    def test_materializes_full_range_by_default(self) -> None:
        """With no end_level, every level up to the max is materialized."""
        spec = self._spec()
        module = get_transform_module(spec, start_level=0)
        assert module is not None
        assert len(module.layers) == 2
        assert module.input_cols == ["a"]
        # Default output_cols is every output produced across the included
        # levels, including intermediate ones -- not just the "leftover"
        # (unconsumed) columns, which is how input_cols is computed instead.
        assert module.output_cols == ["a_cast", "a_scaled"]

    def test_end_level_clamped_to_max(self) -> None:
        """An end_level beyond the spec's max level is clamped, not an error."""
        spec = self._spec()
        module = get_transform_module(spec, start_level=0, end_level=99)
        assert len(module.layers) == 2

    def test_single_level_range(self) -> None:
        """A single-level range materializes only that level's layer."""
        spec = self._spec()
        module = get_transform_module(spec, start_level=0, end_level=0)
        assert len(module.layers) == 1
        assert module.output_cols == ["a_cast"]

    def test_no_layers_in_range_returns_none(self) -> None:
        """A level range with no layers returns None instead of an empty module."""
        spec = self._spec()
        assert get_transform_module(spec, start_level=5, end_level=5) is None

    def test_custom_output_cols(self) -> None:
        """An explicit output_cols overrides the default full-output set."""
        spec = self._spec()
        module = get_transform_module(
            spec, start_level=0, end_level=0, output_cols={"a_cast"}
        )
        assert module.output_cols == ["a_cast"]

    def test_forward_end_to_end(self) -> None:
        """The materialized module runs Cast then Scale end to end."""
        spec = self._spec()
        module = get_transform_module(spec, start_level=0)
        module.eval()
        result = module({"a": torch.tensor([1, 2, 3], dtype=torch.int32)})
        torch.testing.assert_close(result["a_scaled"], torch.tensor([2.0, 4.0, 6.0]))
