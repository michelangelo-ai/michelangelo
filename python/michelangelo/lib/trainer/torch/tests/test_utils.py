"""Tests for ``michelangelo.lib.trainer.torch.utils``.

Covers the memory-footprint estimators: ``get_total_training_memory_transformers``
(Transformers/EleutherAI formula), ``estimate_activation_memory_non_transformer``
(pure shape arithmetic), and ``get_total_training_memory_nn_module``
(forward-hook-based estimation on a real ``nn.Module``).

A fake ``PreTrainedModel`` stand-in is used for the Transformers estimator so the
``transformers`` package is not required; the generic estimators run against
real small ``torch`` modules.
"""

from __future__ import annotations

import pytest

# These estimators operate on real torch tensors/modules. Skip cleanly if torch
# is unavailable in a lightweight environment.
torch = pytest.importorskip("torch")
nn = pytest.importorskip("torch.nn")

from michelangelo.lib.trainer.torch.utils import (  # noqa: E402
    estimate_activation_memory_non_transformer,
    get_total_training_memory_nn_module,
    get_total_training_memory_transformers,
)


class _FakeConfig:
    """Minimal ``config`` stand-in for a Hugging Face model."""

    def __init__(
        self, hidden_size, num_hidden_layers, num_attention_heads, torch_dtype
    ):
        """Store the attributes the estimator reads off ``model.config``."""
        self.hidden_size = hidden_size
        self.num_hidden_layers = num_hidden_layers
        self.num_attention_heads = num_attention_heads
        self.torch_dtype = torch_dtype


class _FakeTransformersModel:
    """Minimal ``PreTrainedModel`` stand-in for the Transformers estimator."""

    def __init__(self, num_parameters, config):
        """Capture the parameter count and config used by the estimator."""
        self._num_parameters = num_parameters
        self.config = config

    def num_parameters(self):
        """Return the configured parameter count."""
        return self._num_parameters


# -----------------------------------------------------------------------------
# get_total_training_memory_transformers
# -----------------------------------------------------------------------------


class TestGetTotalTrainingMemoryTransformers:
    """Transformers-model memory estimation."""

    def _model(self, dtype=torch.float16, num_parameters=1_000_000):
        """Build a fake Transformers model with a small config."""
        config = _FakeConfig(
            hidden_size=128,
            num_hidden_layers=4,
            num_attention_heads=8,
            torch_dtype=dtype,
        )
        return _FakeTransformersModel(num_parameters, config)

    def test_returns_positive_float(self):
        """A populated model yields a positive memory estimate."""
        mem = get_total_training_memory_transformers(
            self._model(), batch_size=8, sequence_length=64
        )
        assert isinstance(mem, float)
        assert mem > 0

    def test_more_parameters_costs_more_memory(self):
        """Doubling the parameter count increases the estimate."""
        small = get_total_training_memory_transformers(
            self._model(num_parameters=1_000_000), batch_size=4, sequence_length=32
        )
        large = get_total_training_memory_transformers(
            self._model(num_parameters=2_000_000), batch_size=4, sequence_length=32
        )
        assert large > small

    def test_longer_sequence_costs_more_memory(self):
        """A longer sequence length increases activation memory."""
        short = get_total_training_memory_transformers(
            self._model(), batch_size=4, sequence_length=32
        )
        long = get_total_training_memory_transformers(
            self._model(), batch_size=4, sequence_length=128
        )
        assert long > short

    def test_larger_batch_costs_more_memory(self):
        """A larger batch size increases activation memory."""
        small = get_total_training_memory_transformers(
            self._model(), batch_size=2, sequence_length=32
        )
        large = get_total_training_memory_transformers(
            self._model(), batch_size=16, sequence_length=32
        )
        assert large > small

    @pytest.mark.parametrize("dtype", [torch.float16, torch.float32])
    def test_supported_dtypes(self, dtype):
        """The estimator works for fp16 and fp32 parameter dtypes."""
        mem = get_total_training_memory_transformers(
            self._model(dtype=dtype), batch_size=4, sequence_length=32
        )
        assert mem > 0


# -----------------------------------------------------------------------------
# estimate_activation_memory_non_transformer
# -----------------------------------------------------------------------------


class TestEstimateActivationMemoryNonTransformer:
    """Pure activation-memory arithmetic over captured layer output shapes."""

    def test_empty_dims_returns_zero(self):
        """No captured layers means zero activation memory."""
        assert (
            estimate_activation_memory_non_transformer(
                {}, batch_size=8, bytes_per_value=4
            )
            == 0
        )

    def test_single_layer_matches_formula(self):
        """A single layer's memory equals ``batch * last_dim * bytes / 1MiB``."""
        dims = {"layer0": (8, 256)}
        out = estimate_activation_memory_non_transformer(
            dims, batch_size=8, bytes_per_value=4
        )
        expected = (8 * 256 * 4) / (1024**2)
        assert out == pytest.approx(expected)

    def test_uses_last_dim_of_shape(self):
        """Only the last dimension of each output shape is used."""
        dims = {"layer0": (8, 999, 16)}
        out = estimate_activation_memory_non_transformer(
            dims, batch_size=2, bytes_per_value=2
        )
        expected = (2 * 16 * 2) / (1024**2)
        assert out == pytest.approx(expected)

    def test_multiple_layers_sum(self):
        """Per-layer activation memory is summed across layers."""
        dims = {"a": (4, 10), "b": (4, 20)}
        out = estimate_activation_memory_non_transformer(
            dims, batch_size=4, bytes_per_value=4
        )
        expected = (4 * 10 * 4 + 4 * 20 * 4) / (1024**2)
        assert out == pytest.approx(expected)

    def test_scales_with_batch_size(self):
        """Activation memory scales linearly with batch size."""
        dims = {"a": (1, 32)}
        one = estimate_activation_memory_non_transformer(dims, 1, 4)
        ten = estimate_activation_memory_non_transformer(dims, 10, 4)
        assert ten == pytest.approx(one * 10)


# -----------------------------------------------------------------------------
# get_total_training_memory_nn_module
# -----------------------------------------------------------------------------


class TestGetTotalTrainingMemoryNnModule:
    """Forward-hook-based estimation for a generic ``nn.Module``."""

    def test_returns_positive_float_for_linear_model(self):
        """A simple linear model yields a positive estimate."""
        model = nn.Sequential(nn.Linear(16, 8))
        mem = get_total_training_memory_nn_module(model, batch_size=4, input_size=16)
        assert isinstance(mem, float)
        assert mem > 0

    def test_single_linear_layer(self):
        """A bare ``nn.Linear`` (a top-level child) is estimated without error."""
        model = nn.Linear(16, 4)
        mem = get_total_training_memory_nn_module(model, batch_size=2, input_size=16)
        assert mem > 0

    def test_larger_model_costs_more(self):
        """A model with more parameters yields a larger estimate."""
        small = get_total_training_memory_nn_module(
            nn.Sequential(nn.Linear(16, 8)), batch_size=4, input_size=16
        )
        large = get_total_training_memory_nn_module(
            nn.Sequential(nn.Linear(16, 256)), batch_size=4, input_size=16
        )
        assert large > small

    def test_hooks_are_removed_after_call(self):
        """Forward hooks registered for estimation are cleaned up afterward."""
        layer = nn.Linear(16, 8)
        model = nn.Sequential(layer)
        get_total_training_memory_nn_module(model, batch_size=4, input_size=16)
        assert len(layer._forward_hooks) == 0
