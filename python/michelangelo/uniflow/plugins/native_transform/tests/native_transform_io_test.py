"""Tests for the native_transform uniflow plugin's default_io registration."""

from __future__ import annotations

import pytest

pytest.importorskip("torch")
pytest.importorskip("pydantic")
pytest.importorskip("yaml")
pytest.importorskip("fsspec")


class TestNativeTransformIORegistration:
    """Importing the plugin registers TransformSpec in default_io."""

    def test_registered_in_default_io(self) -> None:
        """Importing the plugin package registers the handler."""
        import michelangelo.uniflow.plugins.native_transform  # noqa: F401
        from michelangelo.lib.native_transform.torch.transform_spec import (
            TransformSpec,
        )
        from michelangelo.uniflow.core.io_registry import default_io
        from michelangelo.uniflow.plugins.native_transform.io import TransformSpecIO

        assert TransformSpec in default_io
        assert isinstance(default_io[TransformSpec], TransformSpecIO)
