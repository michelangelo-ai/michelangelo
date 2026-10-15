"""Tests for ModelBackend, resolution and building."""

from __future__ import annotations

import os

import pytest

from michelangelo.lib.inference import (
    BackendContext,
    ModelBackend,
    build_backend,
    default_device,
    resolve_backend,
)
from michelangelo.lib.inference.raw_backend import create_raw_backend

_FIX = "michelangelo.lib.inference.tests.fixtures"


def test_protocol_accepts_conforming_object_and_rejects_others():
    """Isinstance checks that all three members exist."""
    from michelangelo.lib.inference.tests.fixtures import GoodBackend

    assert isinstance(GoodBackend(), ModelBackend)
    assert not isinstance(object(), ModelBackend)


def test_backend_context_is_frozen():
    """A context cannot be mutated after construction."""
    ctx = BackendContext(model_path="/tmp/x")
    assert ctx.device == "cpu"
    assert ctx.schema is None
    with pytest.raises(AttributeError):
        ctx.device = "cuda"


def test_default_device_is_cpu_or_cuda():
    """The device is one of the two supported strings."""
    assert default_device() in ("cpu", "cuda")


def test_default_device_without_torch(monkeypatch):
    """A missing torch install falls back to cpu."""
    import builtins

    real_import = builtins.__import__

    def fake_import(name, *args, **kwargs):
        if name == "torch":
            raise ImportError("no torch")
        return real_import(name, *args, **kwargs)

    monkeypatch.setattr(builtins, "__import__", fake_import)
    assert default_device() == "cpu"


def test_resolve_dotted_path_returns_factory():
    """A dotted path resolves to the imported callable."""
    from michelangelo.lib.inference.tests.fixtures import good_factory

    assert resolve_backend("/unused", backend=f"{_FIX}.good_factory") is good_factory


@pytest.mark.parametrize(
    ("path", "message"),
    [
        ("factory", "dotted path"),
        ("no_such_pkg_xyz.factory", "Could not import"),
        (f"{_FIX}.missing", "Could not import"),
        (f"{_FIX}.not_callable", "not callable"),
    ],
)
def test_resolve_bad_dotted_path(path, message):
    """Unusable backend paths fail with an actionable message."""
    with pytest.raises(ValueError, match=message):
        resolve_backend("/unused", backend=path)


def test_resolve_raw_package(tmp_path):
    """A package with metadata/type.yaml selects the raw backend."""
    (tmp_path / "metadata").mkdir()
    (tmp_path / "metadata" / "type.yaml").write_text("type: torch\n")
    assert resolve_backend(str(tmp_path)) is create_raw_backend


@pytest.mark.parametrize("name", ["model.onnx", "model.pt"])
def test_resolve_deployable_package_fails_loudly(tmp_path, name):
    """A serving bundle is never silently scored as a raw model."""
    (tmp_path / "0").mkdir()
    (tmp_path / "0" / name).write_text("x")
    with pytest.raises(ValueError, match="deployable"):
        resolve_backend(str(tmp_path))


def test_resolve_non_package(tmp_path):
    """A directory with no package files is reported as such."""
    with pytest.raises(FileNotFoundError, match="metadata/type.yaml"):
        resolve_backend(str(tmp_path))
    assert os.path.isdir(tmp_path)


def test_build_backend_returns_conforming_instance():
    """A good factory result passes through."""
    from michelangelo.lib.inference.tests.fixtures import good_factory

    backend = build_backend(good_factory, BackendContext("/unused"))
    assert backend.predict({"a": 1}) == {"y": 1}


def test_build_backend_names_missing_members():
    """A non-conforming result lists exactly what is missing."""
    from michelangelo.lib.inference.tests.fixtures import bad_factory

    with pytest.raises(ValueError) as err:
        build_backend(bad_factory, BackendContext("/unused"))
    assert "output_names" in str(err.value)
    assert "predict" in str(err.value)
