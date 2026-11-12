"""Tests for GPU poisoned-context detection."""

from __future__ import annotations

from unittest import TestCase

import torch

from michelangelo.lib.inference._private.lib.gpu_errors import (
    _MAX_EXCEPTION_CHAIN_DEPTH,
    _should_restart_actor_for_gpu_error,
)


def _accelerator_error(message: str) -> RuntimeError:
    """Build a stand-in for torch.AcceleratorError, which needs torch >= 2.8.

    The matcher keys on the type NAME so it does not need the real class.
    """
    return type("AcceleratorError", (RuntimeError,), {})(message)


class ShouldRestartActorForGpuErrorTest(TestCase):
    """Restart only for GPU faults that poison the CUDA context.

    Those are True anywhere in the exception chain. Everything else, including
    CUDA OOM, is False.
    """

    def test_matches_poisoned_context_signatures(self):
        for message in (
            # A real NVLink fault message.
            "CUDA error: Invalid access of peer GPU memory over nvlink "
            "or a hardware error",
            "CUDA error: an illegal memory access was encountered",
            "CUDA error: uncorrectable ECC error encountered",
            "CUDA error: unspecified launch failure",
            "AcceleratorError: device-side assert triggered",
        ):
            self.assertTrue(
                _should_restart_actor_for_gpu_error(RuntimeError(message)), message
            )

    def test_ignores_recoverable_errors(self):
        for exc in (
            # CUDA OOM leaves the context usable and is a sizing problem —
            # restarting and
            # retrying the same batch would just OOM again.
            RuntimeError("CUDA out of memory. Tried to allocate 2.00 GiB"),
            # torch.AcceleratorError form of the same OOM: contains "CUDA
            # error" but must
            # still be treated as recoverable.
            RuntimeError("CUDA error: out of memory"),
            RuntimeError("CUDA_ERROR_OUT_OF_MEMORY: out of memory"),
            # Poisoned-context words without any GPU indicator:
            # application/storage errors.
            ValueError("uncorrectable input"),
            OSError("disk hardware error on /dev/sdb"),
            ValueError("bad input shape"),
            OSError("Could not open Parquet input source"),
        ):
            self.assertFalse(_should_restart_actor_for_gpu_error(exc), str(exc))

    def test_matches_fault_wrapped_in_explicit_cause_chain(self):
        # Model code or a library frequently re-raises: `raise
        # RuntimeError(...) from e`.
        try:
            try:
                raise RuntimeError(
                    "CUDA error: an illegal memory access was encountered"
                )
            except RuntimeError as inner:
                raise ValueError("model failed on batch 17") from inner
        except ValueError as wrapped:
            self.assertTrue(_should_restart_actor_for_gpu_error(wrapped))

    def test_matches_fault_in_implicit_context_chain(self):
        # A different error raised while handling the CUDA fault (no `from`):
        # the fault is
        # only reachable via __context__, and the process is poisoned all the same.
        try:
            try:
                raise RuntimeError("CUDA error: uncorrectable ECC error encountered")
            except RuntimeError:
                raise KeyError("cleanup lookup failed")  # noqa: B904 - implicit chaining is the point
        except KeyError as wrapped:
            self.assertTrue(_should_restart_actor_for_gpu_error(wrapped))

    def test_matches_fault_behind_suppressed_context(self):
        try:
            try:
                raise RuntimeError("CUDA error: unspecified launch failure")
            except RuntimeError:
                raise ValueError("opaque wrapper") from None
        except ValueError as wrapped:
            self.assertTrue(_should_restart_actor_for_gpu_error(wrapped))

    def test_wrapped_oom_is_still_not_a_restart_trigger(self):
        try:
            try:
                raise RuntimeError("CUDA out of memory. Tried to allocate 2.00 GiB")
            except RuntimeError as inner:
                raise ValueError("model failed") from inner
        except ValueError as wrapped:
            self.assertFalse(_should_restart_actor_for_gpu_error(wrapped))

    def test_wrapped_benign_error_is_not_a_restart_trigger(self):
        # Wrapping alone must not make an ordinary error look fatal.
        try:
            try:
                raise OSError("Could not open Parquet input source")
            except OSError as inner:
                raise RuntimeError("GPU worker: read failed") from inner
        except RuntimeError as wrapped:
            self.assertFalse(_should_restart_actor_for_gpu_error(wrapped))

    def test_exception_chain_walk_is_bounded(self):
        # Cycle: a.__cause__ -> b, b.__cause__ -> a. Must terminate and still match.
        a = RuntimeError("wrapper A")
        b = RuntimeError("CUDA error: an illegal memory access was encountered")
        a.__cause__ = b
        b.__cause__ = a
        self.assertTrue(_should_restart_actor_for_gpu_error(a))
        # Fault buried deeper than the depth cap is not reached (bounded work,
        # benign miss).
        head = current = RuntimeError("wrapper 0")
        for i in range(1, _MAX_EXCEPTION_CHAIN_DEPTH):
            nxt = RuntimeError(f"wrapper {i}")
            current.__cause__ = nxt
            current = nxt
        current.__cause__ = RuntimeError(
            "CUDA error: an illegal memory access was encountered"
        )
        self.assertFalse(_should_restart_actor_for_gpu_error(head))

    def test_cross_link_indicator_and_signature_do_not_combine(self):
        # The GPU indicator on the wrapper and the poisoned signature
        # on the cause must NOT combine into a restart. Each condition is
        # evaluated per link.
        try:
            try:
                raise OSError("disk hardware error on /dev/sdb")
            except OSError as inner:
                raise RuntimeError("GPU worker failed") from inner
        except RuntimeError as wrapped:
            self.assertFalse(_should_restart_actor_for_gpu_error(wrapped))

    def test_torch_oom_type_is_never_a_restart_trigger(self):
        # Type-based short-circuit: torch.cuda.OutOfMemoryError is definitive
        # even when its
        # message carries no OOM wording (or, pathologically, poisoned-context wording).
        self.assertFalse(
            _should_restart_actor_for_gpu_error(torch.cuda.OutOfMemoryError(""))
        )
        self.assertFalse(
            _should_restart_actor_for_gpu_error(
                torch.cuda.OutOfMemoryError("CUDA error: unspecified launch failure")
            )
        )

    def test_accelerator_error_type_counts_as_gpu_indicator(self):
        # torch >= 2.8 raises torch.AcceleratorError; the type alone proves
        # GPU origin, so a
        # message without "cuda"/"gpu" still matches when it carries a
        # poisoned signature.
        self.assertTrue(
            _should_restart_actor_for_gpu_error(
                _accelerator_error("unspecified launch failure")
            )
        )
        # Same message as a bare RuntimeError has no GPU indicator and must not match.
        self.assertFalse(
            _should_restart_actor_for_gpu_error(
                RuntimeError("unspecified launch failure")
            )
        )
        # ... and AcceleratorError without a poisoned signature (e.g. OOM)
        # still does not match.
        self.assertFalse(
            _should_restart_actor_for_gpu_error(
                _accelerator_error("CUDA error: out of memory")
            )
        )

    def test_poisoned_fault_raised_while_handling_oom_still_restarts(self):
        # Per-link OOM check: an OOM earlier in the chain does not mask a real
        # poisoned-context fault that happened while handling it. The process is
        # unusable either way.
        try:
            try:
                raise RuntimeError("CUDA out of memory. Tried to allocate 2.00 GiB")
            except RuntimeError:
                raise RuntimeError(  # noqa: B904
                    "CUDA error: an illegal memory access was encountered"
                )
        except RuntimeError as wrapped:
            self.assertTrue(_should_restart_actor_for_gpu_error(wrapped))
