"""Tests for S3Sink — upload dispatch and DatasetVariable integration."""

from __future__ import annotations

import os
import tempfile
from unittest import TestCase
from unittest.mock import MagicMock, patch

import pandas as pd

from michelangelo.workflow.schema.pusher import DatasetFormat
from michelangelo.workflow.schema.sinks.s3 import S3SinkConfig
from michelangelo.workflow.tasks.functions.sinks.s3 import S3Sink
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
    backend = MagicMock()
    backend.upload.return_value = uri
    return backend


def _config(
    key: str = "datasets/california/v1",
    fmt: DatasetFormat = DatasetFormat.PARQUET,
    backend: MagicMock | None = None,
) -> S3SinkConfig:
    return S3SinkConfig(
        destination_key=key,
        storage_backend=backend
        or _mock_backend(f"s3://test-bucket/{key}/data.{fmt.value}"),
        format=fmt,
    )


def _sink(
    key: str = "datasets/california/v1",
    fmt: DatasetFormat = DatasetFormat.PARQUET,
    backend: MagicMock | None = None,
) -> S3Sink:
    return S3Sink(_config(key=key, fmt=fmt, backend=backend))


# ---------------------------------------------------------------------------
# S3SinkConfig — validation and defaults
# ---------------------------------------------------------------------------


class TestS3SinkConfig(TestCase):
    """Tests for S3SinkConfig validation and defaults."""

    def test_default_format_is_parquet(self):
        """Format defaults to PARQUET when not specified."""
        cfg = S3SinkConfig("datasets/v1", storage_backend=_mock_backend())
        self.assertEqual(cfg.format, DatasetFormat.PARQUET)

    def test_explicit_format_preserved(self):
        """An explicitly set format is stored correctly."""
        cfg = S3SinkConfig(
            "d/v1", storage_backend=_mock_backend(), format=DatasetFormat.CSV
        )
        self.assertEqual(cfg.format, DatasetFormat.CSV)

    def test_destination_key_stored(self):
        """The destination_key field is stored after normalisation."""
        cfg = S3SinkConfig("my/prefix/v3", storage_backend=_mock_backend())
        self.assertEqual(cfg.destination_key, "my/prefix/v3")

    def test_raises_on_empty_destination_key(self):
        """It raises ValueError when destination_key is empty."""
        with self.assertRaises(ValueError):
            S3SinkConfig("", storage_backend=_mock_backend())

    def test_raises_on_whitespace_destination_key(self):
        """It raises ValueError when destination_key is whitespace only."""
        with self.assertRaises(ValueError):
            S3SinkConfig("   ", storage_backend=_mock_backend())

    def test_raises_on_leading_slash(self):
        """It raises ValueError when destination_key starts with '/'."""
        with self.assertRaises(ValueError):
            S3SinkConfig("/datasets/v1", storage_backend=_mock_backend())

    def test_trailing_slash_is_stripped(self):
        """Trailing slashes are stripped from destination_key."""
        cfg = S3SinkConfig("datasets/v1/", storage_backend=_mock_backend())
        self.assertEqual(cfg.destination_key, "datasets/v1")

    def test_raises_when_storage_backend_is_none(self):
        """It raises ValueError when storage_backend is None."""
        with self.assertRaises(ValueError):
            S3SinkConfig("d/v1", storage_backend=None)

    def test_storage_backend_stored_on_config(self):
        """The storage_backend is accessible on the config."""
        backend = _mock_backend()
        cfg = S3SinkConfig("d/v1", storage_backend=backend)
        self.assertIs(cfg.storage_backend, backend)


# ---------------------------------------------------------------------------
# S3Sink constructor — single-arg pattern
# ---------------------------------------------------------------------------


class TestS3SinkConstructor(TestCase):
    """Tests that S3Sink follows the single-argument constructor pattern."""

    def test_accepts_single_config_argument(self):
        """S3Sink(config) constructs without error."""
        sink = S3Sink(_config())
        self.assertIsNotNone(sink)

    def test_backend_accessible_from_config(self):
        """S3Sink exposes the backend via _backend (used internally)."""
        backend = _mock_backend()
        sink = S3Sink(_config(backend=backend))
        self.assertIs(sink._backend, backend)


# ---------------------------------------------------------------------------
# S3Sink.write() — object key construction
# ---------------------------------------------------------------------------


class TestS3SinkObjectKey(TestCase):
    """Tests that S3Sink uploads to the correct object key."""

    def test_parquet_key_has_data_parquet_suffix(self):
        """Parquet format appends /data.parquet to the destination_key."""
        backend = _mock_backend()
        S3Sink(_config("datasets/v1", DatasetFormat.PARQUET, backend)).write(
            _artifact()
        )
        _, call_key = backend.upload.call_args[0]
        self.assertEqual(call_key, "datasets/v1/data.parquet")

    def test_csv_key_has_data_csv_suffix(self):
        """CSV format appends /data.csv to the destination_key."""
        backend = _mock_backend()
        S3Sink(_config("datasets/v1", DatasetFormat.CSV, backend)).write(_artifact())
        _, call_key = backend.upload.call_args[0]
        self.assertEqual(call_key, "datasets/v1/data.csv")

    def test_json_key_has_data_json_suffix(self):
        """JSON format appends /data.json to the destination_key."""
        backend = _mock_backend()
        S3Sink(_config("datasets/v1", DatasetFormat.JSON, backend)).write(_artifact())
        _, call_key = backend.upload.call_args[0]
        self.assertEqual(call_key, "datasets/v1/data.json")


# ---------------------------------------------------------------------------
# S3Sink.write() — SinkResult
# ---------------------------------------------------------------------------


class TestS3SinkResult(TestCase):
    """Tests for the SinkResult returned by S3Sink.write()."""

    def test_uri_is_backend_upload_return_value(self):
        """The returned URI is whatever the backend's upload() returns."""
        expected = "s3://my-bucket/datasets/v1/data.parquet"
        result = _sink(backend=_mock_backend(expected)).write(_artifact())
        self.assertEqual(result.uri, expected)

    def test_num_records_matches_dataframe_length(self):
        """num_records equals the number of rows in the artifact."""
        self.assertEqual(_sink().write(_artifact()).num_records, len(_RECORDS))

    def test_num_records_zero_for_empty_dataframe(self):
        """num_records is 0 for an empty DataFrame."""
        result = _sink().write(DatasetVariable(value=pd.DataFrame()))
        self.assertEqual(result.num_records, 0)


# ---------------------------------------------------------------------------
# S3Sink.write() — serialisation and temp file cleanup
# ---------------------------------------------------------------------------


class TestS3SinkSerialisation(TestCase):
    """Tests that S3Sink serialises the DataFrame correctly and cleans up."""

    def _capture_upload_path(self) -> tuple[MagicMock, list[str]]:
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
        with patch("michelangelo.workflow.tasks.functions.sinks.s3.os.unlink"):
            S3Sink(_config("d/v1", DatasetFormat.PARQUET, backend)).write(_artifact())
        df_out = pd.read_parquet(paths[0])
        self.assertEqual(len(df_out), len(_RECORDS))
        os.unlink(paths[0])

    def test_csv_temp_file_is_readable(self):
        """The temp file passed to upload() is a valid CSV file."""
        backend, paths = self._capture_upload_path()
        with patch("michelangelo.workflow.tasks.functions.sinks.s3.os.unlink"):
            S3Sink(_config("d/v1", DatasetFormat.CSV, backend)).write(_artifact())
        df_out = pd.read_csv(paths[0])
        self.assertEqual(len(df_out), len(_RECORDS))
        os.unlink(paths[0])

    def test_temp_file_cleaned_up_on_success(self):
        """The temporary file is deleted after a successful upload."""
        deleted: list[str] = []
        backend = MagicMock()
        backend.upload.return_value = "s3://b/k"
        original_unlink = os.unlink

        def _unlink(path: str) -> None:
            deleted.append(path)
            original_unlink(path)

        _patch = "michelangelo.workflow.tasks.functions.sinks.s3.os.unlink"
        with patch(_patch, side_effect=_unlink):
            S3Sink(_config(backend=backend)).write(_artifact())

        self.assertEqual(len(deleted), 1)
        self.assertFalse(os.path.exists(deleted[0]))

    def test_temp_file_cleaned_up_on_upload_failure(self):
        """The temporary file is deleted even when the upload raises."""
        deleted: list[str] = []
        backend = MagicMock()
        backend.upload.side_effect = OSError("network error")
        original_unlink = os.unlink

        def _unlink(path: str) -> None:
            deleted.append(path)
            original_unlink(path)

        _patch = "michelangelo.workflow.tasks.functions.sinks.s3.os.unlink"
        with patch(_patch, side_effect=_unlink), self.assertRaises(OSError):
            S3Sink(_config(backend=backend)).write(_artifact())

        self.assertEqual(len(deleted), 1)
        self.assertFalse(os.path.exists(deleted[0]))


# ---------------------------------------------------------------------------
# S3Sink.write() — error cases
# ---------------------------------------------------------------------------


class TestS3SinkErrors(TestCase):
    """Tests for S3Sink error handling."""

    def test_raises_type_error_for_non_pandas_artifact(self):
        """It raises TypeError when artifact.value is not a pandas DataFrame."""
        with self.assertRaisesRegex(TypeError, "pandas.DataFrame"):
            _sink().write(DatasetVariable(value={"not": "a dataframe"}))

    def test_raises_value_error_for_unsupported_format(self):
        """It raises ValueError for an unrecognised DatasetFormat."""
        bad_fmt = MagicMock()
        bad_fmt.value = "xyz"
        cfg = S3SinkConfig("d/v1", storage_backend=_mock_backend(), format=bad_fmt)
        with self.assertRaises(ValueError):
            S3Sink(cfg).write(_artifact())

    def test_propagates_oserror_from_backend(self):
        """It propagates OSError raised by the storage backend."""
        backend = _mock_backend()
        backend.upload.side_effect = OSError("connection refused")
        with self.assertRaises(OSError):
            _sink(backend=backend).write(_artifact())

    def test_upload_called_once_per_write(self):
        """upload() is called exactly once per write() invocation."""
        backend = _mock_backend()
        _sink(backend=backend).write(_artifact())
        backend.upload.assert_called_once()


# ---------------------------------------------------------------------------
# S3Sink via DatasetPusherPlugin (integration)
# ---------------------------------------------------------------------------


class TestS3SinkViaDatasetPusherPlugin(TestCase):
    """Tests for DatasetPusherPlugin dispatching to S3Sink."""

    def test_plugin_dispatches_to_s3_sink(self):
        """DatasetPusherPlugin dispatches to S3Sink and returns the s3:// URI."""
        from michelangelo.workflow.schema.pusher import DatasetPluginConfig
        from michelangelo.workflow.tasks.pusher.plugins.dataset_plugin import (
            DatasetPusherPlugin,
        )

        expected = "s3://my-bucket/datasets/v1/data.parquet"
        backend = _mock_backend(expected)
        plugin = DatasetPusherPlugin(
            config=DatasetPluginConfig(sinks=[S3Sink(_config(backend=backend))]),
            artifact=_artifact(),
        )
        result = plugin.execute()

        backend.upload.assert_called_once()
        self.assertEqual(result["destination_path"], expected)
        self.assertEqual(result["num_records"], len(_RECORDS))

    def test_plugin_local_and_s3_sinks_together(self):
        """Plugin dispatches to both LocalFileSink and S3Sink in sequence."""
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
                    LocalFileSink(
                        LocalFileSinkConfig(local_dest, format=DatasetFormat.PARQUET)
                    ),
                    S3Sink(_config(backend=backend)),
                ]
            ),
            artifact=_artifact(),
        )
        result = plugin.execute()

        self.assertEqual(len(result["sinks"]), 2)
        self.assertTrue(result["sinks"][0]["uri"].startswith(local_dest))
        self.assertEqual(
            result["sinks"][1]["uri"], "s3://my-bucket/datasets/v1/data.parquet"
        )
        backend.upload.assert_called_once()


# ---------------------------------------------------------------------------
# get_storage_location() — StorageBackend ABC and implementations
# ---------------------------------------------------------------------------


class TestGetStorageLocation(TestCase):
    """Tests for get_storage_location() on StorageBackend implementations."""

    def test_local_storage_backend_returns_base_dir(self):
        """LocalStorageBackend.get_storage_location() returns the base directory."""
        from michelangelo.lib.artifact_manager.storage_backend import (
            LocalStorageBackend,
        )

        backend = LocalStorageBackend(tempfile.mkdtemp())
        self.assertEqual(backend.get_storage_location(), backend._base_dir)

    def test_minio_storage_backend_returns_s3_bucket_uri(self):
        """MinioStorageBackend.get_storage_location() returns s3://{bucket}."""
        import sys
        from unittest.mock import MagicMock, patch

        mock_minio = MagicMock()
        mock_minio.error.S3Error = Exception

        with patch.dict(
            sys.modules, {"minio": mock_minio, "minio.error": mock_minio.error}
        ):
            from michelangelo.lib.artifact_manager.minio_backend import (
                MinioStorageBackend,
            )

            b = object.__new__(MinioStorageBackend)
            b._bucket = "my-bucket"
            b._client = MagicMock()
            b._S3Error = Exception
            self.assertEqual(b.get_storage_location(), "s3://my-bucket")

    def test_base_storage_backend_returns_none_by_default(self):
        """The default ABC implementation returns None."""
        from michelangelo.lib.artifact_manager.storage_backend import StorageBackend

        class _Minimal(StorageBackend):
            def upload(self, local_path, destination_key):
                return ""

            def download(self, uri, local_path):
                pass

        self.assertIsNone(_Minimal().get_storage_location())
