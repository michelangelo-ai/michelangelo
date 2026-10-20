"""Shared fixtures for the sandbox CLI tests."""

import pytest


@pytest.fixture(autouse=True)
def isolate_kubeconfig(monkeypatch):
    """Point kubectl and helm at a missing kubeconfig.

    Some tests leave subprocess unpatched, so with a real kubeconfig a command
    such as helm uninstall would act on the developer's sandbox.
    """
    monkeypatch.setenv("KUBECONFIG", "/nonexistent/kubeconfig")
