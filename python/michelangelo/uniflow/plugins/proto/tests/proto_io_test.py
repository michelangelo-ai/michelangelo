"""Tests for ProtoIO — protobuf read/write via fsspec."""

from __future__ import annotations

import tempfile
from unittest import TestCase
from unittest.mock import MagicMock, patch


class TestProtoIO(TestCase):
    """Roundtrip and contract tests for ProtoIO."""

    def _make_proto_env(self):
        """Return a minimal mock google.protobuf environment."""
        msg_class = MagicMock()
        instance = MagicMock()
        msg_class.return_value = instance
        mock_json_format = MagicMock()
        mock_json_format.MessageToJson.return_value = '{"stringValue": "hello"}'
        mock_json_format.Parse.return_value = None
        return msg_class, instance, mock_json_format

    def test_write_returns_value_type_metadata(self):
        """write() returns {'value_type': type(value)}."""
        from michelangelo.uniflow.plugins.proto.io import _META_VALUE_TYPE, ProtoIO

        msg_class, instance, mock_json_format = self._make_proto_env()
        instance.__class__ = msg_class

        with tempfile.NamedTemporaryFile(suffix=".json", delete=False) as f:
            path = f.name

        _mods = {
            "google.protobuf.json_format": mock_json_format,
            "google.protobuf": MagicMock(),
        }
        with patch.dict("sys.modules", _mods):
            io = ProtoIO()
            with patch("fsspec.core.url_to_fs") as mock_url_to_fs:
                mock_fs = MagicMock()
                mock_fs.open.return_value.__enter__ = lambda s: MagicMock()
                mock_fs.open.return_value.__exit__ = MagicMock(return_value=False)
                mock_url_to_fs.return_value = (mock_fs, path)
                result = io.write(path, instance)
            self.assertIn(_META_VALUE_TYPE, result)

    def test_write_read_roundtrip_with_local_file(self):
        """write() + read() roundtrip using a temp local file and real fsspec."""
        try:
            from google.protobuf import struct_pb2
        except ImportError:
            self.skipTest("google-protobuf not installed")

        from michelangelo.uniflow.plugins.proto.io import ProtoIO

        msg = struct_pb2.Value(string_value="hello")
        io = ProtoIO()
        with tempfile.NamedTemporaryFile(suffix=".json", delete=False) as f:
            path = f.name
        metadata = io.write(path, msg)
        restored = io.read(path, metadata)
        self.assertEqual(restored.string_value, "hello")

    def test_meta_key_constant(self):
        """_META_VALUE_TYPE equals 'value_type'."""
        from michelangelo.uniflow.plugins.proto.io import _META_VALUE_TYPE

        self.assertEqual(_META_VALUE_TYPE, "value_type")

    def test_read_raises_value_error_on_none_metadata(self):
        """read() raises ValueError when metadata is None."""
        from michelangelo.uniflow.plugins.proto.io import ProtoIO

        with self.assertRaises(ValueError) as ctx:
            ProtoIO().read("/tmp/x.json", None)
        self.assertIn("value_type", str(ctx.exception))

    def test_read_raises_value_error_on_empty_metadata(self):
        """read() raises ValueError when metadata dict is missing 'value_type'."""
        from michelangelo.uniflow.plugins.proto.io import ProtoIO

        with self.assertRaises(ValueError) as ctx:
            ProtoIO().read("/tmp/x.json", {})
        self.assertIn("value_type", str(ctx.exception))
