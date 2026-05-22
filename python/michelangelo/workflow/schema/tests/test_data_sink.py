"""Tests for workflow/schema/data_sink.py — DataSink sinks and HiveSink."""

from __future__ import annotations

import os
import sys
import tempfile
import types as _types
from unittest import TestCase
from unittest.mock import MagicMock, patch

import pandas as pd

from michelangelo.workflow.schema.data_sink import (
    DataSink,
    HiveSink,
    InMemorySink,
    LocalFileSink,
    SinkResult,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.pusher import DatasetFormat, DatasetPluginConfig
from michelangelo.workflow.variables import DatasetVariable

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


class TestLocalFileSinkCSV(TestCase):
    """Tests for LocalFileSink CSV output."""

    def test_writes_csv_file(self):
        """It writes a valid CSV file with header and data rows."""
        dest = tempfile.mkdtemp()
        sink = LocalFileSink(dest, format=DatasetFormat.CSV)
        result = sink.write(_artifact())
        self.assertTrue(os.path.exists(result.uri))
        df_out = pd.read_csv(result.uri)
        self.assertEqual(len(df_out), len(_DF))
        self.assertIn("name", df_out.columns)

    def test_returns_correct_sink_result(self):
        """It returns a SinkResult with the correct uri and num_records."""
        dest = tempfile.mkdtemp()
        sink = LocalFileSink(dest, format=DatasetFormat.CSV)
        result = sink.write(_artifact())
        self.assertEqual(result.num_records, len(_DF))
        self.assertTrue(result.uri.startswith(dest))


class TestLocalFileSinkParquet(TestCase):
    """Tests for LocalFileSink Parquet output."""

    def test_writes_parquet_file(self):
        """It writes a valid Parquet file with correct shape."""
        dest = tempfile.mkdtemp()
        sink = LocalFileSink(dest, format=DatasetFormat.PARQUET)
        result = sink.write(_artifact())
        df_out = pd.read_parquet(result.uri)
        self.assertEqual(df_out.shape[0], len(_DF))

    def test_empty_dataframe_writes_zero_row_parquet(self):
        """It writes a valid zero-row Parquet file for an empty artifact."""
        dest = tempfile.mkdtemp()
        sink = LocalFileSink(dest, format=DatasetFormat.PARQUET)
        result = sink.write(_artifact(pd.DataFrame()))
        self.assertEqual(result.num_records, 0)
        df_out = pd.read_parquet(result.uri)
        self.assertEqual(len(df_out), 0)


class TestLocalFileSinkJSON(TestCase):
    """Tests for LocalFileSink JSON Lines output."""

    def test_writes_json_lines_file(self):
        """It writes a JSON Lines file with one object per row."""
        import json

        dest = tempfile.mkdtemp()
        sink = LocalFileSink(dest, format=DatasetFormat.JSON)
        result = sink.write(_artifact())
        with open(result.uri) as f:
            lines = f.readlines()
        self.assertEqual(len(lines), len(_DF))
        for line in lines:
            obj = json.loads(line)
            self.assertIn("name", obj)


class TestLocalFileSinkDirectory(TestCase):
    """Tests for LocalFileSink directory creation."""

    def test_creates_destination_directory_if_absent(self):
        """It creates the destination directory automatically."""
        base = tempfile.mkdtemp()
        dest = os.path.join(base, "new_subdir", "nested")
        sink = LocalFileSink(dest, format=DatasetFormat.CSV)
        sink.write(_artifact())
        self.assertTrue(os.path.isdir(dest))

    def test_raises_type_error_for_non_pandas_artifact(self):
        """It raises TypeError when artifact.value is not a pandas DataFrame."""
        artifact = DatasetVariable(value={"not": "a dataframe"})
        sink = LocalFileSink(tempfile.mkdtemp(), format=DatasetFormat.CSV)
        with self.assertRaises(TypeError):
            sink.write(artifact)

    def test_raises_value_error_for_unsupported_format(self):
        """It raises ValueError for a DatasetFormat not handled by LocalFileSink."""
        from unittest.mock import MagicMock

        import pandas as pd
        bad_format = MagicMock()
        bad_format.value = "xyz"
        sink = LocalFileSink(tempfile.mkdtemp())
        sink._format = bad_format
        artifact = DatasetVariable(value=pd.DataFrame([{"x": 1}]))
        with self.assertRaises(ValueError):
            sink.write(artifact)


class TestInMemorySink(TestCase):
    """Tests for InMemorySink."""

    def test_write_stores_records(self):
        """It stores written records accessible via .records property."""
        sink = InMemorySink()
        sink.write(_artifact())
        self.assertEqual(len(sink.records), len(_DF))
        self.assertEqual(sink.records[0]["name"], "alice")

    def test_records_empty_before_write(self):
        """It returns an empty list before any write() call."""
        sink = InMemorySink()
        self.assertEqual(sink.records, [])

    def test_returns_memory_uri(self):
        """It returns a SinkResult with a memory:// URI."""
        sink = InMemorySink()
        result = sink.write(_artifact())
        self.assertTrue(result.uri.startswith("memory://"))

    def test_raises_type_error_for_non_pandas_artifact(self):
        """It raises TypeError when artifact.value is not a pandas DataFrame."""
        artifact = DatasetVariable(value={"not": "a dataframe"})
        with self.assertRaises(TypeError):
            InMemorySink().write(artifact)


class TestDatasetPluginConfigPostInit(TestCase):
    """Tests for DatasetPluginConfig.__post_init__ LocalFileSink auto-creation."""

    def test_auto_creates_local_file_sink_from_destination_path(self):
        """It creates a LocalFileSink when destination_path is set."""
        cfg = DatasetPluginConfig(destination_path="/tmp/out")
        self.assertEqual(len(cfg.sinks), 1)
        self.assertIsInstance(cfg.sinks[0], LocalFileSink)

    def test_explicit_sinks_not_overridden(self):
        """It does not auto-create a sink when sinks is already populated."""
        sink = InMemorySink()
        cfg = DatasetPluginConfig(sinks=[sink])
        self.assertIs(cfg.sinks[0], sink)

    def test_no_sinks_and_no_destination_path_leaves_sinks_empty(self):
        """It leaves sinks empty when neither sinks nor destination_path is set."""
        cfg = DatasetPluginConfig()
        self.assertEqual(cfg.sinks, [])

    def test_explicit_empty_sinks_not_overridden_by_destination_path(self):
        """It does not auto-create a sink when sinks=[] is passed explicitly."""
        cfg = DatasetPluginConfig(sinks=[], destination_path="/tmp/out")
        self.assertEqual(cfg.sinks, [])

    def test_raises_config_error_when_plugin_has_no_sinks(self):
        """DatasetPusherPlugin raises ConfigurationError when sinks is empty."""
        from michelangelo.workflow.tasks.pusher.plugins.dataset_plugin import (
            DatasetPusherPlugin,
        )

        with self.assertRaises(ConfigurationError):
            DatasetPusherPlugin(
                config=DatasetPluginConfig(),
                artifact=_artifact(),
            )


def _mock_pyspark_sql():
    """Return (mock_pyspark_sql_module, mock_df_class)."""
    mock_df_class = type("DataFrame", (), {})
    mock_sql = _types.SimpleNamespace(DataFrame=mock_df_class)
    return mock_sql, mock_df_class


class TestHiveSink(TestCase):
    """Tests for HiveSink — Spark-native Hive table writes."""

    def _make_spark_df(self, num_records: int = 3):
        """Return a mock Spark DataFrame with a write chain and count()."""
        mock_sql, mock_df_class = _mock_pyspark_sql()
        spark_df = mock_df_class()
        spark_df.write = MagicMock()
        spark_df.write.mode.return_value = spark_df.write
        spark_df.write.saveAsTable = MagicMock()
        spark_df.count = MagicMock(return_value=num_records)
        return mock_sql, spark_df

    def _artifact_from_spark(self, spark_df, mock_sql):
        from michelangelo.workflow.variables import DatasetVariable

        return DatasetVariable(value=spark_df)

    def _pyspark_mods(self, mock_sql):
        mock_pyspark = _types.SimpleNamespace(sql=mock_sql)
        return {"pyspark": mock_pyspark, "pyspark.sql": mock_sql}

    def test_writes_to_hive_table(self):
        """It calls spark_df.write.mode(mode).saveAsTable(database.table)."""
        mock_sql, spark_df = self._make_spark_df()
        artifact = self._artifact_from_spark(spark_df, mock_sql)
        sink = HiveSink(database="ml", table="features")
        with patch.dict(sys.modules, self._pyspark_mods(mock_sql)):
            sink.write(artifact)
        spark_df.write.mode.assert_called_once_with("overwrite")
        spark_df.write.saveAsTable.assert_called_once_with("ml.features")

    def test_count_called_before_save(self):
        """It counts rows before saveAsTable to avoid a second scan."""
        call_order = []
        mock_sql, spark_df = self._make_spark_df()
        spark_df.count = MagicMock(side_effect=lambda: call_order.append("count") or 3)
        spark_df.write.saveAsTable = MagicMock(
            side_effect=lambda *a: call_order.append("save")
        )
        artifact = self._artifact_from_spark(spark_df, mock_sql)
        sink = HiveSink(database="ml", table="features")
        with patch.dict(sys.modules, self._pyspark_mods(mock_sql)):
            sink.write(artifact)
        self.assertEqual(call_order, ["count", "save"])

    def test_raises_value_error_for_invalid_mode(self):
        """It raises ValueError for an unsupported write mode."""
        with self.assertRaises(ValueError):
            HiveSink(database="ml", table="t", mode="truncate")

    def test_returns_hive_uri(self):
        """It returns a SinkResult with a hive:// URI."""
        mock_sql, spark_df = self._make_spark_df(num_records=5)
        artifact = self._artifact_from_spark(spark_df, mock_sql)
        sink = HiveSink(database="ml", table="training_data")
        with patch.dict(sys.modules, self._pyspark_mods(mock_sql)):
            result = sink.write(artifact)
        self.assertEqual(result.uri, "hive://ml.training_data")
        self.assertEqual(result.num_records, 5)

    def test_respects_append_mode(self):
        """It passes 'append' mode to Spark's write."""
        mock_sql, spark_df = self._make_spark_df()
        artifact = self._artifact_from_spark(spark_df, mock_sql)
        sink = HiveSink(database="ml", table="logs", mode="append")
        with patch.dict(sys.modules, self._pyspark_mods(mock_sql)):
            sink.write(artifact)
        spark_df.write.mode.assert_called_once_with("append")

    def test_raises_import_error_when_pyspark_missing(self):
        """It raises ImportError when pyspark is not installed."""
        from michelangelo.workflow.variables import DatasetVariable

        artifact = DatasetVariable(value=MagicMock())
        sink = HiveSink(database="ml", table="t")
        with patch.dict(sys.modules, {"pyspark": None, "pyspark.sql": None}), \
                self.assertRaises(ImportError):
            sink.write(artifact)

    def test_raises_type_error_for_non_spark_artifact(self):
        """It raises TypeError when artifact.value is not a Spark DataFrame."""
        from michelangelo.workflow.variables import DatasetVariable

        mock_sql, _ = _mock_pyspark_sql()
        artifact = DatasetVariable(value=_DF.copy())
        sink = HiveSink(database="ml", table="t")
        mods = {"pyspark": MagicMock(), "pyspark.sql": mock_sql}
        with patch.dict(sys.modules, mods), self.assertRaises(TypeError):
            sink.write(artifact)
