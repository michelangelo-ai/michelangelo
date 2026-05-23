"""Tests for workflow/schema/sinks config dataclasses."""

from __future__ import annotations

from unittest import TestCase

from michelangelo.workflow.schema.pusher import DatasetFormat
from michelangelo.workflow.schema.sinks import (
    HiveSinkConfig,
    InMemorySinkConfig,
    LocalFileSinkConfig,
)


class TestHiveSinkConfig(TestCase):
    """Tests for HiveSinkConfig."""

    def test_required_fields(self):
        """It stores database and table."""
        cfg = HiveSinkConfig(database="ml", table="predictions")
        self.assertEqual(cfg.database, "ml")
        self.assertEqual(cfg.table, "predictions")

    def test_default_mode_is_overwrite(self):
        """It defaults mode to 'overwrite'."""
        cfg = HiveSinkConfig(database="ml", table="t")
        self.assertEqual(cfg.mode, "overwrite")

    def test_default_partition_by_is_empty(self):
        """It defaults partition_by to an empty list."""
        cfg = HiveSinkConfig(database="ml", table="t")
        self.assertEqual(cfg.partition_by, [])

    def test_accepts_valid_modes(self):
        """It accepts overwrite, append, ignore, error."""
        for mode in ("overwrite", "append", "ignore", "error"):
            cfg = HiveSinkConfig(database="ml", table="t", mode=mode)
            self.assertEqual(cfg.mode, mode)

    def test_rejects_invalid_mode(self):
        """It raises ValueError for unsupported write modes."""
        with self.assertRaises(ValueError):
            HiveSinkConfig(database="ml", table="t", mode="truncate")


class TestLocalFileSinkConfig(TestCase):
    """Tests for LocalFileSinkConfig."""

    def test_required_destination_path(self):
        """It stores destination_path."""
        cfg = LocalFileSinkConfig(destination_path="/tmp/out")
        self.assertEqual(cfg.destination_path, "/tmp/out")

    def test_default_format_is_parquet(self):
        """It defaults format to DatasetFormat.PARQUET."""
        cfg = LocalFileSinkConfig(destination_path="/tmp/out")
        self.assertEqual(cfg.format, DatasetFormat.PARQUET)

    def test_explicit_format(self):
        """It stores an explicit format."""
        cfg = LocalFileSinkConfig(destination_path="/tmp/out", format=DatasetFormat.CSV)
        self.assertEqual(cfg.format, DatasetFormat.CSV)

    def test_default_partition_by_is_empty(self):
        """It defaults partition_by to an empty list."""
        cfg = LocalFileSinkConfig(destination_path="/tmp/out")
        self.assertEqual(cfg.partition_by, [])


class TestInMemorySinkConfig(TestCase):
    """Tests for InMemorySinkConfig."""

    def test_instantiates_with_no_args(self):
        """It instantiates with no arguments."""
        cfg = InMemorySinkConfig()
        self.assertIsInstance(cfg, InMemorySinkConfig)

    def test_repr(self):
        """It has a useful repr."""
        cfg = InMemorySinkConfig()
        self.assertIn("InMemorySinkConfig", repr(cfg))


class TestSinkResult(TestCase):
    """Tests for SinkResult frozen dataclass."""

    def test_stores_uri_and_num_records(self):
        """It stores uri and num_records fields."""
        from michelangelo.workflow.schema.sinks import SinkResult

        r = SinkResult(uri="/tmp/data.parquet", num_records=3)
        self.assertEqual(r.uri, "/tmp/data.parquet")
        self.assertEqual(r.num_records, 3)

    def test_is_frozen(self):
        """It raises AttributeError on assignment."""
        from michelangelo.workflow.schema.sinks import SinkResult

        r = SinkResult(uri="/tmp/x", num_records=1)
        with self.assertRaises(AttributeError):
            r.uri = "/tmp/other"  # type: ignore[misc]

    def test_extra_defaults_to_empty_dict(self):
        """It defaults extra to an empty dict."""
        from michelangelo.workflow.schema.sinks import SinkResult

        r = SinkResult(uri="/tmp/x", num_records=0)
        self.assertEqual(r.extra, {})

    def test_extra_stores_metadata(self):
        """It stores arbitrary metadata in extra."""
        from michelangelo.workflow.schema.sinks import SinkResult

        r = SinkResult(uri="hive://ml.t", num_records=10, extra={"partitions": 3})
        self.assertEqual(r.extra["partitions"], 3)
