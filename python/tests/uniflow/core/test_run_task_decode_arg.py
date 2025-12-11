"""Tests for the _decode_arg function in run_task module."""

import argparse
import logging

import pytest

from michelangelo.uniflow.core.run_task import _decode_arg


def test_decode_arg_success():
    """Test successful decoding of valid JSON strings."""
    # Test dict
    value = '{"key": "value"}'
    result = _decode_arg(value)
    assert result == {"key": "value"}

    # Test list
    value = "[1, 2, 3]"
    result = _decode_arg(value)
    assert result == [1, 2, 3]

    # Test primitive
    value = "123"
    result = _decode_arg(value)
    assert result == 123


def test_decode_arg_failure():
    """Test that invalid JSON raises ArgumentTypeError."""
    value = "invalid json"

    with pytest.raises(argparse.ArgumentTypeError) as excinfo:
        _decode_arg(value)

    assert f"Failed to decode argument: {value}" in str(excinfo.value)


def test_decode_arg_logging(caplog):
    """Test that decoding errors are logged with stack trace."""
    value = "invalid json"

    with pytest.raises(argparse.ArgumentTypeError), caplog.at_level(logging.ERROR):
        _decode_arg(value)

    # Verify log record
    assert len(caplog.records) == 1
    record = caplog.records[0]
    assert record.levelname == "ERROR"
    assert f"Failed to decode argument: {value}" in record.message
    # Verify exc_info is present (stack trace)
    assert record.exc_info is not None
