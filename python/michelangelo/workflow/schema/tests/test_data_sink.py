"""Tests for DataSink ABC and SinkResult."""

from __future__ import annotations

from unittest import TestCase

from michelangelo.workflow.schema.data_sink import DataSink, SinkResult
from michelangelo.workflow.variables import DatasetVariable

import pandas as pd

_DF = pd.DataFrame([{"name": "alice", "score": 0.92}, {"name": "bob", "score": 0.88}])


def _artifact(df: pd.DataFrame | None = None) -> DatasetVariable:
    return DatasetVariable(value=df if df is not None else _DF.copy())


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
