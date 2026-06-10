"""Tests for MinioSink — upload dispatch and DatasetVariable integration."""

from __future__ import annotations

import os
import tempfile
from unittest import TestCase
from unittest.mock import MagicMock, patch

import pandas as pd

from michelangelo.workflow.schema.pusher import DatasetFormat
from michelangelo.workflow.schema.sinks.minio import MinioSinkConfig
from michelangelo.workflow.tasks.functions.sinks.minio import MinioSink
from michelangelo.workflow.variables._private.dataset import DatasetVariable

_RECORDS = [
    {"name": "alice", "score": 0.92},
    {"name": "bob", "score": 0.88},
    {"name": "carol", "score": 0.95},
]
_DF = pd.DataFrame(_RECORDS)


def _artifact(records: list | None = None) -> DatasetVariable:
    df = pd.DataFrame(records if records is not None else _RECORDS)
    return DatasetVariable(value=df)


def _mock_backend(uri: str = "s3://test-bucket/datasets/v1/data.parquet") -> MagicMock:
    """Return a mock MinioStorageBackend whose upload() returns the given URI."""
    backend = MagicMock()
    backend.upload.return_value = uri
    return backend


def _sink(
    *,
    key: str = "datasets/california/v1",
    fmt: DatasetFormat = DatasetFormat.PARQUET,
    backend: MagicMock | None = None,
) -> MinioSink:
    return MinioSink(
        MinioSinkConfig(destination_key=key, format=fmt),
        storage_backend=backend or _mock_backend(f"s3://test-bucket/{key}/data.{fmt.value}"),
    )


# ---------------------------------------------------------------------------
# MinioSinkConfig
# ---------------------------------------------------------------------------


class TestMinioSinkConfig(TestCase):
    """Tests for MinioSinkConfig defaults and field types."""

    def test_default_format_is_parquet(self):
        """Format defaults to PARQUET when not specified."""
        cfg = MinioSinkConfig(destination_key="datasets/v1")
        self.assertEqual(cfg.format, DatasetFormat.PARQUET)

    def test_explicit_format_is_preserved(self):
        """An explicitly set format is stored correctly."""
        cfg = MinioSinkConfig(destination_key="datasets/v1", format=DatasetFormat.CSV)
        self.assertEqual(cfg.format, DatasetFormat.CSV)

    def test_destination_key_is_stored(self):
        """The destination_key field is stored as-is."""
        cfg = MinioSinkConfig(destination_key="my/prefix/v3")
        self.assertEqual(cfg.destination_key, "my/prefix/v3")


# ---------------------------------------------------------------------------
# MinioSink.write() — object key construction
# ---------------------------------------------------------------------------


class TestMinioSinkObjectKey(TestCase):
    """Tests that MinioSink uploads to the correct object key."""

    def test_parquet_key_has_data_parquet_suffix(self):
        """Parquet format appends /data.parquet to the destination_key."""
        backend = _mock_backend()
        sink = MinioSink(
            MinioSinkConfig("datasets/v1", format=DatasetFormat.PARQUET),
            storage_backend=backend,
        )
        sink.write(_artifact())
        _, call_key = backend.upload.call_args[0]
        self.assertEqual(call_key, "datasets/v1/data.parquet")

    def test_csv_key_has_data_csv_suffix(self):
        """CSV format appends /data.csv to the destination_key."""
        backend = _mock_backend()
        sink = MinioSink(
            MinioSinkConfig("datasets/v1", format=DatasetFormat.CSV),
            storage_backend=backend,
        )
        sink.write(_artifact())
        _, call_key = backend.upload.call_args[0]
        self.assertEqual(call_key, "datasets/v1/data.csv")

    def test_json_key_has_data_json_suffix(self):
        """JSON format appends /data.json to the destination_key."""
        backend = _mock_backend()
        sink = MinioSink(
            MinioSinkConfig("datasets/v1", format=DatasetFormat.JSON),
            storage_backend=backend,
        )
        sink.write(_artifact())
        _, call_key = backend.upload.call_args[0]
        self.assertEqual(call_key, "datasets/v1/data.json")


# ---------------------------------------------------------------------------
# MinioSink.write() — SinkResult
# ---------------------------------------------------------------------------


class TestMinioSinkResult(TestCase):
    """Tests for the SinkResult returned by MinioSink.write()."""

    def test_uri_is_backend_upload_return_value(self):
        """The returned URI is whatever the backend's upload() returns."""
        expected_uri = "s3://my-bucket/datasets/v1/data.parquet"
        sink = _sink(backend=_mock_backend(expected_uri))
        result = sink.write(_artifact())
        self.assertEqual(result.uri, expected_uri)

    def test_num_records_matches_dataframe_length(self):
        """num_records equals the number of rows in the artifact."""
        sink = _sink()
        result = sink.write(_artifact())
        self.assertEqual(result.num_records, len(_RECORDS))

    def test_num_records_zero_for_empty_dataframe(self):
        """num_records is 0 for an empty DataFrame."""
        sink = _sink()
        result = sink.write(DatasetVariable(value=pd.DataFrame()))
        self.assertEqual(result.num_records, 0)


# ---------------------------------------------------------------------------
# MinioSink.write() — serialisation (temp file content passed to upload)
# ---------------------------------------------------------------------------


class TestMinioSinkSerialisation(TestCase):
    """Tests that MinioSink serialises the DataFrame correctly before upload."""

    def _capture_upload_path(self) -> tuple[MagicMock, list[str]]:
        """Return a mock backend that captures the local_path passed to upload()."""
        captured: list[str] = []
        backend = MagicMock()

        def _upload(local_path: str, key: str) -> str:
            captured.append(local_path)
            return f"s3://bucket/{key}"

        backend.upload.side_effect = _upload
        return backend, captured

    def test_parquet_temp_file_is_readable(self):
        """The temp file passed to upload() is a valid Parquet file."""
        backend, paths = self._capture_upload_path()
        # Patch os.unlink so we can read the temp file after write() completes.
        with patch("michelangelo.workflow.tasks.functions.sinks.minio.os.unlink"):
            MinioSink(
                MinioSinkConfig("d/v1", format=DatasetFormat.PARQUET),
                storage_backend=backend,
            ).write(_artifact())
        self.assertEqual(len(paths), 1)
        df_out = pd.read_parquet(paths[0])
        self.assertEqual(len(df_out), len(_RECORDS))
        os.unlink(paths[0])

    def test_csv_temp_file_is_readable(self):
        """The temp file passed to upload() is a valid CSV file."""
        backend, paths = self._capture_upload_path()
        with patch("michelangelo.workflow.tasks.functions.sinks.minio.os.unlink"):
            MinioSink(
                MinioSinkConfig("d/v1", format=DatasetFormat.CSV),
                storage_backend=backend,
            ).write(_artifact())
        df_out = pd.read_csv(paths[0])
        self.assertEqual(len(df_out), len(_RECORDS))
        os.unlink(paths[0])

    def test_json_temp_file_is_readable(self):
        """The temp file passed to upload() is a valid JSON Lines file."""
        import json

        backend, paths = self._capture_upload_path()
        with patch("michelangelo.workflow.tasks.functions.sinks.minio.os.unlink"):
            MinioSink(
                MinioSinkConfig("d/v1", format=DatasetFormat.JSON),
                storage_backend=backend,
            ).write(_artifact())
        with open(paths[0]) as f:
            lines = f.readlines()
        self.assertEqual(len(lines), len(_RECORDS))
        self.assertIn("name", json.loads(lines[0]))
        os.unlink(paths[0])

    def test_temp_file_is_cleaned_up_on_success(self):
        """The temporary file is deleted after a successful upload."""
        deleted: list[str] = []
        backend = MagicMock()
        backend.upload.return_value = "s3://b/k"

        original_unlink = os.unlink

        def _unlink(path: str) -> None:
            deleted.append(path)
            original_unlink(path)

        with patch("michelangelo.workflow.tasks.functions.sinks.minio.os.unlink", side_effect=_unlink):
            MinioSink(
                MinioSinkConfig("d/v1", format=DatasetFormat.PARQUET),
                storage_backend=backend,
            ).write(_artifact())

        self.assertEqual(len(deleted), 1)
        self.assertFalse(os.path.exists(deleted[0]))

    def test_temp_file_is_cleaned_up_on_upload_failure(self):
        """The temporary file is deleted even when the upload raises."""
        deleted: list[str] = []
        backend = MagicMock()
        backend.upload.side_effect = OSError("network error")

        original_unlink = os.unlink

        def _unlink(path: str) -> None:
            deleted.append(path)
            original_unlink(path)

        with patch("michelangelo.workflow.tasks.functions.sinks.minio.os.unlink", side_effect=_unlink):
            with self.assertRaises(OSError):
                MinioSink(
                    MinioSinkConfig("d/v1"),
                    storage_backend=backend,
                ).write(_artifact())

        self.assertEqual(len(deleted), 1)
        self.assertFalse(os.path.exists(deleted[0]))


# ---------------------------------------------------------------------------
# MinioSink.write() — error cases
# ---------------------------------------------------------------------------


class TestMinioSinkErrors(TestCase):
    """Tests for MinioSink error handling."""

    def test_raises_type_error_for_non_pandas_artifact(self):
        """It raises TypeError when artifact.value is not a pandas DataFrame."""
        artifact = DatasetVariable(value={"not": "a dataframe"})
        with self.assertRaises(TypeError) as ctx:
            _sink().write(artifact)
        self.assertIn("pandas.DataFrame", str(ctx.exception))

    def test_raises_value_error_for_unsupported_format(self):
        """It raises ValueError for an unrecognised DatasetFormat."""
        bad_fmt = MagicMock()
        bad_fmt.value = "xyz"
        cfg = MinioSinkConfig(destination_key="d/v1", format=bad_fmt)
        sink = MinioSink(cfg, storage_backend=_mock_backend())
        with self.assertRaises(ValueError):
            sink.write(_artifact())

    def test_propagates_oserror_from_backend(self):
        """It propagates OSError raised by the storage backend's upload()."""
        backend = MagicMock()
        backend.upload.side_effect = OSError("connection refused")
        sink = MinioSink(MinioSinkConfig("d/v1"), storage_backend=backend)
        with self.assertRaises(OSError, msg="connection refused"):
            sink.write(_artifact())

    def test_upload_called_once_per_write(self):
        """upload() is called exactly once per write() invocation."""
        backend = _mock_backend()
        _sink(backend=backend).write(_artifact())
        backend.upload.assert_called_once()


# ---------------------------------------------------------------------------
# MinioSink via DatasetPusherPlugin (integration)
# ---------------------------------------------------------------------------


class TestMinioSinkViaDatasetPusherPlugin(TestCase):
    """Tests for DatasetPusherPlugin dispatching to MinioSink."""

    def test_plugin_dispatches_to_minio_sink(self):
        """DatasetPusherPlugin.execute() calls upload() and returns the s3:// URI."""
        from michelangelo.workflow.schema.pusher import DatasetPluginConfig
        from michelangelo.workflow.tasks.pusher.plugins.dataset_plugin import (
            DatasetPusherPlugin,
        )

        backend = _mock_backend("s3://my-bucket/datasets/v1/data.parquet")
        sink = MinioSink(
            MinioSinkConfig("datasets/v1", format=DatasetFormat.PARQUET),
            storage_backend=backend,
        )
        plugin = DatasetPusherPlugin(
            config=DatasetPluginConfig(sinks=[sink]),
            artifact=_artifact(),
        )
        result = plugin.execute()

        backend.upload.assert_called_once()
        self.assertEqual(result["destination_path"], "s3://my-bucket/datasets/v1/data.parquet")
        self.assertEqual(result["num_records"], len(_RECORDS))

    def test_plugin_local_and_minio_sinks_together(self):
        """Plugin dispatches to both LocalFileSink and MinioSink in sequence."""
        from michelangelo.workflow.schema.pusher import DatasetPluginConfig
        from michelangelo.workflow.schema.sinks import LocalFileSinkConfig
        from michelangelo.workflow.tasks.functions.sinks import LocalFileSink
        from michelangelo.workflow.tasks.pusher.plugins.dataset_plugin import (
            DatasetPusherPlugin,
        )

        local_dest = tempfile.mkdtemp()
        backend = _mock_backend("s3://my-bucket/datasets/v1/data.parquet")

        plugin = DatasetPusherPlugin(
            config=DatasetPluginConfig(
                sinks=[
                    LocalFileSink(LocalFileSinkConfig(local_dest, format=DatasetFormat.PARQUET)),
                    MinioSink(MinioSinkConfig("datasets/v1"), storage_backend=backend),
                ]
            ),
            artifact=_artifact(),
        )
        result = plugin.execute()

        self.assertEqual(len(result["sinks"]), 2)
        # First sink: local file
        self.assertTrue(result["sinks"][0]["uri"].startswith(local_dest))
        # Second sink: MinIO upload
        self.assertEqual(result["sinks"][1]["uri"], "s3://my-bucket/datasets/v1/data.parquet")
        backend.upload.assert_called_once()
