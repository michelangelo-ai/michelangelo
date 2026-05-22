"""Tests for PyTorchStateDictIO — state dict read/write."""

from __future__ import annotations

import tempfile
from unittest import TestCase
from unittest.mock import MagicMock, patch


class TestPyTorchStateDictIO(TestCase):
    """Tests for PyTorchStateDictIO — uses mocked torch when not installed."""

    def _mock_torch(self):
        mock = MagicMock()
        mock.save = MagicMock(return_value=None)
        mock.load = MagicMock(return_value={"weight": "tensor", "bias": "tensor"})
        return mock

    def test_write_calls_torch_save(self):
        """write() calls torch.save with the state dict and path."""
        from michelangelo.uniflow.plugins.pytorch.io import PyTorchStateDictIO

        mock_torch = self._mock_torch()
        io = PyTorchStateDictIO()
        sd = {"weight": MagicMock(), "bias": MagicMock()}
        with patch.dict("sys.modules", {"torch": mock_torch}):
            result = io.write("/tmp/model.pt", sd)
        mock_torch.save.assert_called_once_with(sd, "/tmp/model.pt")
        self.assertIsNone(result)

    def test_read_calls_torch_load(self):
        """read() calls torch.load and returns the state dict."""
        from michelangelo.uniflow.plugins.pytorch.io import PyTorchStateDictIO

        mock_torch = self._mock_torch()
        io = PyTorchStateDictIO()
        with patch.dict("sys.modules", {"torch": mock_torch}):
            result = io.read("/tmp/model.pt", None)
        mock_torch.load.assert_called_once_with(
            "/tmp/model.pt", map_location=None, weights_only=True
        )
        self.assertIn("weight", result)

    def test_map_location_forwarded_to_load(self):
        """map_location is passed through to torch.load."""
        from michelangelo.uniflow.plugins.pytorch.io import PyTorchStateDictIO

        mock_torch = self._mock_torch()
        io = PyTorchStateDictIO(map_location="cpu")
        with patch.dict("sys.modules", {"torch": mock_torch}):
            io.read("/tmp/model.pt", None)
        mock_torch.load.assert_called_once_with(
            "/tmp/model.pt", map_location="cpu", weights_only=True
        )

    def test_write_returns_none(self):
        """write() always returns None."""
        from michelangelo.uniflow.plugins.pytorch.io import PyTorchStateDictIO

        mock_torch = self._mock_torch()
        io = PyTorchStateDictIO()
        with patch.dict("sys.modules", {"torch": mock_torch}):
            self.assertIsNone(io.write("/tmp/x.pt", {}))

    def test_roundtrip_with_real_torch(self):
        """write() + read() roundtrip using real torch if installed."""
        import importlib.util
        if importlib.util.find_spec("torch") is None:
            self.skipTest("torch not installed")
        import torch.nn as nn

        from michelangelo.uniflow.plugins.pytorch.io import PyTorchStateDictIO

        model = nn.Linear(4, 2)
        io = PyTorchStateDictIO(map_location="cpu")
        with tempfile.NamedTemporaryFile(suffix=".pt", delete=False) as f:
            path = f.name
        io.write(path, model.state_dict())
        restored = io.read(path, None)
        self.assertIn("weight", restored)
        self.assertIn("bias", restored)
