import os
import tempfile
from unittest import TestCase
from michelangelo._internal.utils.file_utils import generate_folder, cd


class FolderGeneratorTest(TestCase):
    def test_generate_folder(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            tmp_binary_file = os.path.join(temp_dir, "tmp_binary_file")
            with open(tmp_binary_file, "wb") as f:
                f.write(b"binary_file_content")

            tmp_text_file = os.path.join(temp_dir, "tmp_text_file")
            with open(tmp_text_file, "w+") as f:
                f.write("text_file_content")

            tmp_sub_dir = os.path.join(temp_dir, "tmp_sub_dir")
            os.mkdir(tmp_sub_dir)
            with open(os.path.join(tmp_sub_dir, "tmp_file_1"), "w+") as f:
                f.write("tmp_file_content_1")

            with open(os.path.join(tmp_sub_dir, "tmp_file_2"), "w+") as f:
                f.write("tmp_file_content_2")
            tmp_sub_sub_dir = os.path.join(tmp_sub_dir, "tmp_sub_sub_dir")
            os.mkdir(tmp_sub_sub_dir)
            with open(os.path.join(tmp_sub_sub_dir, "tmp_file_3"), "w+") as f:
                f.write("tmp_file_content_3")

            content = {
                "config.pbtxt": "a",
                "0": {
                    "model.py": "b",
                    "download.yaml": "c",
                    "logs": {"log": "d"},
                    "binary_file": f"file://{tmp_binary_file}",
                    "text_file": f"file://{tmp_text_file}",
                    ".": f"dir://{tmp_sub_dir}",
                    "sub_dir": f"dir://{tmp_sub_dir}",
                },
            }

            with cd(temp_dir):
                generate_folder(content, "triton")

            pjoin = os.path.join

            files = os.listdir(pjoin(temp_dir, "triton"))
            self.assertEqual(sorted(files), ["0", "config.pbtxt"])

            files = os.listdir(pjoin(temp_dir, "triton", "0"))
            self.assertEqual(
                sorted(files),
                [
                    "binary_file",
                    "download.yaml",
                    "logs",
                    "model.py",
                    "sub_dir",
                    "text_file",
                    "tmp_file_1",
                    "tmp_file_2",
                    "tmp_sub_sub_dir",
                ],
            )

            files = os.listdir(pjoin(temp_dir, "triton", "0", "logs"))
            self.assertEqual(files, ["log"])

            with open(pjoin(temp_dir, "triton", "config.pbtxt")) as f:
                config_pbtext = f.read()
                self.assertEqual(config_pbtext, "a")

            with open(pjoin(temp_dir, "triton", "0", "model.py")) as f:
                model_py = f.read()
                self.assertEqual(model_py, "b")

            with open(pjoin(temp_dir, "triton", "0", "download.yaml")) as f:
                download_yaml = f.read()
                self.assertEqual(download_yaml, "c")

            with open(pjoin(temp_dir, "triton", "0", "logs", "log")) as f:
                log = f.read()
                self.assertEqual(log, "d")

            with open(pjoin(temp_dir, "triton", "0", "binary_file"), "rb") as f:
                binary_content = f.read()
                self.assertEqual(binary_content, b"binary_file_content")

            with open(pjoin(temp_dir, "triton", "0", "text_file")) as f:
                text_content = f.read()
                self.assertEqual(text_content, "text_file_content")

            with open(pjoin(temp_dir, "triton", "0", "sub_dir", "tmp_file_1")) as f:
                content = f.read()
                self.assertEqual(content, "tmp_file_content_1")

            with open(pjoin(temp_dir, "triton", "0", "sub_dir", "tmp_file_2")) as f:
                content = f.read()
                self.assertEqual(content, "tmp_file_content_2")

            with open(pjoin(temp_dir, "triton", "0", "tmp_file_1")) as f:
                content = f.read()
                self.assertEqual(content, "tmp_file_content_1")

            with open(pjoin(temp_dir, "triton", "0", "tmp_file_2")) as f:
                content = f.read()
                self.assertEqual(content, "tmp_file_content_2")

            with open(
                pjoin(temp_dir, "triton", "0", "tmp_sub_sub_dir", "tmp_file_3")
            ) as f:
                content = f.read()
                self.assertEqual(content, "tmp_file_content_3")

    def test_generate_model_package_folder_with_pre_existing_files(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            root_path = os.path.join(temp_dir, "triton")
            os.makedirs(root_path)

            with open(os.path.join(root_path, "config.pbtxt"), "w+") as f:
                f.write("old_config_pbtxt")

            sub_path_0 = os.path.join(root_path, "0")
            os.makedirs(sub_path_0)

            with open(os.path.join(sub_path_0, "model.py"), "w+") as f:
                f.write("old_model_py")

            sub_path_1 = os.path.join(root_path, "1")
            os.makedirs(sub_path_1)

            with open(os.path.join(sub_path_1, "tmp_file_1"), "w+") as f:
                f.write("tmp_file_1_content")

            with open(os.path.join(sub_path_1, "tmp_file_2"), "w+") as f:
                f.write("tmp_file_2_content")

            sub_sub_path_1 = os.path.join(sub_path_1, "sub_1")
            os.makedirs(sub_sub_path_1)

            with open(os.path.join(sub_sub_path_1, "tmp_file_3"), "w+") as f:
                f.write("tmp_file_3_content")

            content = {
                "config.pbtxt": "a",
                "0": {
                    "model.py": f"file://{sub_path_0}",
                },
                "1": {
                    "tmp_file_1": "tmp_file_1_content_1",
                    ".": f"dir://{sub_path_1}",
                },
            }

            generate_folder(content, root_path)

            pjoin = os.path.join

            files = os.listdir(os.path.join(temp_dir, "triton"))
            self.assertEqual(sorted(files), ["0", "1", "config.pbtxt"])

            with open(pjoin(temp_dir, "triton", "config.pbtxt")) as f:
                config_pbtext = f.read()
                self.assertEqual(config_pbtext, "old_config_pbtxt")

            with open(pjoin(temp_dir, "triton", "0", "model.py")) as f:
                model_py = f.read()
                self.assertEqual(model_py, "old_model_py")

            with open(pjoin(temp_dir, "triton", "1", "tmp_file_1")) as f:
                content = f.read()
                self.assertEqual(content, "tmp_file_1_content")

            with open(pjoin(temp_dir, "triton", "1", "tmp_file_2")) as f:
                content = f.read()
                self.assertEqual(content, "tmp_file_2_content")

            with open(pjoin(temp_dir, "triton", "1", "sub_1", "tmp_file_3")) as f:
                content = f.read()
                self.assertEqual(content, "tmp_file_3_content")
