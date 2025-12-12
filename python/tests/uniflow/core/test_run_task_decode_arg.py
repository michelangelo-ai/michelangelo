"""Tests for the _decode_arg function in run_task module."""

import argparse
import logging
import unittest

from michelangelo.uniflow.core.run_task import _decode_arg


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

        with self.assertRaises(argparse.ArgumentTypeError), \
             self.assertLogs(level=logging.ERROR) as cm:
            _decode_arg(value)

        # Verify log record
        self.assertEqual(len(cm.records), 1)
        record = cm.records[0]
        self.assertEqual(record.levelname, "ERROR")
        self.assertIn(f"Failed to decode argument: {value}", record.message)
        # Verify exc_info is present (stack trace)
        self.assertIsNotNone(record.exc_info)
