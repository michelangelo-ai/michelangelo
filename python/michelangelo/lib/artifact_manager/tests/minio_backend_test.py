"""Tests for MinioStorageBackend."""

from __future__ import annotations

import os
import shutil
import sys
import tarfile
import tempfile
from unittest import TestCase
from unittest.mock import MagicMock, patch

from michelangelo.lib.artifact_manager.schema.minio import MinioStorageConfig
from michelangelo.lib.exceptions import ConfigurationError


def _config(**kwargs) -> MinioStorageConfig:
    defaults = {
        "endpoint": "localhost:9000",
        "bucket": "test-bucket",
        "access_key": "minioadmin",
        "secret_key": "minioadmin",
    }
    defaults.update(kwargs)
    return MinioStorageConfig(**defaults)


def _make_mock_minio() -> tuple[MagicMock, MagicMock]:
    """Return (mock_module, mock_client_instance)."""
    mock_module = MagicMock()
    mock_client = MagicMock()
    mock_client.bucket_exists.return_value = True
    mock_module.Minio.return_value = mock_client
    return mock_module, mock_client


class TestMinioStorageConfig(TestCase):
    """Tests for MinioStorageConfig validation."""

    def test_raises_on_empty_endpoint(self):
        """It raises ConfigurationError when endpoint is empty."""
        with self.assertRaises(ConfigurationError):
            _config(endpoint="")

    def test_raises_on_empty_bucket(self):
        """It raises ConfigurationError when bucket is empty."""
        with self.assertRaises(ConfigurationError):
            _config(bucket="")

    def test_defaults(self):
        """It defaults to insecure=False and no region."""
        cfg = _config()
        self.assertFalse(cfg.secure)
        self.assertIsNone(cfg.region)


class TestMinioStorageBackendInit(TestCase):
    """Tests for MinioStorageBackend.__init__()."""

    def test_raises_import_error_when_minio_missing(self):
        """It raises ImportError with an install hint when minio is absent."""
        with patch.dict(sys.modules, {"minio": None}):
            from michelangelo.lib.artifact_manager.minio_backend import (
                MinioStorageBackend,
            )
            with self.assertRaises(ImportError) as ctx:
                MinioStorageBackend(_config())
        self.assertIn("pip install", str(ctx.exception))

    def test_ensure_bucket_skips_make_when_exists(self):
        """It does not call make_bucket when bucket_exists returns True."""
        mock_module, mock_client = _make_mock_minio()
        mock_client.bucket_exists.return_value = True
        with patch.dict(sys.modules, {"minio": mock_module}):
            from michelangelo.lib.artifact_manager.minio_backend import (
                MinioStorageBackend,
            )
            MinioStorageBackend(_config())
        mock_client.make_bucket.assert_not_called()

    def test_ensure_bucket_calls_make_when_absent(self):
        """It calls make_bucket when the bucket does not exist."""
        mock_module, mock_client = _make_mock_minio()
        mock_client.bucket_exists.return_value = False
        with patch.dict(sys.modules, {"minio": mock_module}):
            from michelangelo.lib.artifact_manager.minio_backend import (
                MinioStorageBackend,
            )
            MinioStorageBackend(_config())
        mock_client.make_bucket.assert_called_once_with("test-bucket")


class TestMinioStorageBackendUpload(TestCase):
    """Tests for MinioStorageBackend.upload()."""

    def setUp(self) -> None:
        """Initialise temp-dir tracking list."""
        self._tmp_dirs: list[str] = []

    def tearDown(self) -> None:
        """Remove temp dirs created during tests."""
        for d in self._tmp_dirs:
            if os.path.exists(d):
                shutil.rmtree(d)

    def _make_tmp_file(self, content: str = "data") -> str:
        d = tempfile.mkdtemp()
        self._tmp_dirs.append(d)
        p = os.path.join(d, "model.pt")
        with open(p, "w") as f:
            f.write(content)
        return p

    def _make_tmp_dir(self) -> str:
        d = tempfile.mkdtemp()
        self._tmp_dirs.append(d)
        with open(os.path.join(d, "weights.bin"), "w") as f:
            f.write("weights")
        return d

    def _backend(self, mock_client):
        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend
        b = object.__new__(MinioStorageBackend)
        b._config = _config()
        b._client = mock_client
        return b

    def test_upload_file_returns_s3_uri(self):
        """It returns an s3:// URI matching the bucket and key."""
        mock_client = MagicMock()
        backend = self._backend(mock_client)
        uri = backend.upload(self._make_tmp_file(), "models/clf/abc/raw")
        self.assertEqual(uri, "s3://test-bucket/models/clf/abc/raw")

    def test_upload_file_calls_fput_object(self):
        """It calls fput_object with the correct bucket and key."""
        mock_client = MagicMock()
        backend = self._backend(mock_client)
        local = self._make_tmp_file()
        backend.upload(local, "models/clf/abc/raw")
        mock_client.fput_object.assert_called_once_with(
            "test-bucket", "models/clf/abc/raw", local
        )

    def test_upload_directory_creates_tar_and_uploads(self):
        """It tars a directory and uploads a single object."""
        mock_client = MagicMock()
        backend = self._backend(mock_client)
        local_dir = self._make_tmp_dir()
        uri = backend.upload(local_dir, "models/clf/abc/raw")
        self.assertEqual(uri, "s3://test-bucket/models/clf/abc/raw")
        # fput_object called once with a temp .tar path (not the original dir)
        call_args = mock_client.fput_object.call_args
        uploaded_path = call_args[0][2]
        is_tar = uploaded_path.endswith(".tar")
        self.assertTrue(is_tar or not os.path.isdir(uploaded_path))

    def test_upload_raises_on_empty_key(self):
        """It raises ValueError when destination_key is empty."""
        mock_client = MagicMock()
        backend = self._backend(mock_client)
        with self.assertRaises(ValueError):
            backend.upload(self._make_tmp_file(), "")


class TestMinioStorageBackendDownload(TestCase):
    """Tests for MinioStorageBackend.download()."""

    def setUp(self) -> None:
        """Initialise temp-dir tracking list."""
        self._tmp_dirs: list[str] = []

    def tearDown(self) -> None:
        """Remove temp dirs created during tests."""
        for d in self._tmp_dirs:
            if os.path.exists(d):
                shutil.rmtree(d)

    def _backend(self, mock_client):
        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend
        b = object.__new__(MinioStorageBackend)
        b._config = _config()
        b._client = mock_client
        return b

    def _write_tmp_file(self, content: bytes = b"filedata") -> str:
        d = tempfile.mkdtemp()
        self._tmp_dirs.append(d)
        p = os.path.join(d, "payload")
        with open(p, "wb") as f:
            f.write(content)
        return p

    def test_download_file_copies_to_local_path(self):
        """It copies the downloaded object to the local path."""
        src = self._write_tmp_file(b"model-weights")

        def fake_fget(bucket, key, local):
            shutil.copy2(src, local)

        mock_client = MagicMock()
        mock_client.fget_object.side_effect = fake_fget
        backend = self._backend(mock_client)

        dest_dir = tempfile.mkdtemp()
        self._tmp_dirs.append(dest_dir)
        dest = os.path.join(dest_dir, "out.pt")
        backend.download("s3://test-bucket/models/clf/raw", dest)
        with open(dest, "rb") as f:
            self.assertEqual(f.read(), b"model-weights")

    def test_download_directory_extracts_tar(self):
        """It extracts a tar archive into the destination directory."""
        src_dir = tempfile.mkdtemp()
        self._tmp_dirs.append(src_dir)
        with open(os.path.join(src_dir, "weights.bin"), "w") as f:
            f.write("weights")

        tar_dir = tempfile.mkdtemp()
        self._tmp_dirs.append(tar_dir)
        tar_path = os.path.join(tar_dir, "archive.tar")
        with tarfile.open(tar_path, "w") as tar:
            tar.add(src_dir, arcname="")

        def fake_fget(bucket, key, local):
            shutil.copy2(tar_path, local)

        mock_client = MagicMock()
        mock_client.fget_object.side_effect = fake_fget
        backend = self._backend(mock_client)

        dest = tempfile.mkdtemp()
        self._tmp_dirs.append(dest)
        backend.download("s3://test-bucket/models/clf/raw", dest)
        self.assertTrue(os.path.exists(os.path.join(dest, "weights.bin")))

    def test_download_raises_on_non_s3_uri(self):
        """It raises ValueError for a URI that is not s3://."""
        mock_client = MagicMock()
        backend = self._backend(mock_client)
        with self.assertRaises(ValueError):
            backend.download("gs://bucket/key", "/tmp/out")
