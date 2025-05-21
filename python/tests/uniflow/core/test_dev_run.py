import unittest
from michelangelo.uniflow.core.dev_run import (
    UniflowDevRunFileBuilder,
)
from pathlib import Path
from unittest.mock import patch, MagicMock
import tempfile
import os


class TestUniflowDevRunFileBuilder(unittest.TestCase):
    def setUp(self):
        class TestableFileBuilder(UniflowDevRunFileBuilder):
            def get_git_sha(self):
                return "0241feca9a6a681c917c3bb712dcb62918522aed"

            def upload_tarball(self, local_path: str, remote_path: str):
                pass

        self.builder = TestableFileBuilder(
            project="my_project",
            pipeline="my_pipeline",
        )
        os.environ["UF_BASE_PROJECTS_PATH"] = (
            "/prod/michelangelo/uniflow/uniflow_dev_run/projects"
        )

    def test_get_random_file_name(self):
        file_name = self.builder.get_random_file_name()
        self.assertIsNotNone(file_name)
        self.assertTrue(file_name.endswith(".tar.gz"))

    def test_get_file_name(self):
        file_name = self.builder.get_file_name()
        self.assertTrue(file_name.startswith("my_pipeline"))
        self.assertTrue(file_name.endswith(".tar.gz"))

    def test_get_remote_file_path(self):
        remote_file_path = self.builder.get_remote_file_path()
        self.assertTrue(
            remote_file_path.startswith(
                "/prod/michelangelo/uniflow/uniflow_dev_run/projects/my_project/"
            )
        )
        self.assertTrue(remote_file_path.endswith(".tar.gz"))

    def test_create_diff_tarball_bytes(self):
        self.builder.get_git_sha = MagicMock(
            return_value="0241feca9a6a681c917c3bb712dcb62918522aed"
        )

        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp_dir_path = Path(tmp_dir)
            file1 = tmp_dir_path / "file1.py"
            file2 = tmp_dir_path / "file2.py"
            file1.write_text("print('hello')")
            file2.write_text("print('world')")

            with (
                patch("michelangelo.uniflow.core.dev_run.subprocess.run") as mock_run,
                patch("pathlib.Path.exists", return_value=True),
                patch(
                    "michelangelo.uniflow.core.dev_run.Path",
                    side_effect=lambda p: tmp_dir_path / p,
                ),
            ):
                mock_run.return_value = MagicMock(
                    stdout="file1.py\nfile2.py", returncode=0
                )
                tarball_bytes = self.builder.create_diff_tarball_bytes()
                self.assertIsNotNone(tarball_bytes)
                self.assertTrue(isinstance(tarball_bytes, bytes))

    def test_create_and_upload_tarball_success(self):
        with (
            patch(
                "michelangelo.uniflow.core.dev_run.UniflowDevRunFileBuilder.create_diff_tarball_bytes",
                return_value=b"fake-bytes",
            ) as mock_tarball,
            patch(
                "michelangelo.uniflow.core.dev_run.UniflowDevRunFileBuilder.get_file_name",
                return_value="fake.tar.gz",
            ) as mock_filename,
            patch(
                "michelangelo.uniflow.core.dev_run.UniflowDevRunFileBuilder.get_remote_file_path",
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
                "michelangelo.uniflow.core.dev_run.UniflowDevRunFileBuilder.create_diff_tarball_bytes",
                return_value=None,
            ) as mock_tarball,
            patch(
                "michelangelo.uniflow.core.dev_run.UniflowDevRunFileBuilder.upload_tarball"
            ) as mock_upload,
        ):
            result = self.builder.create_and_upload_tarball()
            self.assertEqual(result, "")
            mock_tarball.assert_called_once()
            mock_upload.assert_not_called()
