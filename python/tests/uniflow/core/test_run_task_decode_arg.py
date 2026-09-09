"""Tests for the _decode_arg function in run_task module."""

import argparse
import json
import logging
import tempfile
import unittest

from michelangelo.uniflow.core.codec import encoder
from michelangelo.uniflow.core.run_task import _decode_arg, _decode_kwargs_file


class TestDecodeArg(unittest.TestCase):
    """Tests for _decode_arg."""

    def test_decode_arg_success(self):
        """Test successful decoding of valid JSON strings."""
        # Test dict
        value = '{"key": "value"}'
        result = _decode_arg(value)
        self.assertEqual(result, {"key": "value"})

        # Test list
        value = "[1, 2, 3]"
        result = _decode_arg(value)
        self.assertEqual(result, [1, 2, 3])

        # Test primitive
        value = "123"
        result = _decode_arg(value)
        self.assertEqual(result, 123)

    def test_decode_arg_failure(self):
        """Test that invalid JSON raises ArgumentTypeError."""
        value = "invalid json"

        with self.assertRaises(argparse.ArgumentTypeError) as cm:
            _decode_arg(value)

        self.assertIn(f"Failed to decode argument: {value}", str(cm.exception))

    def test_decode_arg_logging(self):
        """Test that decoding errors are logged with stack trace."""
        value = "invalid json"

        with (
            self.assertRaises(argparse.ArgumentTypeError),
            self.assertLogs(level=logging.ERROR) as cm,
        ):
            _decode_arg(value)

        # Verify log record
        self.assertEqual(len(cm.records), 1)
        record = cm.records[0]
        self.assertEqual(record.levelname, "ERROR")
        self.assertIn(f"Failed to decode argument: {value}", record.message)
        # Verify exc_info is present (stack trace)
        self.assertIsNotNone(record.exc_info)


class TestDecodeKwargsFile(unittest.TestCase):
    """Tests for _decode_kwargs_file."""

    def test_codec_values_match_inline_decoding(self):
        """Test that files retain the inline path's custom codec behavior."""
        encoded = json.dumps(
            {"payload": b"binary kwargs"},
            separators=(",", ":"),
            default=encoder.default,
        )
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json") as kwargs_file:
            kwargs_file.write(encoded)
            kwargs_file.flush()

            from_file = _decode_kwargs_file(kwargs_file.name)

        self.assertEqual(_decode_arg(encoded), from_file)

    def test_missing_file_retains_cause(self):
        """Test that file read failures retain their underlying exception."""
        path = "/a/kwargs/file/that/does/not/exist.json"

        with self.assertRaises(argparse.ArgumentTypeError) as cm:
            _decode_kwargs_file(path)

        self.assertEqual(f"Failed to decode kwargs file: {path}", str(cm.exception))
        self.assertIsInstance(cm.exception.__cause__, FileNotFoundError)

    def test_malformed_file_retains_cause_without_logging_contents(self):
        """Test that decode failures retain their cause without leaking contents."""
        secret_contents = "not-json-sensitive-contents"
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json") as kwargs_file:
            kwargs_file.write(secret_contents)
            kwargs_file.flush()

            with (
                self.assertRaises(argparse.ArgumentTypeError) as cm,
                self.assertLogs(level=logging.ERROR) as logs,
            ):
                _decode_kwargs_file(kwargs_file.name)

        self.assertIsInstance(cm.exception.__cause__, json.JSONDecodeError)
        self.assertIn(kwargs_file.name, logs.output[0])
        self.assertNotIn(secret_contents, logs.output[0])
