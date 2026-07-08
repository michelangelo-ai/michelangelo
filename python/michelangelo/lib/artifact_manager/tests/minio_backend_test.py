"""Tests for MinioStorageBackend."""

from __future__ import annotations

import os
import shutil
import sys
import tarfile
import tempfile
from unittest import TestCase
from unittest.mock import MagicMock, patch

from michelangelo.lib.exceptions import ConfigurationError


class _FakeS3Error(Exception):
    """Stand-in for minio.error.S3Error in unit tests."""

    def __init__(self, msg: str = "", code: str = "") -> None:
        super().__init__(msg)
        self.code = code


def _make_mock_minio(bucket_exists: bool = True) -> tuple[MagicMock, MagicMock]:
    """Return (mock_module, mock_client_instance)."""
    mock_module = MagicMock()
    mock_client = MagicMock()
    mock_client.bucket_exists.return_value = bucket_exists
    mock_module.Minio.return_value = mock_client
    mock_module.error.S3Error = _FakeS3Error
    return mock_module, mock_client


_DEFAULT_KWARGS = {
    "endpoint": "localhost:9000",
    "bucket": "test-bucket",
    "access_key": "minioadmin",
    "secret_key": "minioadmin",
    "secure": False,
}


class TestMinioStorageBackendValidation(TestCase):
    """Tests for MinioStorageBackend constructor validation."""

    def test_raises_on_empty_endpoint(self):
        """It raises ConfigurationError when endpoint is empty."""
        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend

        with self.assertRaises(ConfigurationError):
            MinioStorageBackend(endpoint="", bucket="b", access_key="a", secret_key="s")

    def test_raises_on_empty_bucket(self):
        """It raises ConfigurationError when the bucket is empty."""
        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend

        with self.assertRaises(ConfigurationError):
            MinioStorageBackend(
                endpoint="localhost:9000", bucket="", access_key="a", secret_key="s"
            )

    def test_defaults_secure_true(self):
        """Secure defaults to True (matches MinIO SDK default)."""
        mock_module, _ = _make_mock_minio()
        with patch.dict(
            sys.modules, {"minio": mock_module, "minio.error": mock_module.error}
        ):
            from michelangelo.lib.artifact_manager.minio_backend import (
                MinioStorageBackend,
            )

            MinioStorageBackend(
                endpoint="localhost:9000", bucket="b", access_key="a", secret_key="s"
            )
        call_kwargs = mock_module.Minio.call_args[1]
        self.assertTrue(call_kwargs["secure"])

    def test_defaults_create_bucket_false(self):
        """create_bucket_if_missing defaults to False — bucket_exists not called."""
        mock_module, mock_client = _make_mock_minio()
        with patch.dict(
            sys.modules, {"minio": mock_module, "minio.error": mock_module.error}
        ):
            from michelangelo.lib.artifact_manager.minio_backend import (
                MinioStorageBackend,
            )

            MinioStorageBackend(**_DEFAULT_KWARGS)
        mock_client.bucket_exists.assert_not_called()


class TestMinioStorageBackendInit(TestCase):
    """Tests for MinioStorageBackend.__init__() bucket management."""

    def test_raises_import_error_when_minio_missing(self):
        """It raises ImportError with an install hint when minio is absent."""
        with patch.dict(sys.modules, {"minio": None}):
            from michelangelo.lib.artifact_manager.minio_backend import (
                MinioStorageBackend,
            )

            with self.assertRaises(ImportError) as ctx:
                MinioStorageBackend(**_DEFAULT_KWARGS)
        self.assertIn("pip install", str(ctx.exception))

    def test_no_ensure_bucket_by_default(self):
        """It does not check or create the bucket when create_bucket_if_missing=False.

        Neither bucket_exists nor make_bucket should be called.
        """
        mock_module, mock_client = _make_mock_minio()
        with patch.dict(
            sys.modules, {"minio": mock_module, "minio.error": mock_module.error}
        ):
            from michelangelo.lib.artifact_manager.minio_backend import (
                MinioStorageBackend,
            )

            MinioStorageBackend(**_DEFAULT_KWARGS)
        mock_client.bucket_exists.assert_not_called()
        mock_client.make_bucket.assert_not_called()

    def test_ensure_bucket_skips_make_when_exists(self):
        """With create_bucket_if_missing=True it skips make_bucket when bucket exists.

        make_bucket should not be called if the bucket already exists.
        """
        mock_module, mock_client = _make_mock_minio(bucket_exists=True)
        with patch.dict(
            sys.modules, {"minio": mock_module, "minio.error": mock_module.error}
        ):
            from michelangelo.lib.artifact_manager.minio_backend import (
                MinioStorageBackend,
            )

            MinioStorageBackend(**_DEFAULT_KWARGS, create_bucket_if_missing=True)
        mock_client.make_bucket.assert_not_called()

    def test_ensure_bucket_calls_make_when_absent(self):
        """With create_bucket_if_missing=True it calls make_bucket when absent."""
        mock_module, mock_client = _make_mock_minio(bucket_exists=False)
        with patch.dict(
            sys.modules, {"minio": mock_module, "minio.error": mock_module.error}
        ):
            from michelangelo.lib.artifact_manager.minio_backend import (
                MinioStorageBackend,
            )

            MinioStorageBackend(**_DEFAULT_KWARGS, create_bucket_if_missing=True)
        mock_client.make_bucket.assert_called_once_with("test-bucket")

    def test_ensure_bucket_handles_concurrent_creation(self):
        """With create_bucket_if_missing=True it tolerates BucketAlreadyOwnedByYou."""
        mock_module, mock_client = _make_mock_minio(bucket_exists=False)
        mock_client.make_bucket.side_effect = _FakeS3Error(
            "already owned", code="BucketAlreadyOwnedByYou"
        )
        with patch.dict(
            sys.modules, {"minio": mock_module, "minio.error": mock_module.error}
        ):
            from michelangelo.lib.artifact_manager.minio_backend import (
                MinioStorageBackend,
            )

            MinioStorageBackend(**_DEFAULT_KWARGS, create_bucket_if_missing=True)


class TestMinioStorageBackendUpload(TestCase):
    """Tests for MinioStorageBackend.upload()."""

    def setUp(self) -> None:
        """Set up test fixtures."""
        self._tmp_dirs: list[str] = []

    def tearDown(self) -> None:
        """Clean up test fixtures."""
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
        b._bucket = "test-bucket"
        b._client = mock_client
        b._S3Error = _FakeS3Error
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

    def test_upload_directory_returns_dir_tar_uri(self):
        """It returns a URI with /__dir__.tar suffix for directory uploads."""
        mock_client = MagicMock()
        backend = self._backend(mock_client)
        local_dir = self._make_tmp_dir()
        uri = backend.upload(local_dir, "models/clf/abc/raw")
        self.assertEqual(uri, "s3://test-bucket/models/clf/abc/raw/__dir__.tar")

    def test_upload_directory_stores_at_dir_tar_key(self):
        """fput_object is called with the /__dir__.tar key for directories."""
        mock_client = MagicMock()
        backend = self._backend(mock_client)
        backend.upload(self._make_tmp_dir(), "models/clf/abc/raw")
        call_args = mock_client.fput_object.call_args[0]
        self.assertEqual(call_args[0], "test-bucket")
        self.assertEqual(call_args[1], "models/clf/abc/raw/__dir__.tar")

    def test_upload_raises_on_empty_key(self):
        """It raises ValueError when destination_key is empty."""
        mock_client = MagicMock()
        backend = self._backend(mock_client)
        with self.assertRaises(ValueError):
            backend.upload(self._make_tmp_file(), "")

    def test_upload_wraps_s3error_as_oserror(self):
        """It converts S3Error from fput_object into OSError."""
        mock_client = MagicMock()
        mock_client.fput_object.side_effect = _FakeS3Error("access denied")
        backend = self._backend(mock_client)
        with self.assertRaises(OSError):
            backend.upload(self._make_tmp_file(), "models/clf/raw")


class TestMinioStorageBackendDownload(TestCase):
    """Tests for MinioStorageBackend.download()."""

    def setUp(self) -> None:
        """Set up test fixtures."""
        self._tmp_dirs: list[str] = []

    def tearDown(self) -> None:
        """Clean up test fixtures."""
        for d in self._tmp_dirs:
            if os.path.exists(d):
                shutil.rmtree(d)

    def _backend(self, mock_client):
        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend

        b = object.__new__(MinioStorageBackend)
        b._bucket = "test-bucket"
        b._client = mock_client
        b._S3Error = _FakeS3Error
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

    def test_download_directory_extracts_tar_on_dir_tar_uri(self):
        """It extracts a tar archive when the URI ends with /__dir__.tar."""
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
        backend.download("s3://test-bucket/models/clf/raw/__dir__.tar", dest)
        self.assertTrue(os.path.exists(os.path.join(dest, "weights.bin")))

    def test_download_plain_tar_not_extracted(self):
        """A plain .tar file is NOT extracted when URI lacks /__dir__.tar suffix."""
        src_dir = tempfile.mkdtemp()
        self._tmp_dirs.append(src_dir)
        tar_path = os.path.join(src_dir, "data.tar")
        with tarfile.open(tar_path, "w"):
            pass

        def fake_fget(bucket, key, local):
            shutil.copy2(tar_path, local)

        mock_client = MagicMock()
        mock_client.fget_object.side_effect = fake_fget
        backend = self._backend(mock_client)

        dest_dir = tempfile.mkdtemp()
        self._tmp_dirs.append(dest_dir)
        dest = os.path.join(dest_dir, "out.tar")
        backend.download("s3://test-bucket/models/clf/raw", dest)
        self.assertTrue(os.path.isfile(dest))

    def test_download_raises_on_non_s3_uri(self):
        """It raises ValueError for a URI that is not s3://."""
        mock_client = MagicMock()
        backend = self._backend(mock_client)
        with self.assertRaises(ValueError):
            backend.download("gs://bucket/key", "/tmp/out")

    def test_download_raises_on_uri_with_no_key(self):
        """It raises ValueError when the URI has a bucket but no object key."""
        mock_client = MagicMock()
        backend = self._backend(mock_client)
        with self.assertRaises(ValueError):
            backend.download("s3://bucket", "/tmp/out")

    def test_download_wraps_s3error_as_oserror(self):
        """It converts S3Error from fget_object into OSError."""
        mock_client = MagicMock()
        mock_client.fget_object.side_effect = _FakeS3Error("no such key")
        backend = self._backend(mock_client)
        with self.assertRaises(OSError):
            backend.download("s3://test-bucket/models/clf/raw", "/tmp/out")
