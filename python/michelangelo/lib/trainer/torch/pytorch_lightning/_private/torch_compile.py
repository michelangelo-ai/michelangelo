"""``torch.compile`` support for the PyTorch Lightning trainer."""

from __future__ import annotations

import logging

import torch

from michelangelo.lib._internal.errors import UserInputError

_logger = logging.getLogger(__name__)


def _strip_orig_mod_keys(state_dict: dict) -> dict:
    """Return a new dict with ``_orig_mod.`` removed from all keys.

    ``torch.compile`` wraps the model in an ``OptimizedModule`` whose
    ``state_dict()`` prefixes every key with ``_orig_mod.``.  Stripping
    these prefixes allows the checkpoint to be loaded by an uncompiled
    model (e.g. during assembly/inference).
    """
    return {key.replace("_orig_mod.", ""): value for key, value in state_dict.items()}


def _on_save_checkpoint(checkpoint: dict) -> None:
    """Strip ``_orig_mod.`` key prefixes from checkpoint state_dict.

    Assigned as a bound method on the compiled model so that every
    Lightning checkpoint is compatible with uncompiled model loading.
    """
    sd = checkpoint.get("state_dict")
    if sd:
        checkpoint["state_dict"] = _strip_orig_mod_keys(sd)


def _compile_model_forward(model: torch.nn.Module, config: dict) -> torch.nn.Module:
    """Compile *model*.forward in place with ``torch.compile``.

    Only ``model.forward`` is compiled, not the full model, so Lightning
    lifecycle hooks (``configure_optimizers``, ``training_step``, etc.) are
    never traced.  Any exception raised by ``torch.compile`` propagates to
    the caller unmodified.
    """
    mode = config.get("mode", "default")
    fullgraph = config.get("fullgraph")
    if fullgraph is None:
        fullgraph = True
    dynamic = config.get("dynamic")

    model.forward = torch.compile(
        model.forward, mode=mode, fullgraph=fullgraph, dynamic=dynamic
    )

    _logger.info(
        "torch.compile applied to model.forward: mode=%r, fullgraph=%r, dynamic=%r",
        mode,
        fullgraph,
        dynamic,
    )

    return model


def apply_torch_compile(model: torch.nn.Module, config: dict) -> None:
    """Compile *model*.forward, strip ``_orig_mod.`` keys on checkpoint save.

    Raises ``UserInputError`` on compile failure.
    """
    print_graph_breaks = config.get("print_graph_breaks", False)

    if print_graph_breaks:
        torch._logging.set_logs(graph_breaks=True)

    try:
        _compile_model_forward(model, config)
    except Exception as e:
        raise UserInputError(
            f"torch.compile failed to initialize with config={config!r}: {e!r}."
        ) from e

    original_on_save_checkpoint = getattr(model, "on_save_checkpoint", None)

    def _wrapped_on_save_checkpoint(checkpoint):
        _on_save_checkpoint(checkpoint)
        if original_on_save_checkpoint is not None:
            original_on_save_checkpoint(checkpoint)

    model.on_save_checkpoint = _wrapped_on_save_checkpoint
