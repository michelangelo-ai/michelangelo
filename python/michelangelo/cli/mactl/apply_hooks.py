"""Pre-apply check registry — pluggable hook fired before every CRD apply.

Consumers register callbacks via ``register_pre_apply_check``. ``apply_func_impl``
invokes ``run_pre_apply_checks(crd_full_name)`` at the top of every apply, so a
registered callback can short-circuit the apply by raising.

This module exists so that downstream distributions (e.g. internal CLIs) can
enforce policy on every CRD without monkey-patching ``apply_func_impl`` or
wiring per-plugin call sites.
"""

from typing import Callable

PreApplyCheck = Callable[[str], None]

_pre_apply_checks: list[PreApplyCheck] = []


def register_pre_apply_check(fn: PreApplyCheck) -> None:
    """Register a callback to run before every CRD apply.

    The callback receives the CRD's gRPC full name (e.g.
    ``michelangelo.api.v1.Pipeline``) and should raise to abort the apply.
    Callbacks run in registration order; an exception from any callback
    propagates without running the remaining callbacks.
    """
    _pre_apply_checks.append(fn)


def run_pre_apply_checks(crd_full_name: str) -> None:
    """Run all registered pre-apply checks.

    Called from ``apply_func_impl``. If no callbacks are registered this is a
    no-op, which is the default behavior for OSS users.
    """
    for fn in _pre_apply_checks:
        fn(crd_full_name)
