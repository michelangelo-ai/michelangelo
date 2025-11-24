"""Unit tests for sitecustomize.py

Note: These tests are simplified because sitecustomize.py is designed to run
as a module initialization script with complex dependencies on file system,
environment variables, and remote storage.
"""

import logging
import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch


class TestSitecustomize(unittest.TestCase):
    """Basic unit tests for sitecustomize.py"""

    def test_storage_downloader_is_abstract(self):
        """Test that StorageDownloader cannot be instantiated directly"""
        from michelangelo.uniflow.core.sitecustomize import StorageDownloader

        with self.assertRaises(TypeError):
            # Should fail because it's abstract
            StorageDownloader()

    def test_fsspec_downloader_exists(self):
        """Test that FsspecDownloader class exists and can be instantiated"""
        from michelangelo.uniflow.core.sitecustomize import FsspecDownloader

        # Should not raise
        downloader = FsspecDownloader()
        self.assertIsNotNone(downloader)

    def test_download_and_extract_dev_files_exists(self):
        """Test that download_and_extract_dev_files function exists"""
        from michelangelo.uniflow.core.sitecustomize import (
            download_and_extract_dev_files,
        )

        # Should not raise
        self.assertIsNotNone(download_and_extract_dev_files)
        self.assertTrue(callable(download_and_extract_dev_files))


class TestFsspecDownloader(unittest.TestCase):
    """Unit tests for FsspecDownloader"""

    def setUp(self):
        """Set up test fixtures"""
        from michelangelo.uniflow.core.sitecustomize import FsspecDownloader

        self.downloader = FsspecDownloader()
        self.logger = logging.getLogger("test")

    def test_fsspec_downloader_has_download_method(self):
        """Test that FsspecDownloader has a download method"""
        self.assertTrue(hasattr(self.downloader, "download"))
        self.assertTrue(callable(self.downloader.download))

    @patch("fsspec.open")
    def test_download_success(self, mock_fsspec_open):
        """Test successful download using fsspec"""
        # Mock fsspec.open to return fake tarball data
        fake_data = b"fake tarball content"
        mock_remote_file = MagicMock()
        mock_remote_file.read.return_value = fake_data
        mock_fsspec_open.return_value.__enter__.return_value = mock_remote_file

        # Create a temporary file path
        with tempfile.TemporaryDirectory() as tmp_dir:
            local_path = Path(tmp_dir) / "test.tar.gz"

            # Call download
            result = self.downloader.download(
                "s3://bucket/path/file.tar.gz", local_path, self.logger
            )

            # Verify success
            self.assertTrue(result)
            mock_fsspec_open.assert_called_once_with(
                "s3://bucket/path/file.tar.gz", "rb"
            )

            # Verify file was written
            self.assertTrue(local_path.exists())
            with open(local_path, "rb") as f:
                self.assertEqual(f.read(), fake_data)

    @patch("fsspec.open")
    def test_download_fsspec_error(self, mock_fsspec_open):
        """Test download failure due to fsspec error"""
        # Mock fsspec.open to raise an exception
        mock_fsspec_open.side_effect = Exception("S3 connection failed")

        with tempfile.TemporaryDirectory() as tmp_dir:
            local_path = Path(tmp_dir) / "test.tar.gz"

            # Call download
            result = self.downloader.download(
                "s3://bucket/path/file.tar.gz", local_path, self.logger
            )

            # Verify failure
            self.assertFalse(result)
            # File should not exist
            self.assertFalse(local_path.exists())

    @patch("fsspec.open")
    def test_download_with_different_protocols(self, mock_fsspec_open):
        """Test download with different storage protocols (S3, MinIO, etc.)"""
        fake_data = b"test data"
        mock_remote_file = MagicMock()
        mock_remote_file.read.return_value = fake_data
        mock_fsspec_open.return_value.__enter__.return_value = mock_remote_file

        test_paths = [
            "s3://bucket/file.tar.gz",
            "s3://minio-bucket/path/to/file.tar.gz",
            "hdfs:///path/to/file.tar.gz",
        ]

        with tempfile.TemporaryDirectory() as tmp_dir:
            for remote_path in test_paths:
                mock_fsspec_open.reset_mock()
                local_path = Path(tmp_dir) / f"test_{hash(remote_path)}.tar.gz"

                result = self.downloader.download(remote_path, local_path, self.logger)

                self.assertTrue(result, f"Failed for {remote_path}")
                mock_fsspec_open.assert_called_once_with(remote_path, "rb")

    @patch("fsspec.open")
    def test_download_large_file(self, mock_fsspec_open):
        """Test download of large file (simulated)"""
        # Simulate a 10MB file
        fake_data = b"x" * (10 * 1024 * 1024)  # 10MB
        mock_remote_file = MagicMock()
        mock_remote_file.read.return_value = fake_data
        mock_fsspec_open.return_value.__enter__.return_value = mock_remote_file

        with tempfile.TemporaryDirectory() as tmp_dir:
            local_path = Path(tmp_dir) / "large_file.tar.gz"

            result = self.downloader.download(
                "s3://bucket/large.tar.gz", local_path, self.logger
            )

            self.assertTrue(result)
            self.assertEqual(local_path.stat().st_size, len(fake_data))


if __name__ == "__main__":
    unittest.main()
