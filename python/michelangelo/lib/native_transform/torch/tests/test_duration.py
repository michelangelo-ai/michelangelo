"""Tests for :mod:`michelangelo.lib.native_transform.torch.duration`.

Covers ``TimeDuration`` forward semantics (unit scaling, reshaping, clipping
applied after log scaling, mixed input dtypes, init validation) and its
``torch.jit.script``/``torch.jit.trace`` round trips.
"""

from __future__ import annotations

import pytest

# These layers operate on real torch tensors/modules. Skip cleanly if torch is
# unavailable in a lightweight environment.
torch = pytest.importorskip("torch")

from michelangelo.lib.native_transform.torch.duration import TimeDuration  # noqa: E402

_ONE_DAY_MS = 24 * 60 * 60 * 1000


class TestTimeDuration:
    """Forward semantics of the TimeDuration layer."""

    def test_forward(self) -> None:
        """Difference is floor-divided by unit."""
        layer = TimeDuration(
            unit=_ONE_DAY_MS, input_cols=["target", "source"], output_cols=["duration"]
        )
        inputs = {
            "target": torch.tensor(
                [[1727171357801], [1727213336630]], dtype=torch.float32
            ),
            "source": torch.tensor(
                [
                    [1727171357801, 1727085467217, 1726997165128],
                    [1727213336630, 1727196549297, 1727149789328],
                ],
                dtype=torch.float32,
            ),
        }
        outputs = layer(inputs)
        expected = torch.tensor([[0, 0, 2], [0, 0, 0]], dtype=torch.float32)
        torch.testing.assert_close(outputs["duration"], expected)

    def test_forward_with_reshape(self) -> None:
        """target_shape reshapes the target tensor before the difference."""
        layer = TimeDuration(
            unit=_ONE_DAY_MS,
            target_shape=(-1, 1),
            input_cols=["target", "source"],
            output_cols=["duration"],
        )
        inputs = {
            "target": torch.tensor([1727171357801, 1727213336630], dtype=torch.float32),
            "source": torch.tensor(
                [
                    [1727171357801, 1727085467217, 1726997165128],
                    [1727213336630, 1727196549297, 1727149789328],
                ],
                dtype=torch.float32,
            ),
        }
        outputs = layer(inputs)
        expected = torch.tensor([[0, 0, 2], [0, 0, 0]], dtype=torch.float32)
        torch.testing.assert_close(outputs["duration"], expected)

    def test_forward_with_source_shape(self) -> None:
        """source_shape reshapes the source tensor before the difference."""
        layer = TimeDuration(
            unit=_ONE_DAY_MS,
            source_shape=(-1, 2),
            input_cols=["target", "source"],
            output_cols=["duration"],
        )
        inputs = {
            "target": torch.tensor(
                [[1727171357801], [1727213336630]], dtype=torch.float32
            ),
            "source": torch.tensor(
                [1727171357801, 1726997165128, 1727213336630, 1727149789328],
                dtype=torch.float32,
            ),
        }
        outputs = layer(inputs)
        expected = torch.tensor([[0, 2], [0, 0]], dtype=torch.float32)
        torch.testing.assert_close(outputs["duration"], expected)

    def test_forward_with_clipping(self) -> None:
        """min_value/max_value clamp the floored duration."""
        layer = TimeDuration(
            unit=_ONE_DAY_MS,
            min_value=1,
            max_value=10,
            input_cols=["target", "source"],
            output_cols=["duration"],
        )
        inputs = {
            "target": torch.tensor(
                [[1727171357801], [1727213336630]], dtype=torch.float32
            ),
            "source": torch.tensor(
                [
                    [1727171357801, 1726997165128, 1726097165128],
                    [1727213336630, 1727196549297, 1727149789328],
                ],
                dtype=torch.float32,
            ),
        }
        outputs = layer(inputs)
        expected = torch.tensor([[1, 2, 10], [1, 1, 1]], dtype=torch.float32)
        torch.testing.assert_close(outputs["duration"], expected)

    def test_forward_with_log_scale(self) -> None:
        """log_scale applies log1p(abs(x)) to the floored duration."""
        layer = TimeDuration(
            unit=_ONE_DAY_MS,
            log_scale=True,
            input_cols=["target", "source"],
            output_cols=["duration"],
        )
        inputs = {
            "target": torch.tensor(
                [[1727171357801], [1727213336630]], dtype=torch.float32
            ),
            "source": torch.tensor(
                [[1727171357801, 1726997165128], [1727213336630, 1727149789328]],
                dtype=torch.float32,
            ),
        }
        outputs = layer(inputs)
        expected = torch.tensor(
            [[0.0, torch.log1p(torch.tensor(2.0))], [0.0, 0.0]], dtype=torch.float32
        )
        torch.testing.assert_close(outputs["duration"], expected)

    def test_clipping_after_log_scale_order(self) -> None:
        """Clipping is applied after log scaling, not before."""
        layer = TimeDuration(
            unit=_ONE_DAY_MS,
            min_value=1,
            max_value=2,
            log_scale=True,
            input_cols=["target", "source"],
            output_cols=["duration"],
        )
        inputs = {
            "target": torch.tensor(
                [[1727171357801], [1727171357801]], dtype=torch.float32
            ),
            "source": torch.tensor(
                [[1726997165128, 1726097165128], [1726997165128, 1726097165128]],
                dtype=torch.float32,
            ),
        }
        outputs = layer(inputs)
        expected_log2 = torch.log1p(torch.tensor(2.0))
        expected = torch.tensor(
            [[expected_log2, 2.0], [expected_log2, 2.0]], dtype=torch.float32
        )
        torch.testing.assert_close(outputs["duration"], expected)

    def test_different_dtypes(self) -> None:
        """Integer input dtypes are computed in float before flooring."""
        layer = TimeDuration(
            unit=1000, input_cols=["target", "source"], output_cols=["duration"]
        )
        inputs = {
            "target": torch.tensor([[5000]], dtype=torch.int64),
            "source": torch.tensor([[3000]], dtype=torch.int32),
        }
        outputs = layer(inputs)
        expected = torch.tensor([[2.0]], dtype=torch.float32)
        torch.testing.assert_close(outputs["duration"], expected)

    def test_init_validation(self) -> None:
        """min_value and max_value must both be set or both be None."""
        TimeDuration(
            input_cols=["target", "source"],
            output_cols=["duration"],
            min_value=None,
            max_value=None,
        )
        TimeDuration(
            input_cols=["target", "source"],
            output_cols=["duration"],
            min_value=0,
            max_value=10,
        )
        with pytest.raises(AssertionError):
            TimeDuration(
                input_cols=["target", "source"],
                output_cols=["duration"],
                min_value=0,
                max_value=None,
            )
        with pytest.raises(AssertionError):
            TimeDuration(
                input_cols=["target", "source"],
                output_cols=["duration"],
                min_value=None,
                max_value=10,
            )


class TestTimeDurationTorchScriptRoundTrip:
    """TimeDuration must script/trace, save/load, and match eager output."""

    def test_scripted_matches_eager(self, tmp_path) -> None:
        """Scripting (and reloading) reproduces the eager forward output."""
        layer = TimeDuration(
            unit=_ONE_DAY_MS, input_cols=["target", "source"], output_cols=["duration"]
        )
        layer.eval()
        inputs = {
            "target": torch.tensor(
                [[1727171357801], [1727213336630]], dtype=torch.float32
            ),
            "source": torch.tensor(
                [[1727171357801, 1726997165128], [1727213336630, 1727149789328]],
                dtype=torch.float32,
            ),
        }
        eager = layer(inputs)

        scripted = torch.jit.script(layer)
        scripted_out = scripted(inputs)
        torch.testing.assert_close(scripted_out["duration"], eager["duration"])

        model_path = tmp_path / "scripted_time_duration.pt"
        scripted.save(str(model_path))
        loaded = torch.jit.load(str(model_path))
        loaded_out = loaded(inputs)
        torch.testing.assert_close(loaded_out["duration"], eager["duration"])

    def test_traced_matches_eager_with_log_scale_and_clipping(self) -> None:
        """Tracing reproduces the eager output when log_scale and clipping combine."""
        layer = TimeDuration(
            unit=_ONE_DAY_MS,
            min_value=1,
            max_value=10,
            log_scale=True,
            input_cols=["target", "source"],
            output_cols=["duration"],
        )
        inputs = {
            "target": torch.tensor(
                [[1727171357801], [1727213336630]], dtype=torch.float32
            ),
            "source": torch.tensor(
                [[1727171357801, 1726097165128], [1727213336630, 1727149789328]],
                dtype=torch.float32,
            ),
        }
        eager = layer(inputs)
        traced = torch.jit.trace(layer, (inputs,), strict=False)
        traced_out = traced(inputs)
        torch.testing.assert_close(traced_out["duration"], eager["duration"])

    def test_traced_matches_eager_with_reshape(self) -> None:
        """Tracing matches eager when target_shape/source_shape reshape inputs."""
        layer = TimeDuration(
            unit=_ONE_DAY_MS,
            target_shape=(-1, 1),
            source_shape=(-1, 2),
            input_cols=["target", "source"],
            output_cols=["duration"],
        )
        inputs = {
            "target": torch.tensor([1727171357801, 1727213336630], dtype=torch.float32),
            "source": torch.tensor(
                [1727171357801, 1726997165128, 1727213336630, 1727149789328],
                dtype=torch.float32,
            ),
        }
        eager = layer(inputs)
        traced = torch.jit.trace(layer, (inputs,), strict=False)
        traced_out = traced(inputs)
        torch.testing.assert_close(traced_out["duration"], eager["duration"])
