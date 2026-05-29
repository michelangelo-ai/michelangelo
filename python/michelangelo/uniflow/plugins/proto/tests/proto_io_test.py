"""Tests for ProtoIO — protobuf read/write via fsspec."""

from __future__ import annotations

import tempfile
from unittest import TestCase
from unittest.mock import MagicMock, patch


class TestProtoIO(TestCase):
    """Roundtrip and contract tests for ProtoIO."""

    def test_write_returns_qualified_class_name_in_metadata(self):
        """write() encodes the exact 'module:ClassName' string — not a class object."""
        try:
            from google.protobuf import struct_pb2
        except ImportError:
            self.skipTest("google-protobuf not installed")

        from michelangelo.uniflow.plugins.proto.io import _META_VALUE_TYPE, ProtoIO

        with patch("fsspec.core.url_to_fs") as mock_url_to_fs:
            mock_url_to_fs.return_value = (MagicMock(), "/tmp/x.json")
            result = ProtoIO().write("/tmp/x.json", struct_pb2.Value(number_value=1.0))

        self.assertEqual(result[_META_VALUE_TYPE], "google.protobuf.struct_pb2:Value")

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

    def test_read_raises_value_error_on_unresolvable_class(self):
        """read() raises ValueError when the encoded class cannot be imported."""
        from michelangelo.uniflow.plugins.proto.io import ProtoIO

        with self.assertRaises(ValueError) as ctx:
            ProtoIO().read("/tmp/x.json", {"value_type": "no.such.module:NoSuchClass"})
        self.assertIn("no.such.module:NoSuchClass", str(ctx.exception))
