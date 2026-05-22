"""Tests for DataSink ABC (workflow/sinks/base.py) and SinkResult (schema/sinks/result.py)."""

from __future__ import annotations

from unittest import TestCase

from michelangelo.workflow.schema.sinks import SinkResult
from michelangelo.workflow.sinks import DataSink


class TestDataSinkABC(TestCase):
    """Tests for the DataSink abstract base class."""

    def test_cannot_be_instantiated_directly(self):
        """It raises TypeError when instantiated without implementing write()."""
        with self.assertRaises(TypeError):
            DataSink()  # type: ignore[abstract]


class TestSinkResult(TestCase):
    """Tests for the SinkResult frozen dataclass."""

    def test_stores_uri_and_num_records(self):
        """It stores uri and num_records fields."""
        r = SinkResult(uri="/tmp/data.parquet", num_records=3)
        self.assertEqual(r.uri, "/tmp/data.parquet")
        self.assertEqual(r.num_records, 3)

    def test_is_frozen(self):
        """It raises FrozenInstanceError on attribute assignment."""
        r = SinkResult(uri="/tmp/x", num_records=1)
        with self.assertRaises(AttributeError):
            r.uri = "/tmp/other"  # type: ignore[misc]

    def test_extra_defaults_to_empty_dict(self):
        """It defaults extra to an empty dict."""
        r = SinkResult(uri="/tmp/x", num_records=0)
        self.assertEqual(r.extra, {})

    def test_extra_stores_metadata(self):
        """It stores arbitrary metadata in extra."""
        r = SinkResult(uri="hive://ml.t", num_records=10, extra={"partitions": 3})
        self.assertEqual(r.extra["partitions"], 3)
