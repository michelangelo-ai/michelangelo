"""Unit tests for file synchronization functionality."""

import io
import os
import tarfile
import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock, mock_open, patch

from michelangelo.uniflow.core.file_sync import (
    DefaultFileSync,
    FileSync,
)


class _TestableFileBuilder(FileSync):
    """Concrete implementation for testing abstract base class."""

    def get_git_sha(self):
        return "0241feca9a6a681c917c3bb712dcb62918522aed"

    def upload_tarball(self, local_path: str, remote_path: str):
        pass


class TestFileSync(unittest.TestCase):
    """Unit tests for FileSync abstract base class."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = _TestableFileBuilder()
        os.environ["UF_BASE_PROJECTS_PATH"] = (
            "/prod/michelangelo/uniflow/uniflow_dev_run/projects"
        )

    def test_get_random_file_name(self):
        """Test that get_random_file_name generates a valid file name."""
        file_name = self.builder.get_random_file_name()
        self.assertIsNotNone(file_name)
        self.assertTrue(file_name.startswith("file-sync-"))
        self.assertTrue(file_name.endswith(".tar.gz"))

    def test_get_random_file_name_with_none_pipeline(self):
        """Test that filename always uses 'file-sync' prefix."""
        builder = _TestableFileBuilder()
        file_name = builder.get_random_file_name()
        self.assertIn("file-sync", file_name)
        self.assertTrue(file_name.endswith(".tar.gz"))

    def test_get_file_name(self):
        """Test that get_file_name returns a file name with correct prefix."""
        file_name = self.builder.get_file_name()
        self.assertTrue(file_name.startswith("file-sync"))
        self.assertTrue(file_name.endswith(".tar.gz"))

    def test_get_remote_file_path(self):
        """Test that get_remote_file_path returns the correct remote path."""
        os.environ["UF_FILE_SYNC_STORAGE_URL"] = "s3://test-bucket/file-sync"
        remote_file_path = self.builder.get_remote_file_path()
        self.assertTrue(remote_file_path.startswith("s3://test-bucket/file-sync/"))
        self.assertTrue(remote_file_path.endswith(".tar.gz"))

    def test_get_remote_file_path_with_fallback(self):
        """Test fallback to default when env var is not set."""
        if "UF_FILE_SYNC_STORAGE_URL" in os.environ:
            del os.environ["UF_FILE_SYNC_STORAGE_URL"]

        # Create a new builder without the env var set
        builder = _TestableFileBuilder()
        remote_file_path = builder.get_remote_file_path()
        # Should use the default path from file_sync.py
        self.assertTrue(remote_file_path.startswith("s3://default/uniflow/"))
        self.assertTrue(remote_file_path.endswith(".tar.gz"))

    def test_create_diff_tarball_bytes_strips_python_prefix(self):
        """Test that python/ prefix is stripped from arcname in tarball."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp_dir_path = Path(tmp_dir)
            git_dir = tmp_dir_path / ".git"
            git_dir.mkdir()

            python_dir = tmp_dir_path / "python"
            python_dir.mkdir()
            test_file = python_dir / "test.py"
            test_file.write_text("print('test')")

            with (
                patch("subprocess.run") as mock_run,
                patch("os.getcwd", return_value=str(tmp_dir_path)),
            ):
                # Mock git rev-parse to return repo root
                def run_side_effect(*args, **kwargs):
                    cmd = args[0] if args else kwargs.get("args", [])
                    if "rev-parse --show-toplevel" in " ".join(cmd):
                        return MagicMock(stdout=str(tmp_dir_path), returncode=0)
                    elif "diff --name-only" in " ".join(cmd):
                        return MagicMock(stdout="python/test.py", returncode=0)
                    elif "ls-files --others" in " ".join(cmd):
                        return MagicMock(stdout="", returncode=0)
                    return MagicMock(stdout="", returncode=0)

                mock_run.side_effect = run_side_effect

                self.builder.get_git_sha = MagicMock(return_value="abc123")
                tarball_bytes = self.builder.create_diff_tarball_bytes()

                # Verify tarball contains file without python/ prefix
                if tarball_bytes:
                    tar_io = io.BytesIO(tarball_bytes)
                    with tarfile.open(fileobj=tar_io, mode="r:gz") as tar:
                        names = tar.getnames()
                        # Should have "test.py" not "python/test.py"
                        self.assertTrue(any("test.py" in name for name in names))

    def test_create_and_upload_tarball_success(self):
        """Test successful tarball creation and upload."""
        with (
            patch(
                "michelangelo.uniflow.core.file_sync.FileSync.create_diff_tarball_bytes",
                return_value=b"fake-bytes",
            ) as mock_tarball,
            patch(
                "michelangelo.uniflow.core.file_sync.FileSync.get_file_name",
                return_value="fake.tar.gz",
            ) as mock_filename,
            patch(
                "michelangelo.uniflow.core.file_sync.FileSync.get_remote_file_path",
                return_value="/remote/path/fake.tar.gz",
            ) as mock_path,
        ):
            result = self.builder.create_and_upload_tarball()
            self.assertEqual(result, "/remote/path/fake.tar.gz")
            mock_tarball.assert_called_once()
            mock_filename.assert_called_once()
            mock_path.assert_called()
            self.assertEqual(mock_path.call_count, 3)

    def test_create_and_upload_tarball_no_tarball(self):
        """Test that no tarball is created when there are no changed files."""
        with (
            patch(
                "michelangelo.uniflow.core.file_sync.FileSync.create_diff_tarball_bytes",
                return_value=None,
            ) as mock_tarball,
            patch(
                "michelangelo.uniflow.core.file_sync.FileSync.upload_tarball"
            ) as mock_upload,
        ):
            result = self.builder.create_and_upload_tarball()
            self.assertEqual(result, "")
            mock_tarball.assert_called_once()
            mock_upload.assert_not_called()

    def test_get_git_sha(self):
        """Test that get_git_sha returns the correct Git SHA."""
        self.builder.get_git_sha = MagicMock(
            return_value="0241feca9a6a681c917c3bb712dcb62918522aed"
        )
        git_sha = self.builder.get_git_sha()
        self.assertEqual(git_sha, "0241feca9a6a681c917c3bb712dcb62918522aed")


class TestDefaultFileSync(unittest.TestCase):
    """Unit tests for DefaultFileSync."""

    def setUp(self):
        """Set up test fixtures."""
        self.builder = DefaultFileSync(
            docker_image="examples:latest",
        )

    def test_init(self):
        """Test initialization."""
        self.assertEqual(self.builder._docker_image, "examples:latest")

    def test_get_git_sha_from_label_git_commit_actual(self):
        """Test getting Git SHA from git.commit label (actual implementation)."""
        # Patch docker inside the method where it's imported
        with patch("docker.from_env") as mock_from_env:
            mock_client = MagicMock()
            mock_image = MagicMock()
            mock_image.labels = {"git.commit": "abc123def456"}
            mock_client.images.get.return_value = mock_image
            mock_from_env.return_value = mock_client

            git_sha = self.builder.get_git_sha()

            self.assertEqual(git_sha, "abc123def456")
            mock_client.images.get.assert_called_once_with("examples:latest")

    def test_get_git_sha_from_label_git_sha(self):
        """Test getting Git SHA from git.sha label."""
        with patch("docker.from_env") as mock_from_env:
            mock_client = MagicMock()
            mock_image = MagicMock()
            mock_image.labels = {"git.sha": "xyz789abc123"}
            mock_client.images.get.return_value = mock_image
            mock_from_env.return_value = mock_client

            git_sha = self.builder.get_git_sha()

            self.assertEqual(git_sha, "xyz789abc123")

    def test_get_git_sha_from_env_var(self):
        """Test getting Git SHA from GIT_SHA environment variable."""
        with patch("docker.from_env") as mock_from_env:
            mock_client = MagicMock()
            mock_image = MagicMock()
            mock_image.labels = {}
            mock_image.attrs = {
                "Config": {
                    "Env": ["PATH=/usr/bin", "GIT_SHA=env123sha456", "HOME=/root"]
                }
            }
            mock_client.images.get.return_value = mock_image
            mock_from_env.return_value = mock_client

            git_sha = self.builder.get_git_sha()

            self.assertEqual(git_sha, "env123sha456")

    def test_get_git_sha_not_found(self):
        """Test that None is returned when Git SHA is not found."""
        with patch("docker.from_env") as mock_from_env:
            mock_client = MagicMock()
            mock_image = MagicMock()
            mock_image.labels = {}
            mock_image.attrs = {"Config": {"Env": ["PATH=/usr/bin"]}}
            mock_client.images.get.return_value = mock_image
            mock_from_env.return_value = mock_client

            git_sha = self.builder.get_git_sha()

            # Should return None instead of raising an exception
            self.assertIsNone(git_sha)

    def test_get_git_sha_docker_error(self):
        """Test that None is returned when Docker fails."""
        with patch("docker.from_env") as mock_from_env:
            mock_client = MagicMock()
            mock_client.images.get.side_effect = Exception("Docker daemon not running")
            mock_from_env.return_value = mock_client

            git_sha = self.builder.get_git_sha()

            # Should return None instead of raising an exception
            self.assertIsNone(git_sha)

    def test_get_git_sha_docker_not_installed(self):
        """Test that None is returned when docker package is not available."""
        # Mock the import to raise ImportError
        with patch(
            "builtins.__import__", side_effect=ImportError("No module named 'docker'")
        ):
            git_sha = self.builder.get_git_sha()
            # Should return None when docker package is not available
            self.assertIsNone(git_sha)

    @patch("builtins.open", new_callable=mock_open, read_data=b"tarball content")
    @patch("michelangelo.uniflow.core.file_sync.fsspec.open")
    def test_upload_tarball_success(self, mock_fsspec_open, mock_builtin_open):
        """Test successful tarball upload."""
        mock_remote_file = MagicMock()
        mock_fsspec_open.return_value.__enter__.return_value = mock_remote_file

        self.builder.upload_tarball(
            local_path="/tmp/test.tar.gz", remote_path="s3://bucket/test.tar.gz"
        )

        mock_builtin_open.assert_called_once_with("/tmp/test.tar.gz", "rb")
        mock_fsspec_open.assert_called_once_with("s3://bucket/test.tar.gz", "wb")
        mock_remote_file.write.assert_called_once_with(b"tarball content")

    @patch("builtins.open", new_callable=mock_open, read_data=b"tarball content")
    @patch("michelangelo.uniflow.core.file_sync.fsspec.open")
    def test_upload_tarball_failure(self, mock_fsspec_open, mock_builtin_open):
        """Test tarball upload failure."""
        mock_fsspec_open.side_effect = Exception("S3 connection failed")

        with self.assertRaises(Exception) as ctx:
            self.builder.upload_tarball(
                local_path="/tmp/test.tar.gz", remote_path="s3://bucket/test.tar.gz"
            )

        self.assertIn("S3 connection failed", str(ctx.exception))

    def test_build_and_upload_tarball_integration(self):
        """Test create_and_upload_tarball integration."""
        with (
            patch("docker.from_env") as mock_from_env,
            patch(
                "michelangelo.uniflow.core.file_sync.FileSync.create_diff_tarball_bytes"
            ) as mock_create_tarball,
            patch("builtins.open", new_callable=mock_open),
            patch(
                "michelangelo.uniflow.core.file_sync.fsspec.open"
            ) as mock_fsspec_open,
        ):
            # Mock Docker client
            mock_client = MagicMock()
            mock_image = MagicMock()
            mock_image.labels = {"git.commit": "abc123"}
            mock_client.images.get.return_value = mock_image
            mock_from_env.return_value = mock_client

            # Mock tarball creation
            mock_create_tarball.return_value = b"fake tarball bytes"

            # Mock fsspec upload
            mock_remote_file = MagicMock()
            mock_fsspec_open.return_value.__enter__.return_value = mock_remote_file

            result = self.builder.create_and_upload_tarball()

            # Verify result is a remote path
            self.assertTrue(result.startswith("s3://") or result.startswith("/"))
            self.assertTrue(result.endswith(".tar.gz"))

            # Verify create_diff_tarball_bytes was called
            mock_create_tarball.assert_called_once()


class TestStorageDownloader(unittest.TestCase):
    """Basic unit tests for StorageDownloader and FsspecDownloader."""

    def test_storage_downloader_is_abstract(self):
        """Test that StorageDownloader cannot be instantiated directly."""
        from michelangelo.uniflow.core.file_sync import StorageDownloader

        with self.assertRaises(TypeError):
            # Should fail because it's abstract
            StorageDownloader()

    def test_fsspec_downloader_exists(self):
        """Test that FsspecDownloader class exists and can be instantiated."""
        from michelangelo.uniflow.core.file_sync import FsspecDownloader

        # Should not raise
        downloader = FsspecDownloader()
        self.assertIsNotNone(downloader)

    def test__download_and_extract_dev_files_exists(self):
        """Test that _download_and_extract_dev_files function exists."""
        from michelangelo.uniflow.core.file_sync import (
            _download_and_extract_dev_files,
        )

        # Should not raise
        self.assertIsNotNone(_download_and_extract_dev_files)
        self.assertTrue(callable(_download_and_extract_dev_files))


class TestFsspecDownloader(unittest.TestCase):
    """Unit tests for FsspecDownloader."""

    def setUp(self):
        """Set up test fixtures."""
        import logging

        from michelangelo.uniflow.core.file_sync import FsspecDownloader

        self.downloader = FsspecDownloader()
        self.logger = logging.getLogger("test")

    def test_fsspec_downloader_has_download_method(self):
        """Test that FsspecDownloader has a download method."""
        self.assertTrue(hasattr(self.downloader, "download"))
        self.assertTrue(callable(self.downloader.download))

    @patch("fsspec.open")
    def test_download_success(self, mock_fsspec_open):
        """Test successful download using fsspec."""
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
        """Test download failure due to fsspec error."""
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
        """Test download with different storage protocols (S3, MinIO, etc.)."""
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
        """Test download of large file (simulated)."""
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


class TestDownloadAndExtractDevFiles(unittest.TestCase):
    """Unit tests for _download_and_extract_dev_files function."""

    @patch.dict(os.environ, {}, clear=True)
    def test_returns_false_when_no_tarball_url(self):
        """Test that function returns False when UF_FILE_SYNC_TARBALL_URL is not set."""
        from michelangelo.uniflow.core.file_sync import (
            FsspecDownloader,
            _download_and_extract_dev_files,
        )

        downloader = FsspecDownloader()
        result = _download_and_extract_dev_files(downloader=downloader)
        self.assertFalse(result)

    @patch("michelangelo.uniflow.core.file_sync.Path.cwd")
    @patch("shutil.copy2")
    @patch.dict(os.environ, {"UF_FILE_SYNC_TARBALL_URL": "s3://bucket/test.tar.gz"})
    def test_successful_download_and_extract(self, mock_copy, mock_cwd):
        """Test successful download, extract, and copy workflow."""
        from michelangelo.uniflow.core.file_sync import (
            FsspecDownloader,
            _download_and_extract_dev_files,
        )

        with tempfile.TemporaryDirectory() as fake_repo_root:
            mock_cwd.return_value = Path(fake_repo_root)

            # Create a real tarball with test content
            with tempfile.TemporaryDirectory() as tar_source_dir:
                test_file = Path(tar_source_dir) / "test.py"
                test_file.write_text("print('test')")

                tarball_bytes = io.BytesIO()
                with tarfile.open(fileobj=tarball_bytes, mode="w:gz") as tar:
                    tar.add(test_file, arcname="test.py")
                tarball_bytes.seek(0)

                # Mock the downloader to write the tarball
                def mock_download(remote_path, local_path, logger):
                    local_path.write_bytes(tarball_bytes.getvalue())
                    return True

                downloader = FsspecDownloader()
                with patch.object(downloader, "download", side_effect=mock_download):
                    result = _download_and_extract_dev_files(downloader=downloader)

                self.assertTrue(result)
                # Verify copy was called (file was applied)
                self.assertGreater(mock_copy.call_count, 0)

    @patch.dict(os.environ, {"UF_FILE_SYNC_TARBALL_URL": "s3://bucket/test.tar.gz"})
    def test_returns_false_when_download_fails(self):
        """Test that function returns False when download fails."""
        from michelangelo.uniflow.core.file_sync import (
            FsspecDownloader,
            _download_and_extract_dev_files,
        )

        downloader = FsspecDownloader()
        # Mock download to fail
        with patch.object(downloader, "download", return_value=False):
            result = _download_and_extract_dev_files(downloader=downloader)
            self.assertFalse(result)

    @patch("tarfile.open")
    @patch.dict(os.environ, {"UF_FILE_SYNC_TARBALL_URL": "s3://bucket/test.tar.gz"})
    def test_returns_false_when_extraction_fails(self, mock_tarfile_open):
        """Test that function returns False when tarball extraction fails."""
        from michelangelo.uniflow.core.file_sync import (
            FsspecDownloader,
            _download_and_extract_dev_files,
        )

        # Mock download to succeed
        downloader = FsspecDownloader()
        with tempfile.TemporaryDirectory() as tmp_dir:
            tarball_path = Path(tmp_dir) / "test.tar.gz"
            tarball_path.write_bytes(b"fake tarball")

            def mock_download(remote_path, local_path, logger):
                local_path.write_bytes(b"fake tarball")
                return True

            # Mock tarfile.open to raise TarError
            mock_tarfile_open.side_effect = tarfile.TarError("Invalid tarball")

            with patch.object(downloader, "download", side_effect=mock_download):
                result = _download_and_extract_dev_files(downloader=downloader)
                self.assertFalse(result)

    @patch("michelangelo.uniflow.core.file_sync.Path.cwd")
    @patch("shutil.copy2")
    @patch.dict(os.environ, {"UF_FILE_SYNC_TARBALL_URL": "s3://bucket/test.tar.gz"})
    def test_extracts_multiple_files(self, mock_copy, mock_cwd):
        """Test extraction and copying of multiple files."""
        from michelangelo.uniflow.core.file_sync import (
            FsspecDownloader,
            _download_and_extract_dev_files,
        )

        with tempfile.TemporaryDirectory() as fake_repo_root:
            mock_cwd.return_value = Path(fake_repo_root)

            # Create a tarball with multiple files
            with tempfile.TemporaryDirectory() as tar_source_dir:
                (Path(tar_source_dir) / "file1.py").write_text("print('file1')")
                (Path(tar_source_dir) / "file2.py").write_text("print('file2')")
                subdir = Path(tar_source_dir) / "subdir"
                subdir.mkdir()
                (subdir / "file3.py").write_text("print('file3')")

                tarball_bytes = io.BytesIO()
                with tarfile.open(fileobj=tarball_bytes, mode="w:gz") as tar:
                    tar.add(tar_source_dir, arcname=".")
                tarball_bytes.seek(0)

                def mock_download(remote_path, local_path, logger):
                    local_path.write_bytes(tarball_bytes.getvalue())
                    return True

                downloader = FsspecDownloader()
                with patch.object(downloader, "download", side_effect=mock_download):
                    result = _download_and_extract_dev_files(downloader=downloader)

                self.assertTrue(result)
                # Should have copied multiple files
                self.assertGreaterEqual(mock_copy.call_count, 3)

    @patch("michelangelo.uniflow.core.file_sync.Path.cwd")
    @patch.dict(os.environ, {"UF_FILE_SYNC_TARBALL_URL": "s3://bucket/test.tar.gz"})
    def test_handles_unexpected_errors(self, mock_cwd):
        """Test that function handles unexpected errors gracefully."""
        from michelangelo.uniflow.core.file_sync import (
            FsspecDownloader,
            _download_and_extract_dev_files,
        )

        # Make cwd() raise an exception
        mock_cwd.side_effect = Exception("Unexpected error")
        downloader = FsspecDownloader()

        with patch.object(downloader, "download", return_value=True):
            result = _download_and_extract_dev_files(downloader=downloader)
            self.assertFalse(result)


class TestFileSyncPreRun(unittest.TestCase):
    """Unit tests for _file_sync_pre_run function."""

    def setUp(self):
        """Set up test fixtures."""
        # Reset the global flag before each test
        import michelangelo.uniflow.core.file_sync as file_sync_module

        file_sync_module._file_sync_executed = False

    def tearDown(self):
        """Clean up after each test."""
        # Reset the global flag after each test
        import michelangelo.uniflow.core.file_sync as file_sync_module

        file_sync_module._file_sync_executed = False

    @patch("michelangelo.uniflow.core.file_sync._download_and_extract_dev_files")
    @patch.dict(os.environ, {"UF_FILE_SYNC_TARBALL_URL": "s3://bucket/test.tar.gz"})
    def test_file_sync_executed_flag_prevents_double_execution(self, mock_download):
        """Test that _file_sync_executed flag prevents multiple executions."""
        from michelangelo.uniflow.core.file_sync import (
            FsspecDownloader,
            _file_sync_pre_run,
        )

        mock_download.return_value = True
        downloader = FsspecDownloader()

        # First call should execute
        _file_sync_pre_run(downloader=downloader)
        self.assertEqual(mock_download.call_count, 1)

        # Second call should be skipped due to flag
        _file_sync_pre_run(downloader=downloader)
        self.assertEqual(mock_download.call_count, 1)  # Still 1, not 2

        # Third call should also be skipped
        _file_sync_pre_run(downloader=downloader)
        self.assertEqual(mock_download.call_count, 1)  # Still 1, not 3

    @patch("michelangelo.uniflow.core.file_sync._download_and_extract_dev_files")
    @patch.dict(os.environ, {"UF_FILE_SYNC_TARBALL_URL": "s3://bucket/test.tar.gz"})
    def test_file_sync_executed_flag_is_set_after_first_call(self, mock_download):
        """Test that _file_sync_executed flag is set to True after first execution."""
        import michelangelo.uniflow.core.file_sync as file_sync_module
        from michelangelo.uniflow.core.file_sync import (
            FsspecDownloader,
            _file_sync_pre_run,
        )

        mock_download.return_value = True
        downloader = FsspecDownloader()

        # Flag should be False initially
        self.assertFalse(file_sync_module._file_sync_executed)

        # Execute
        _file_sync_pre_run(downloader=downloader)

        # Flag should be True after execution
        self.assertTrue(file_sync_module._file_sync_executed)

    @patch("michelangelo.uniflow.core.file_sync._download_and_extract_dev_files")
    @patch.dict(os.environ, {}, clear=True)
    def test_file_sync_executed_flag_set_even_without_tarball_url(self, mock_download):
        """Test that flag is set even when UF_FILE_SYNC_TARBALL_URL is not set."""
        import michelangelo.uniflow.core.file_sync as file_sync_module
        from michelangelo.uniflow.core.file_sync import (
            FsspecDownloader,
            _file_sync_pre_run,
        )

        downloader = FsspecDownloader()

        # Flag should be False initially
        self.assertFalse(file_sync_module._file_sync_executed)

        # Execute (should just log and return, but still set flag)
        _file_sync_pre_run(downloader=downloader)

        # Flag should be True even though no download happened
        self.assertTrue(file_sync_module._file_sync_executed)

        # Download should not have been called
        mock_download.assert_not_called()

    @patch("michelangelo.uniflow.core.file_sync._download_and_extract_dev_files")
    @patch.dict(os.environ, {"UF_FILE_SYNC_TARBALL_URL": "s3://bucket/test.tar.gz"})
    def test_file_sync_executed_flag_set_even_on_error(self, mock_download):
        """Test that flag is set even when download fails."""
        import michelangelo.uniflow.core.file_sync as file_sync_module
        from michelangelo.uniflow.core.file_sync import (
            FsspecDownloader,
            _file_sync_pre_run,
        )

        # Make download fail
        mock_download.side_effect = Exception("Download failed")
        downloader = FsspecDownloader()

        # Flag should be False initially
        self.assertFalse(file_sync_module._file_sync_executed)

        # Execute (will catch exception internally)
        _file_sync_pre_run(downloader=downloader)

        # Flag should be True even though download failed
        self.assertTrue(file_sync_module._file_sync_executed)

    @patch("michelangelo.uniflow.core.file_sync._download_and_extract_dev_files")
    @patch.dict(os.environ, {"UF_FILE_SYNC_TARBALL_URL": "s3://bucket/test.tar.gz"})
    def test_calls_download_and_extract_with_downloader(self, mock_download):
        """Test _file_sync_pre_run calls _download_and_extract_dev_files."""
        from michelangelo.uniflow.core.file_sync import (
            FsspecDownloader,
            _file_sync_pre_run,
        )

        mock_download.return_value = True
        downloader = FsspecDownloader()

        _file_sync_pre_run(downloader=downloader)

        # Verify _download_and_extract_dev_files was called with the downloader
        mock_download.assert_called_once_with(downloader=downloader)


class TestDevRunStorageUrl(unittest.TestCase):
    """Unit tests for dev_run --storage-url parameter functionality."""

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.create.handle_workflow_inputs_retrieval")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.yaml_to_dict")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_name")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_object")
    def test_storage_url_passed_to_workflow_retrieval(self, mock_gen_obj, mock_gen_name, mock_yaml, mock_handle):
        """Test that storage_url parameter is correctly passed through dev_run to handle_workflow_inputs_retrieval."""
        from michelangelo.cli.mactl.plugins.entity.pipeline.dev_run import convert_crd_metadata_pipeline_dev_run
        from pathlib import Path
        from unittest.mock import MagicMock
        from google.protobuf.message import Message

        # Setup mock returns
        mock_yaml.return_value = {
            'metadata': {'namespace': 'test-project', 'name': 'test-pipeline'},
            'spec': {}
        }
        mock_handle.return_value = ({}, 's3://test/path.tar.gz', 'test_workflow')
        mock_gen_name.return_value = 'test-run-123'
        mock_gen_obj.return_value = {'spec': {}}

        # Create test data
        yaml_dict = {
            'metadata': {'namespace': 'test-project', 'name': 'test-pipeline'},
            'spec': {}
        }
        crd_class = MagicMock(spec=Message)
        yaml_path = Path('/tmp/test.yaml')
        test_storage_url = 's3://custom-bucket/custom-path'

        # Call the function with storage_url
        result = convert_crd_metadata_pipeline_dev_run(
            yaml_dict, crd_class, yaml_path, storage_url=test_storage_url
        )

        # Verify that handle_workflow_inputs_retrieval was called with the correct storage_url
        mock_handle.assert_called_once()
        args, kwargs = mock_handle.call_args

        # The storage_url should be the last positional argument
        self.assertEqual(len(args), 5, f"Expected 5 args (including storage_url), got {len(args)}")
        self.assertEqual(args[4], test_storage_url, f"Expected storage_url '{test_storage_url}', got '{args[4]}'")

        # Verify the function returns a valid result
        self.assertIsInstance(result, dict)
        self.assertIn('pipeline_run', result)

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.create.handle_workflow_inputs_retrieval")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.yaml_to_dict")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_name")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_object")
    def test_storage_url_none_by_default(self, mock_gen_obj, mock_gen_name, mock_yaml, mock_handle):
        """Test that storage_url parameter defaults to None when not provided."""
        from michelangelo.cli.mactl.plugins.entity.pipeline.dev_run import convert_crd_metadata_pipeline_dev_run
        from pathlib import Path
        from unittest.mock import MagicMock
        from google.protobuf.message import Message

        # Setup mock returns
        mock_yaml.return_value = {
            'metadata': {'namespace': 'test-project', 'name': 'test-pipeline'},
            'spec': {}
        }
        mock_handle.return_value = ({}, 's3://test/path.tar.gz', 'test_workflow')
        mock_gen_name.return_value = 'test-run-123'
        mock_gen_obj.return_value = {'spec': {}}

        # Create test data
        yaml_dict = {
            'metadata': {'namespace': 'test-project', 'name': 'test-pipeline'},
            'spec': {}
        }
        crd_class = MagicMock(spec=Message)
        yaml_path = Path('/tmp/test.yaml')

        # Call the function without storage_url (should default to None)
        result = convert_crd_metadata_pipeline_dev_run(yaml_dict, crd_class, yaml_path)

        # Verify that handle_workflow_inputs_retrieval was called with None storage_url
        mock_handle.assert_called_once()
        args, kwargs = mock_handle.call_args

        # The storage_url should be the last positional argument and None
        self.assertEqual(len(args), 5, f"Expected 5 args (including storage_url), got {len(args)}")
        self.assertIsNone(args[4], f"Expected storage_url to be None, got '{args[4]}'")

        # Verify the function returns a valid result
        self.assertIsInstance(result, dict)
        self.assertIn('pipeline_run', result)

    @patch("michelangelo.cli.mactl.utils.run_subprocess_registration")
    def test_storage_url_passed_to_subprocess_registration(self, mock_subprocess):
        """Test that storage_url is passed correctly to run_subprocess_registration."""
        from michelangelo.cli.mactl.plugins.entity.pipeline.create import get_pipeline_config_and_tar
        from pathlib import Path
        from unittest.mock import MagicMock
        import tempfile

        # Setup mock subprocess to return success
        mock_subprocess.return_value = MagicMock(returncode=0, stdout="", stderr="")

        # Create temporary files to simulate the subprocess output
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp_path = Path(tmp_dir)

            # Create mock output files
            (tmp_path / "registration_success.txt").write_text("SUCCESS: s3://test/output.tar.gz")
            (tmp_path / "uniflow_tar_path.txt").write_text("s3://test/output.tar.gz")
            (tmp_path / "uniflow_input.txt").write_text('{"environ": {}, "kwargs": []}')
            (tmp_path / "workflow_function_name.txt").write_text("test_workflow")

            # Create a mock config file
            config_path = tmp_path / "config.yaml"
            config_path.write_text("apiVersion: v1\nkind: Pipeline")

            # Mock the read_subprocess_outputs function to return success
            with patch("michelangelo.cli.mactl.plugins.entity.pipeline.create.read_subprocess_outputs") as mock_read:
                mock_read.return_value = (True, "Success", "s3://test/output.tar.gz")

                test_storage_url = "s3://custom-bucket/my-path"

                # Call the function with storage_url
                result = get_pipeline_config_and_tar(
                    repo_root=tmp_path,
                    config_file_relative_path="config.yaml",
                    bazel_target="",
                    project="test-project",
                    pipeline="test-pipeline",
                    storage_url=test_storage_url
                )

                # Verify run_subprocess_registration was called with the correct storage_url
                mock_subprocess.assert_called_once()
                args, kwargs = mock_subprocess.call_args

                # Check that storage_url was passed correctly
                self.assertIn("storage_url", kwargs)
                self.assertEqual(kwargs["storage_url"], test_storage_url)

                # Verify other parameters are as expected
                self.assertEqual(kwargs["project"], "test-project")
                self.assertEqual(kwargs["pipeline"], "test-pipeline")

                # Verify the function returns the expected tuple
                self.assertIsInstance(result, tuple)
                self.assertEqual(len(result), 3)


if __name__ == "__main__":
    unittest.main()
