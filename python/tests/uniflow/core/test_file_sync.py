import unittest
from michelangelo.uniflow.core.file_sync import (
    FileSync,
    DefaultFileSync,
)
from pathlib import Path
from unittest.mock import patch, MagicMock, mock_open
import tempfile
import os
import tarfile
import io


class _TestableFileBuilder(FileSync):
    """Concrete implementation for testing abstract base class"""

    def get_git_sha(self):
        return "0241feca9a6a681c917c3bb712dcb62918522aed"

    def upload_tarball(self, local_path: str, remote_path: str):
        pass


class TestFileSync(unittest.TestCase):
    def setUp(self):
        self.builder = _TestableFileBuilder()
        os.environ["UF_BASE_PROJECTS_PATH"] = (
            "/prod/michelangelo/uniflow/uniflow_dev_run/projects"
        )

    def test_get_random_file_name(self):
        file_name = self.builder.get_random_file_name()
        self.assertIsNotNone(file_name)
        self.assertTrue(file_name.startswith("file-sync-"))
        self.assertTrue(file_name.endswith(".tar.gz"))

    def test_get_random_file_name_with_none_pipeline(self):
        """Test that filename always uses 'file-sync' prefix"""
        builder = _TestableFileBuilder()
        file_name = builder.get_random_file_name()
        self.assertIn("file-sync", file_name)
        self.assertTrue(file_name.endswith(".tar.gz"))

    def test_get_file_name(self):
        file_name = self.builder.get_file_name()
        self.assertTrue(file_name.startswith("file-sync"))
        self.assertTrue(file_name.endswith(".tar.gz"))

    def test_get_remote_file_path(self):
        os.environ["UF_FILE_SYNC_STORAGE_URL"] = "s3://test-bucket/file-sync"
        remote_file_path = self.builder.get_remote_file_path()
        self.assertTrue(remote_file_path.startswith("s3://test-bucket/file-sync/"))
        self.assertTrue(remote_file_path.endswith(".tar.gz"))

    def test_get_remote_file_path_with_fallback(self):
        """Test fallback to default when env var is not set"""
        if "UF_FILE_SYNC_STORAGE_URL" in os.environ:
            del os.environ["UF_FILE_SYNC_STORAGE_URL"]

        # Create a new builder without the env var set
        builder = _TestableFileBuilder()
        remote_file_path = builder.get_remote_file_path()
        # Should use the default path from file_sync.py
        self.assertTrue(remote_file_path.startswith("s3://default/uniflow/"))
        self.assertTrue(remote_file_path.endswith(".tar.gz"))

    def test_create_diff_tarball_bytes_strips_python_prefix(self):
        """Test that python/ prefix is stripped from arcname in tarball"""
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
        self.builder.get_git_sha = MagicMock(
            return_value="0241feca9a6a681c917c3bb712dcb62918522aed"
        )
        git_sha = self.builder.get_git_sha()
        self.assertEqual(git_sha, "0241feca9a6a681c917c3bb712dcb62918522aed")


class TestDefaultFileSync(unittest.TestCase):
    """Unit tests for DefaultFileSync"""

    def setUp(self):
        self.builder = DefaultFileSync(
            docker_image="examples:latest",
        )

    def test_init(self):
        """Test initialization"""
        self.assertEqual(self.builder._docker_image, "examples:latest")

    def test_get_git_sha_from_label_git_commit_actual(self):
        """Test getting Git SHA from git.commit label (actual implementation)"""
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
        """Test getting Git SHA from git.sha label"""
        with patch("docker.from_env") as mock_from_env:
            mock_client = MagicMock()
            mock_image = MagicMock()
            mock_image.labels = {"git.sha": "xyz789abc123"}
            mock_client.images.get.return_value = mock_image
            mock_from_env.return_value = mock_client

            git_sha = self.builder.get_git_sha()

            self.assertEqual(git_sha, "xyz789abc123")

    def test_get_git_sha_from_env_var(self):
        """Test getting Git SHA from GIT_SHA environment variable"""
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
        """Test that None is returned when Git SHA is not found"""
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
        """Test that None is returned when Docker fails"""
        with patch("docker.from_env") as mock_from_env:
            mock_client = MagicMock()
            mock_client.images.get.side_effect = Exception("Docker daemon not running")
            mock_from_env.return_value = mock_client

            git_sha = self.builder.get_git_sha()

            # Should return None instead of raising an exception
            self.assertIsNone(git_sha)

    def test_get_git_sha_docker_not_installed(self):
        """Test that None is returned when docker package is not available"""
        # Mock the import to raise ImportError
        with patch("builtins.__import__", side_effect=ImportError("No module named 'docker'")):
            git_sha = self.builder.get_git_sha()
            # Should return None when docker package is not available
            self.assertIsNone(git_sha)

    @patch("builtins.open", new_callable=mock_open, read_data=b"tarball content")
    @patch("michelangelo.uniflow.core.file_sync.fsspec.open")
    def test_upload_tarball_success(self, mock_fsspec_open, mock_builtin_open):
        """Test successful tarball upload"""
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
        """Test tarball upload failure"""
        mock_fsspec_open.side_effect = Exception("S3 connection failed")

        with self.assertRaises(Exception) as ctx:
            self.builder.upload_tarball(
                local_path="/tmp/test.tar.gz", remote_path="s3://bucket/test.tar.gz"
            )

        self.assertIn("S3 connection failed", str(ctx.exception))

    def test_build_and_upload_tarball_integration(self):
        """Test create_and_upload_tarball integration"""
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
