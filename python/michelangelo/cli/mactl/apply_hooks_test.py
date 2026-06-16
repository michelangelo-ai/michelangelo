"""Tests for the pre-apply hook registry."""

import pytest

from michelangelo.cli.mactl import apply_hooks


@pytest.fixture(autouse=True)
def reset_registry():
    """Clear registered checks between tests."""
    apply_hooks._pre_apply_checks.clear()
    yield
    apply_hooks._pre_apply_checks.clear()


def test_no_checks_registered_is_noop():
    """An empty registry must not raise."""
    apply_hooks.run_pre_apply_checks("michelangelo.api.v1.Pipeline")


def test_single_check_receives_crd_full_name():
    """A registered check is invoked with the CRD full name."""
    received: list[str] = []
    apply_hooks.register_pre_apply_check(received.append)

    apply_hooks.run_pre_apply_checks("michelangelo.api.v1.Pipeline")

    assert received == ["michelangelo.api.v1.Pipeline"]


def test_multiple_checks_run_in_registration_order():
    """Checks fire in the order they were registered."""
    order: list[str] = []
    apply_hooks.register_pre_apply_check(lambda _: order.append("first"))
    apply_hooks.register_pre_apply_check(lambda _: order.append("second"))
    apply_hooks.register_pre_apply_check(lambda _: order.append("third"))

    apply_hooks.run_pre_apply_checks("michelangelo.api.v1.Pipeline")

    assert order == ["first", "second", "third"]


def test_raising_check_aborts_remaining_checks():
    """A raising check propagates and skips later checks."""
    later_ran = False

    def raises(_: str) -> None:
        raise RuntimeError("nope")

    def later(_: str) -> None:
        nonlocal later_ran
        later_ran = True

    apply_hooks.register_pre_apply_check(raises)
    apply_hooks.register_pre_apply_check(later)

    with pytest.raises(RuntimeError, match="nope"):
        apply_hooks.run_pre_apply_checks("michelangelo.api.v1.Pipeline")

    assert later_ran is False
