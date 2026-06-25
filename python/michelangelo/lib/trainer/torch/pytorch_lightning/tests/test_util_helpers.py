"""Tests for untested helpers in ``_private/util.py``.

Covers the helpers not exercised by ``test_resolve_helpers.py``:
``_apply_layer_freeze`` (substring / regex layer freezing) and
``_load_weights_from_path`` (fsspec download + ``torch.load`` + ``load_state_dict``).

``UserInputError`` tests live in ``michelangelo/lib/_internal/tests/test_errors.py``.
``get_module_attr`` tests live in
``michelangelo/_internal/utils/reflection_utils/tests/module_attr_test.py``.

``ray.train.get_context`` and the fsspec / ``torch.load`` I/O are mocked so the
helpers run without a Ray cluster or remote storage.
"""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

# The util module imports ray / torch / pytorch_lightning at import time. Skip
# cleanly when those optional heavy dependencies are unavailable.
pytest.importorskip("ray")
torch = pytest.importorskip("torch")
nn = pytest.importorskip("torch.nn")
pytest.importorskip("pytorch_lightning")

from michelangelo.lib.trainer.torch.pytorch_lightning._private.util import (  # noqa: E402
    _apply_layer_freeze,
    _load_weights_from_path,
)

_UTIL_MODULE = "michelangelo.lib.trainer.torch.pytorch_lightning._private.util"


class _TwoLayerNet(nn.Module):
    """Tiny model with named ``encoder`` / ``decoder`` linear layers."""

    def __init__(self):
        """Build two named linear layers for freeze/state-dict testing."""
        super().__init__()
        self.encoder = nn.Linear(4, 4)
        self.decoder = nn.Linear(4, 2)


# -----------------------------------------------------------------------------
# _load_weights_from_path
# -----------------------------------------------------------------------------


class TestLoadWeightsFromPath:
    """Download + load of a remote/local state-dict file."""

    def test_downloads_and_loads_state_dict(self):
        """Fsspec downloads the file, ``torch.load`` reads it, model loads it."""
        model = MagicMock()
        fake_fs = MagicMock()
        state_dict = {"w": object()}

        with (
            patch(
                f"{_UTIL_MODULE}.url_to_fs", return_value=(fake_fs, "remote/path.pt")
            ) as u2fs,
            patch(f"{_UTIL_MODULE}.torch.load", return_value=state_dict) as tload,
        ):
            _load_weights_from_path(model, "s3://bucket/path.pt")

        u2fs.assert_called_once_with("s3://bucket/path.pt")
        fake_fs.get.assert_called_once()
        # torch.load is called on CPU with weights_only=True.
        _, load_kwargs = tload.call_args
        assert load_kwargs["map_location"] == "cpu"
        assert load_kwargs["weights_only"] is True
        model.load_state_dict.assert_called_once_with(state_dict, strict=True)

    def test_loads_into_real_module(self):
        """A real module's parameters are replaced by the loaded state dict."""
        src = _TwoLayerNet()
        dst = _TwoLayerNet()
        # Ensure the two models differ before loading.
        with torch.no_grad():
            for p in dst.parameters():
                p.add_(1.0)

        fake_fs = MagicMock()
        with (
            patch(f"{_UTIL_MODULE}.url_to_fs", return_value=(fake_fs, "p.pt")),
            patch(f"{_UTIL_MODULE}.torch.load", return_value=src.state_dict()),
        ):
            _load_weights_from_path(dst, "file:///tmp/p.pt")

        for (n1, p1), (n2, p2) in zip(src.named_parameters(), dst.named_parameters()):
            assert n1 == n2
            assert torch.equal(p1, p2)


# -----------------------------------------------------------------------------
# _apply_layer_freeze
# -----------------------------------------------------------------------------


@pytest.fixture
def patched_ray_rank():
    """Patch ``ray.train.get_context().get_world_rank()`` used for logging."""
    with patch(f"{_UTIL_MODULE}.ray") as ray_mod:
        ray_mod.train.get_context.return_value.get_world_rank.return_value = 0
        yield ray_mod


class TestApplyLayerFreeze:
    """Re-application of layer freezing from a transfer-learning spec."""

    def _frozen_names(self, model):
        """Return the set of parameter names with ``requires_grad`` disabled."""
        return {n for n, p in model.named_parameters() if not p.requires_grad}

    def test_substring_match_freezes_encoder(self, patched_ray_rank):
        """A substring entry freezes all matching named parameters."""
        model = _TwoLayerNet()
        _apply_layer_freeze(model, {"layer_names_to_freeze": ["encoder"]})
        frozen = self._frozen_names(model)
        assert "encoder.weight" in frozen
        assert "encoder.bias" in frozen
        assert "decoder.weight" not in frozen

    def test_regex_match_freezes(self, patched_ray_rank):
        """A regex entry freezes parameters matched by ``re.search``."""
        model = _TwoLayerNet()
        _apply_layer_freeze(model, {"layer_names_to_freeze_regex": [r"^decoder\."]})
        frozen = self._frozen_names(model)
        assert "decoder.weight" in frozen
        assert "decoder.bias" in frozen
        assert "encoder.weight" not in frozen

    def test_empty_spec_freezes_nothing(self, patched_ray_rank):
        """An empty spec leaves every parameter trainable."""
        model = _TwoLayerNet()
        _apply_layer_freeze(model, {})
        assert self._frozen_names(model) == set()

    def test_none_values_treated_as_empty(self, patched_ray_rank):
        """Explicit ``None`` freeze lists are treated as empty (no crash)."""
        model = _TwoLayerNet()
        _apply_layer_freeze(
            model,
            {"layer_names_to_freeze": None, "layer_names_to_freeze_regex": None},
        )
        assert self._frozen_names(model) == set()

    def test_substring_and_regex_combined(self, patched_ray_rank):
        """Substring and regex matches are unioned."""
        model = _TwoLayerNet()
        _apply_layer_freeze(
            model,
            {
                "layer_names_to_freeze": ["encoder.weight"],
                "layer_names_to_freeze_regex": [r"decoder\.bias"],
            },
        )
        frozen = self._frozen_names(model)
        assert "encoder.weight" in frozen
        assert "decoder.bias" in frozen
        assert "encoder.bias" not in frozen
        assert "decoder.weight" not in frozen

    def test_non_matching_pattern_freezes_nothing(self, patched_ray_rank):
        """A pattern that matches no layer leaves all parameters trainable."""
        model = _TwoLayerNet()
        _apply_layer_freeze(model, {"layer_names_to_freeze": ["nonexistent"]})
        assert self._frozen_names(model) == set()
