import os
import tempfile
from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager._private.utils.asset_utils import download_assets


class DownloadTest(TestCase):
    def test_download_assets_local(self):
        with (
            tempfile.TemporaryDirectory() as src_path,
            tempfile.TemporaryDirectory() as des_path,
        ):
            with open(os.path.join(src_path, "file1.txt"), "w") as f:
                f.write("file1")
            os.makedirs(os.path.join(src_path, "subdir1"))
            os.makedirs(os.path.join(src_path, "subdir2"))
            with open(os.path.join(src_path, "subdir1", "file2.txt"), "w") as f:
                f.write("file2")

            download_assets(src_path, des_path, StorageType.LOCAL)

            paths = []

            for dirpath, _, filenames in os.walk(des_path):
                paths.extend(
                    [os.path.join(dirpath, filename) for filename in filenames]
                )
                if len(filenames) == 0:
                    paths.append(dirpath)

            paths = sorted(paths)

            self.assertEqual(
                paths,
                [
                    os.path.join(des_path, "file1.txt"),
                    os.path.join(des_path, "subdir1", "file2.txt"),
                    os.path.join(des_path, "subdir2"),
                ],
            )

    def test_download_assets_local_single_file(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            source = os.path.join(temp_dir, "source")
            with open(source, "w") as f:
                f.write("content")

            destination = os.path.join(temp_dir, "destination")

            download_assets(source, destination, StorageType.LOCAL)

            with open(destination) as f:
                self.assertEqual(f.read(), "content")

    def test_download_assets_unknown_source_type(self):
        with tempfile.TemporaryDirectory() as des_path:
            download_assets("src_path", des_path, "unknown")
            self.assertEqual(os.listdir(des_path), [])
