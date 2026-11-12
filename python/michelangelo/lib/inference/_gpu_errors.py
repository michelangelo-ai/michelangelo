"""Detect GPU faults that leave a worker's CUDA context unusable.

After an illegal memory access or a hardware fault such as an NVLink or ECC
error, every later kernel launch in the process fails, so retrying in the same
process cannot succeed. Only a fresh process helps, and the predictor exits its
worker so Ray can restart the actor. Every other error, including CUDA
out-of-memory, propagates as usual.

Detection is mostly by message text. Before PyTorch 2.8 every CUDA fault is a
plain ``RuntimeError`` whose message is the only clue, and ONNX Runtime and
other engines raise their own exception types. Types are used only where they
are unambiguous: ``torch.cuda.OutOfMemoryError`` means the context is intact,
and an ``AcceleratorError`` means the error came from the GPU stack.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Iterator

try:  # Workers without PyTorch cannot raise its out-of-memory type.
    import torch

    _TORCH_OOM_TYPES: tuple[type, ...] = (torch.cuda.OutOfMemoryError,)
except Exception:
    _TORCH_OOM_TYPES = ()

# The message must name the GPU stack, so unrelated "hardware error" or
# "uncorrectable" text from storage or application code does not kill the actor,
_GPU_ERROR_INDICATORS = ("cuda", "gpu", "nvlink", "acceleratorerror", "nvidia")
# and carry one of the poisoned-context signatures.
_POISONED_CONTEXT_SIGNATURES = (
    "illegal memory access",
    "illegal instruction",
    "invalid access of peer gpu memory",
    "hardware error",
    "over nvlink",
    "ecc error",
    "uncorrectable",
    "unspecified launch failure",
    "device-side assert",
)
# Out-of-memory is never a restart trigger, even when it is reported as "CUDA
# error: out of memory": the context stays usable and a restart would rerun the
# same batch into the same error. It is a batch-size problem.
_OOM_SIGNATURES = ("out of memory", "outofmemory", "out_of_memory")
# Exception types that prove the error came from the GPU stack. Matched by name
# so this works without a PyTorch that defines them.
_GPU_EXCEPTION_TYPE_NAMES = frozenset({"AcceleratorError"})
# Bounds the ``__cause__`` / ``__context__`` walk. Real chains are 1-3 deep.
_MAX_EXCEPTION_CHAIN_DEPTH = 8


def _iter_exception_chain(exc: BaseException) -> Iterator[BaseException]:
    """Yield ``exc`` and the exceptions chained under it.

    Libraries often wrap the original fault, so the poisoned-context text can
    sit deeper than the outermost exception. Suppressed contexts
    (``raise ... from None``) are followed too, because the CUDA context is
    poisoned however the fault was wrapped. The walk is cycle-safe and bounded
    by ``_MAX_EXCEPTION_CHAIN_DEPTH``.
    """
    seen: set[int] = set()
    current: BaseException | None = exc
    while (
        current is not None
        and id(current) not in seen
        and len(seen) < _MAX_EXCEPTION_CHAIN_DEPTH
    ):
        seen.add(id(current))
        yield current
        current = current.__cause__ or current.__context__


def _is_cuda_oom(exc: BaseException, text: str) -> bool:
    return isinstance(exc, _TORCH_OOM_TYPES) or any(
        sig in text for sig in _OOM_SIGNATURES
    )


def _is_poisoned_context_error(exc: BaseException) -> bool:
    """Whether this one exception, ignoring its chain, poisons the CUDA context.

    The GPU indicator and the signature are checked on the same exception on
    purpose. Matching them against text joined from the whole chain would let
    ``RuntimeError("GPU worker failed")`` caused by ``OSError("disk hardware
    error")`` satisfy the indicator on one link and the signature on another.
    """
    text = f"{type(exc).__name__}: {exc}".lower()
    if _is_cuda_oom(exc, text):
        return False
    from_gpu_stack = type(exc).__name__ in _GPU_EXCEPTION_TYPE_NAMES or any(
        indicator in text for indicator in _GPU_ERROR_INDICATORS
    )
    return from_gpu_stack and any(sig in text for sig in _POISONED_CONTEXT_SIGNATURES)


def should_restart_worker(exc: BaseException) -> bool:
    """Whether ``exc`` or an exception chained under it poisoned the CUDA context.

    Args:
        exc: The exception raised while scoring a batch.

    Returns:
        ``True`` when the only useful response is to exit the worker process so
        Ray restarts the actor on a fresh CUDA context. ``False`` for every
        other error, including CUDA out-of-memory.
    """
    return any(_is_poisoned_context_error(link) for link in _iter_exception_chain(exc))
