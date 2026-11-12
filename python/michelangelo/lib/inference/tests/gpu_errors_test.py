"""Tests for GPU poisoned-context detection."""

from __future__ import annotations

import pytest

from michelangelo.lib.inference._gpu_errors import should_restart_worker


class AcceleratorError(RuntimeError):
    """Stands in for ``torch.AcceleratorError``, which is matched by name."""


@pytest.mark.parametrize(
    "message",
    [
        "CUDA error: an illegal memory access was encountered",
        "CUDA error: unspecified launch failure",
        "CUDA error: device-side assert triggered",
        "GPU hardware error over NVLink",
        "NVIDIA uncorrectable ECC error",
    ],
)
def test_poisoned_context_messages_restart_the_worker(message):
    """A GPU-stack message with a poisoned-context signature is fatal."""
    assert should_restart_worker(RuntimeError(message))


def test_accelerator_error_type_needs_no_gpu_word_in_the_message():
    """The exception type alone proves the GPU stack raised it."""
    assert should_restart_worker(AcceleratorError("illegal memory access"))


@pytest.mark.parametrize(
    "message",
    [
        "CUDA out of memory. Tried to allocate 2.00 GiB",
        "CUDA error: out of memory, illegal memory access",
        "OutOfMemory on GPU: uncorrectable",
    ],
)
def test_out_of_memory_never_restarts_the_worker(message):
    """The context survives OOM, so a restart would only repeat it."""
    assert not should_restart_worker(RuntimeError(message))


def test_torch_out_of_memory_type_never_restarts_the_worker():
    """The type is a definitive not-poisoned signal even with fatal-looking text."""
    import torch

    exc = torch.cuda.OutOfMemoryError("CUDA illegal memory access")
    assert not should_restart_worker(exc)


@pytest.mark.parametrize(
    "exc",
    [
        ValueError("bad column"),
        OSError("disk hardware error"),
        RuntimeError("CUDA error: invalid device ordinal"),
    ],
    ids=["unrelated", "signature-without-gpu-indicator", "gpu-without-signature"],
)
def test_other_errors_propagate(exc):
    """Both a GPU indicator and a signature are required."""
    assert not should_restart_worker(exc)


def test_wrapped_fault_is_found_through_the_cause_chain():
    """A fault wrapped by user code still poisons the context."""
    try:
        try:
            raise RuntimeError("CUDA error: illegal memory access")
        except RuntimeError as inner:
            raise ValueError("scorer failed") from inner
    except ValueError as outer:
        assert should_restart_worker(outer)


def test_suppressed_context_is_still_inspected():
    """``raise ... from None`` does not hide the original fault."""
    try:
        try:
            raise RuntimeError("CUDA error: illegal memory access")
        except RuntimeError:
            raise ValueError("scorer failed") from None
    except ValueError as outer:
        assert should_restart_worker(outer)


def test_signature_and_indicator_on_different_links_do_not_combine():
    """A benign GPU wrapper around a disk error is not a poisoned context."""
    try:
        try:
            raise OSError("disk hardware error")
        except OSError as inner:
            raise RuntimeError("GPU worker failed") from inner
    except RuntimeError as outer:
        assert not should_restart_worker(outer)


def test_cyclic_chains_terminate():
    """A cause cycle does not loop forever."""
    a, b = ValueError("a"), ValueError("b")
    a.__cause__, b.__cause__ = b, a

    assert not should_restart_worker(a)
