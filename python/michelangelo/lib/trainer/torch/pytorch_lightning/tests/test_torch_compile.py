"""Tests for ``torch.compile`` support utilities in ``_private/torch_compile.py``."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest
import torch

from michelangelo.lib._internal.errors import UserInputError
from michelangelo.lib.trainer.torch.pytorch_lightning._private.torch_compile import (
    _compile_model_forward,
    _on_save_checkpoint,
    _strip_orig_mod_keys,
    apply_torch_compile,
)

_MODULE = (
    "michelangelo.lib.trainer.torch.pytorch_lightning._private.torch_compile"
)


# -----------------------------------------------------------------------------
# _compile_model_forward
# -----------------------------------------------------------------------------


class TestCompileModelForward:
    """Argument wiring and failure propagation for ``_compile_model_forward``."""

    @patch(f"{_MODULE}.torch")
    def test_compile_called_with_correct_args(self, mock_torch):
        """``torch.compile`` receives mode, fullgraph, and dynamic from config."""
        mock_torch.compile.return_value = MagicMock(name="compiled_forward")
        model = MagicMock()
        original_forward = model.forward
        config = {"mode": "reduce-overhead", "fullgraph": False}

        result = _compile_model_forward(model, config)

        mock_torch.compile.assert_called_once_with(
            original_forward, mode="reduce-overhead", fullgraph=False, dynamic=None
        )
        assert result is model

    @patch(f"{_MODULE}.torch")
    def test_compile_exception_propagates(self, mock_torch):
        """``torch.compile`` failures propagate unmodified."""
        mock_torch.compile.side_effect = RuntimeError("compile failed")
        model = MagicMock()

        with pytest.raises(RuntimeError):
            _compile_model_forward(model, {"mode": "default", "fullgraph": True})

    @patch(f"{_MODULE}.torch")
    def test_fullgraph_defaults_to_true(self, mock_torch):
        """``fullgraph`` defaults to ``True`` when not in config."""
        mock_torch.compile.return_value = MagicMock()
        model = MagicMock()
        original_forward = model.forward

        _compile_model_forward(model, {"mode": "default"})

        mock_torch.compile.assert_called_once_with(
            original_forward, mode="default", fullgraph=True, dynamic=None
        )

    @patch(f"{_MODULE}.torch")
    def test_dynamic_passed_through(self, mock_torch):
        """``dynamic`` is forwarded to ``torch.compile``."""
        mock_torch.compile.return_value = MagicMock()
        model = MagicMock()
        original_forward = model.forward

        _compile_model_forward(
            model, {"mode": "default", "fullgraph": True, "dynamic": True}
        )

        mock_torch.compile.assert_called_once_with(
            original_forward, mode="default", fullgraph=True, dynamic=True
        )


# -----------------------------------------------------------------------------
# apply_torch_compile
# -----------------------------------------------------------------------------


class TestApplyTorchCompile:
    """Graph-break logging, failure wrapping, and checkpoint hook installation."""

    @patch(f"{_MODULE}.torch")
    def test_installs_hook_when_compile_succeeds(self, mock_torch):
        """A successful compile installs the checkpoint-stripping hook."""
        mock_torch.compile.return_value = MagicMock()
        model = MagicMock()
        original_hook = MagicMock()
        model.on_save_checkpoint = original_hook

        apply_torch_compile(
            model, {"mode": "default", "fullgraph": True, "print_graph_breaks": False}
        )

        assert model.on_save_checkpoint is not original_hook

    @patch(f"{_MODULE}.torch")
    def test_compile_failure_raises_user_input_error(self, mock_torch):
        """Compile failure raises ``UserInputError`` with the original cause."""
        mock_torch.compile.side_effect = RuntimeError("compile failed")
        model = MagicMock()
        original_hook = MagicMock()
        model.on_save_checkpoint = original_hook

        with pytest.raises(UserInputError) as exc_info:
            apply_torch_compile(
                model,
                {"mode": "default", "fullgraph": True, "print_graph_breaks": False},
            )
        assert isinstance(exc_info.value.__cause__, RuntimeError)
        assert model.on_save_checkpoint is original_hook

    @patch(f"{_MODULE}.torch")
    def test_print_graph_breaks_enables_logging(self, mock_torch):
        """``print_graph_breaks=True`` calls ``torch._logging.set_logs``."""
        mock_torch.compile.return_value = MagicMock()
        model = MagicMock()

        apply_torch_compile(
            model, {"mode": "default", "fullgraph": True, "print_graph_breaks": True}
        )

        mock_torch._logging.set_logs.assert_called_once_with(graph_breaks=True)


# -----------------------------------------------------------------------------
# _strip_orig_mod_keys
# -----------------------------------------------------------------------------


class TestStripOrigModKeys:
    """Key-stripping for ``torch.compile`` checkpoint compatibility."""

    def test_strips_leading_orig_mod_prefix(self):
        """Leading ``_orig_mod.`` is removed."""
        t = torch.tensor([1.0])
        result = _strip_orig_mod_keys({"_orig_mod.layer.weight": t})
        assert "layer.weight" in result
        assert "_orig_mod.layer.weight" not in result

    def test_strips_nested_orig_mod(self):
        """Nested ``_orig_mod.`` is removed."""
        t = torch.tensor([2.0])
        result = _strip_orig_mod_keys({"module._orig_mod.layer.weight": t})
        assert "module.layer.weight" in result

    def test_noop_when_no_orig_mod_keys(self):
        """Keys without ``_orig_mod.`` are unchanged."""
        original = {"layer.weight": torch.tensor([1.0]), "layer.bias": torch.tensor([0.0])}
        result = _strip_orig_mod_keys(original)
        assert set(result.keys()) == set(original.keys())

    def test_noop_on_empty_dict(self):
        """Empty dict produces empty dict."""
        assert _strip_orig_mod_keys({}) == {}

    def test_tensor_values_preserved(self):
        """Tensor identity is preserved (no copy)."""
        t = torch.tensor([3.14])
        result = _strip_orig_mod_keys({"_orig_mod.w": t})
        assert result["w"] is t


# -----------------------------------------------------------------------------
# _on_save_checkpoint
# -----------------------------------------------------------------------------


class TestOnSaveCheckpoint:
    """Checkpoint hook that strips ``_orig_mod.`` prefixes."""

    def test_strips_keys_from_state_dict(self):
        """``_orig_mod.`` prefixes are removed from state_dict keys."""
        checkpoint = {
            "state_dict": {
                "_orig_mod.layer.weight": torch.tensor([1.0]),
                "_orig_mod.layer.bias": torch.tensor([0.0]),
            }
        }
        _on_save_checkpoint(checkpoint)
        assert "layer.weight" in checkpoint["state_dict"]
        assert "_orig_mod.layer.weight" not in checkpoint["state_dict"]

    def test_noop_no_orig_mod_keys(self):
        """No-op when keys have no ``_orig_mod.`` prefix."""
        checkpoint = {"state_dict": {"layer.weight": torch.tensor([1.0])}}
        _on_save_checkpoint(checkpoint)
        assert "layer.weight" in checkpoint["state_dict"]

    def test_noop_empty_state_dict(self):
        """No-op on empty state_dict."""
        checkpoint = {"state_dict": {}}
        _on_save_checkpoint(checkpoint)
        assert checkpoint["state_dict"] == {}

    def test_missing_state_dict_key(self):
        """No crash when checkpoint has no ``state_dict`` key."""
        checkpoint = {"epoch": 5}
        _on_save_checkpoint(checkpoint)
        assert "state_dict" not in checkpoint
