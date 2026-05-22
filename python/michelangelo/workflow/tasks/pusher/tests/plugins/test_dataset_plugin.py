"""Tests for DatasetPusherPlugin — sink dispatch and DatasetVariable integration."""

from __future__ import annotations

import os
import tempfile
from unittest import TestCase

import pandas as pd

from michelangelo.workflow.schema.data_sink import (
    DataSink,
    InMemorySink,
    LocalFileSink,
    SinkResult,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.pusher import DatasetFormat, DatasetPluginConfig
from michelangelo.workflow.tasks.pusher.plugins.dataset_plugin import (
    DatasetPusherPlugin,
)
from michelangelo.workflow.variables._private.dataset import DatasetVariable

_RECORDS = [
    {"name": "alice", "score": 0.92},
    {"name": "bob", "score": 0.88},
    {"name": "carol", "score": 0.95},
]
_DF = pd.DataFrame(_RECORDS)


def _artifact(records: list | None = None) -> DatasetVariable:
    """Return a DatasetVariable wrapping a pandas DataFrame."""
    df = pd.DataFrame(records if records is not None else _RECORDS)
    return DatasetVariable(value=df)


def _make_plugin(
    artifact: DatasetVariable | None = None,
    dest: str | None = None,
    fmt: DatasetFormat = DatasetFormat.CSV,
) -> DatasetPusherPlugin:
    """Return a DatasetPusherPlugin with a LocalFileSink via destination_path."""
    return DatasetPusherPlugin(
        config=DatasetPluginConfig(
            destination_path=dest or tempfile.mkdtemp(),
            format=fmt,
        ),
        artifact=artifact or _artifact(),
    )


class TestDatasetVariable(TestCase):
    """Tests for DatasetVariable construction and to_pandas()."""

    def test_value_stores_dataframe(self):
        """DatasetVariable(value=df).value is the DataFrame."""
        artifact = DatasetVariable(value=_DF.copy())
        self.assertIsInstance(artifact.value, pd.DataFrame)
        self.assertEqual(len(artifact.value), len(_RECORDS))

    def test_load_pandas_dataframe_restores_value(self):
        """load_pandas_dataframe() restores the value after save."""
        import tempfile

        df = _DF.copy()
        dest = tempfile.mkdtemp()
        artifact = DatasetVariable(value=df, path=dest)
        artifact.save()
        restored = DatasetVariable(path=dest)
        restored.load_pandas_dataframe()
        self.assertIsInstance(restored.value, type(df))
        self.assertEqual(len(restored.value), len(_RECORDS))

    def test_backend_is_pandas(self):
        """Backend property returns 'pandas' for a DataFrame value."""
        artifact = DatasetVariable(value=_DF.copy())
        self.assertEqual(artifact.backend, "pandas")


class TestDatasetPusherPluginInit(TestCase):
    """Tests for DatasetPusherPlugin.__init__() validation."""

    def test_raises_when_no_sinks_configured(self):
        """It raises ConfigurationError when no sinks or destination_path is set."""
        with self.assertRaises(ConfigurationError) as ctx:
            DatasetPusherPlugin(
                config=DatasetPluginConfig(),
                artifact=_artifact(),
            )
        self.assertIn("sink", str(ctx.exception))

    def test_raises_when_artifact_is_none(self):
        """It raises ConfigurationError when artifact=None is passed."""
        with self.assertRaises(ConfigurationError) as ctx:
            DatasetPusherPlugin(
                config=DatasetPluginConfig(sinks=[InMemorySink()]),
                artifact=None,
            )
        self.assertIn("artifact", str(ctx.exception).lower())


class TestDatasetPusherPluginExecute(TestCase):
    """Tests for DatasetPusherPlugin.execute() sink dispatch."""

    def test_dispatches_to_in_memory_sink(self):
        """It calls write() on the configured InMemorySink."""
        sink = InMemorySink()
        plugin = DatasetPusherPlugin(
            config=DatasetPluginConfig(sinks=[sink]),
            artifact=_artifact(),
        )
        plugin.execute()
        self.assertEqual(len(sink.records), len(_RECORDS))
        self.assertEqual(sink.records[0]["name"], "alice")

    def test_dispatches_to_multiple_sinks(self):
        """It writes to each sink in order."""
        sink1 = InMemorySink()
        sink2 = InMemorySink()
        plugin = DatasetPusherPlugin(
            config=DatasetPluginConfig(sinks=[sink1, sink2]),
            artifact=_artifact(),
        )
        plugin.execute()
        self.assertEqual(len(sink1.records), len(_RECORDS))
        self.assertEqual(len(sink2.records), len(_RECORDS))

    def test_returns_sinks_list_in_result(self):
        """It returns a dict with a 'sinks' list containing per-sink results."""
        sink = InMemorySink()
        plugin = DatasetPusherPlugin(
            config=DatasetPluginConfig(sinks=[sink]),
            artifact=_artifact(),
        )
        result = plugin.execute()
        self.assertIn("sinks", result)
        self.assertEqual(len(result["sinks"]), 1)
        self.assertIn("uri", result["sinks"][0])
        self.assertIn("num_records", result["sinks"][0])

    def test_num_records_from_first_sink(self):
        """It sets num_records from the first sink's result."""
        records = [{"x": i} for i in range(7)]
        plugin = DatasetPusherPlugin(
            config=DatasetPluginConfig(sinks=[InMemorySink()]),
            artifact=_artifact(records),
        )
        result = plugin.execute()
        self.assertEqual(result["num_records"], 7)

    def test_destination_path_is_first_sink_uri(self):
        """It sets destination_path to the first sink's URI for backwards compat."""
        dest = tempfile.mkdtemp()
        plugin = _make_plugin(dest=dest)
        result = plugin.execute()
        self.assertEqual(result["destination_path"], result["sinks"][0]["uri"])
        self.assertTrue(result["destination_path"].startswith(dest))

    def test_local_file_sink_csv_roundtrip(self):
        """It writes a valid CSV file via LocalFileSink."""
        dest = tempfile.mkdtemp()
        plugin = _make_plugin(dest=dest, fmt=DatasetFormat.CSV)
        result = plugin.execute()
        self.assertTrue(os.path.exists(result["destination_path"]))
        df_out = pd.read_csv(result["destination_path"])
        self.assertEqual(len(df_out), len(_RECORDS))

    def test_local_file_sink_parquet_roundtrip(self):
        """It writes a valid Parquet file via LocalFileSink."""
        dest = tempfile.mkdtemp()
        plugin = _make_plugin(dest=dest, fmt=DatasetFormat.PARQUET)
        result = plugin.execute()
        df_out = pd.read_parquet(result["destination_path"])
        self.assertEqual(df_out.shape[0], len(_RECORDS))

    def test_empty_dataframe_writes_zero_row_parquet(self):
        """It writes a valid zero-row Parquet file for an empty artifact."""
        dest = tempfile.mkdtemp()
        plugin = DatasetPusherPlugin(
            config=DatasetPluginConfig(
                destination_path=dest, format=DatasetFormat.PARQUET
            ),
            artifact=DatasetVariable(value=pd.DataFrame()),
        )
        result = plugin.execute()
        self.assertEqual(result["num_records"], 0)
        df_out = pd.read_parquet(result["destination_path"])
        self.assertEqual(len(df_out), 0)

    def test_destination_path_shorthand_auto_creates_local_file_sink(self):
        """It auto-creates a LocalFileSink when destination_path shorthand is used."""
        dest = tempfile.mkdtemp()
        cfg = DatasetPluginConfig(destination_path=dest)
        self.assertEqual(len(cfg.sinks), 1)
        self.assertIsInstance(cfg.sinks[0], LocalFileSink)

    def test_sink_extra_metadata_appears_in_result(self):
        """It includes SinkResult.extra fields in the per-sink result dict."""

        class _ExtraSink(DataSink):
            def write(self, artifact: DatasetVariable) -> SinkResult:  # type: ignore[override]
                return SinkResult(
                    uri="custom://target",
                    num_records=3,
                    extra={"table": "ml.evals", "partition": "2026"},
                )

        plugin = DatasetPusherPlugin(
            config=DatasetPluginConfig(sinks=[_ExtraSink()]),
            artifact=_artifact(),
        )
        result = plugin.execute()
        self.assertEqual(result["sinks"][0]["table"], "ml.evals")
        self.assertEqual(result["sinks"][0]["partition"], "2026")
