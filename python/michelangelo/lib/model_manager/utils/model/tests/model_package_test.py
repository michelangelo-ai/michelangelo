import os
import yaml
import tempfile
from unittest import TestCase
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager.utils.model import retrieve_model_assets


def create_download_yaml_content(source: str):
    with open(os.path.join(source, "a"), "w") as f:
        f.write("a")
    with open(os.path.join(source, "b"), "w") as f:
        f.write("b")
    with open(os.path.join(source, "c"), "w") as f:
        f.write("c")

    return {
        "assets": [
            {"a": f"{source}/a"},
            {"y/b": f"{source}/b"},
            {"x/y/c": f"{source}/c"},
        ],
        "source_type": StorageType.LOCAL,
        "source_prefix": f"{source}/",
    }


class ModelPackageTest(TestCase):
    def test_retrieve_model_assets(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            source = os.path.join(temp_dir, "source")
            os.makedirs(source, exist_ok=True)
            create_download_yaml_content(source)

            target = os.path.join(temp_dir, "target")

            subdir1 = os.path.join(target, "subdir1")
            os.makedirs(subdir1, exist_ok=True)

            subsubdir2 = os.path.join(target, "subdir2", "subsubdir2")
            os.makedirs(subsubdir2, exist_ok=True)

            with open(os.path.join(target, "download.yaml"), "w") as f:
                yaml.dump(create_download_yaml_content(source), f)

            with open(os.path.join(subdir1, "file.txt"), "w") as f:
                f.write("file_content")

            with open(os.path.join(subsubdir2, "download.yaml"), "w") as f:
                yaml.dump(create_download_yaml_content(source), f)

            retrieve_model_assets(target)

            files = sorted(
                [os.path.relpath(os.path.join(dirpath, filename), target) for dirpath, _, filenames in os.walk(target) for filename in filenames]
            )

            self.assertEqual(
                files,
                [
                    "a",
                    "subdir1/file.txt",
                    "subdir2/subsubdir2/a",
                    "subdir2/subsubdir2/x/y/c",
                    "subdir2/subsubdir2/y/b",
                    "x/y/c",
                    "y/b",
                ],
            )
