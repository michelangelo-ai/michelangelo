import os
import yaml
import tempfile
from unittest import TestCase
from michelangelo.lib.model_manager._private.utils.model_utils import (
    convert_to_deployable_download_yaml_content,
    convert_to_deployable_download_yaml,
    is_deployable_download_yaml_content,
    convert_download_yamls_to_deployable,
)


class DownloadYamlTest(TestCase):
    def setUp(self):
        self.content = {
            "assets": [
                {"a": "root/a"},
                {"b": "root/b"},
            ],
            "source_type": "hdfs",
            "source_prefix": "root/",
        }
        self.converted_content = {
            "assets": [
                {"a": "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/0/main/model/a"},
                {"b": "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/0/main/model/b"},
            ],
            "source_type": "terrablob",
            "source_prefix": "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/0/main/model/",
        }
        self.deployable_content = {
            "assets": [
                {"a": "root/a"},
                {"b": "root/b"},
            ],
            "source_type": "terrablob",
            "source_prefix": "root/",
        }

    def test_convert_to_deployable_download_yaml_content(self):
        self.assertEqual(
            convert_to_deployable_download_yaml_content(self.content, "test_project", "test_model", "0"),
            self.converted_content,
        )

    def test_convert_to_deployable_download_yaml_content_no_source_prefix(self):
        content = {"assets": self.content["assets"], "source_type": self.content["source_type"]}
        converted_content = {
            "assets": [
                {"a": "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/0/main/model/root/a"},
                {"b": "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/0/main/model/root/b"},
            ],
            "source_type": "terrablob",
        }
        self.assertEqual(
            convert_to_deployable_download_yaml_content(content, "test_project", "test_model", "0"),
            converted_content,
        )

    def test_convert_to_deployable_download_yaml_content_no_assets(self):
        content = {"source_type": self.content["source_type"], "source_prefix": self.content["source_prefix"]}
        converted_content = {
            "source_type": "terrablob",
            "source_prefix": "root/",
        }
        self.assertEqual(
            convert_to_deployable_download_yaml_content(content, "test_project", "test_model", "0"),
            converted_content,
        )

        content = {"assets": [], "source_type": self.content["source_type"]}
        converted_content = {
            "assets": [],
            "source_type": "terrablob",
        }
        self.assertEqual(
            convert_to_deployable_download_yaml_content(content, "test_project", "test_model", "0"),
            converted_content,
        )

    def test_convert_to_deployable_download_yaml_content_no_conversion(self):
        self.assertEqual(
            convert_to_deployable_download_yaml_content(self.deployable_content, "test_project", "test_model", "0"),
            self.deployable_content,
        )

    def test_is_deployable_download_yaml_content(self):
        self.assertTrue(is_deployable_download_yaml_content(self.deployable_content))
        self.assertFalse(is_deployable_download_yaml_content(self.content))
        self.assertTrue(is_deployable_download_yaml_content({}))
        self.assertTrue(is_deployable_download_yaml_content({"assets": [], "source_type": "terrablob"}))
        self.assertTrue(is_deployable_download_yaml_content({"assets": []}))

    def test_convert_to_deployable_download_yaml(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            file = os.path.join(temp_dir, "download.yaml")
            with open(file, "w") as f:
                yaml.dump(self.content, f)

            convert_to_deployable_download_yaml(file, "test_project", "test_model", "0")

            with open(file) as f:
                content = yaml.safe_load(f)
                self.assertEqual(content, self.converted_content)

    def test_convert_to_deployable_download_yaml_no_conversion(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            file = os.path.join(temp_dir, "download.yaml")
            with open(file, "w") as f:
                yaml.dump(self.deployable_content, f)

            convert_to_deployable_download_yaml(file, "test_project", "test_model", "0")

            with open(file) as f:
                content = yaml.safe_load(f)
                self.assertEqual(content, self.deployable_content)

    def test_convert_download_yamls_to_deployable(self):
        with tempfile.TemporaryDirectory() as model_path:
            os.makedirs(os.path.join(model_path, "subdir"))
            os.makedirs(os.path.join(model_path, "subdir2", "subsubdir"))
            with open(os.path.join(model_path, "config.pbtxt"), "w") as f:
                f.write("config")

            with open(os.path.join(model_path, "download.yaml"), "w") as f:
                yaml.dump(self.content, f)

            with open(os.path.join(model_path, "subdir", "download.yaml"), "w") as f:
                yaml.dump(self.content, f)

            with open(os.path.join(model_path, "subdir2", "subsubdir", "download.yaml"), "w") as f:
                yaml.dump(self.content, f)

            convert_download_yamls_to_deployable(model_path, "test_project", "test_model", "0")

            with open(os.path.join(model_path, "download.yaml")) as f:
                content = yaml.safe_load(f)
                self.assertEqual(content, self.converted_content)

            with open(os.path.join(model_path, "subdir", "download.yaml")) as f:
                content = yaml.safe_load(f)
                self.assertEqual(content, self.converted_content)

            with open(os.path.join(model_path, "subdir2", "subsubdir", "download.yaml")) as f:
                content = yaml.safe_load(f)
                self.assertEqual(content, self.converted_content)
