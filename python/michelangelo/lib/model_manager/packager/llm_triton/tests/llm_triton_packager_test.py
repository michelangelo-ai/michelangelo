import os
import json
import tempfile
from unittest import TestCase
from unittest.mock import patch
from pathlib import PurePath
from uber.ai.michelangelo.sdk.model_manager.constants import StorageType
from uber.ai.michelangelo.sdk.model_manager.packager.llm_triton import LLMTritonPackager
from uber.ai.michelangelo.sdk.model_manager._private.constants import LLMModelType
from uber.ai.michelangelo.sdk.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.tests.fixtures.sample_config_pbtxt import SAMPLE_CONFIG_PBTXT


class LLMTritonPackagerTest(TestCase):
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.list.list_terrablob_dir")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.model_package.infer_llm_model_type")
    def test_create_model_package(
        self,
        mock_infer_llm_model_type,
        mock_list_terrablob_dir,
    ):
        mock_list_terrablob_dir.return_value = ["a", "b"]
        mock_infer_llm_model_type.return_value = LLMModelType.UNKNOWN

        packager = LLMTritonPackager()

        model_path = packager.create_model_package("model_path")

        files = sorted(
            [
                str(
                    PurePath(os.path.join(dirpath, file)).relative_to(model_path),
                )
                for dirpath, _, filenames in os.walk(model_path)
                for file in filenames
            ],
        )
        self.assertEqual(files, ["0/download.yaml", "0/model.py", "0/user_model.py", "config.pbtxt"])

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.list.list_terrablob_dir")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.model_package.infer_llm_model_type")
    def test_create_model_package_with_model_name(
        self,
        mock_infer_llm_model_type,
        mock_list_terrablob_dir,
    ):
        mock_list_terrablob_dir.return_value = ["a", "b"]
        mock_infer_llm_model_type.return_value = LLMModelType.UNKNOWN

        packager = LLMTritonPackager()

        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = packager.create_model_package(
                temp_dir,
                "model",
            )

            with open(os.path.join(model_path, "config.pbtxt")) as f:
                self.assertEqual(f.read(), SAMPLE_CONFIG_PBTXT)

        files = sorted(
            [
                str(
                    PurePath(os.path.join(dirpath, file)).relative_to(model_path),
                )
                for dirpath, _, filenames in os.walk(model_path)
                for file in filenames
            ],
        )
        self.assertEqual(files, ["0/download.yaml", "0/model.py", "0/user_model.py", "config.pbtxt"])

    def test_create_model_package_with_source_type_local_llm_type_default(self):
        packager = LLMTritonPackager()

        with tempfile.TemporaryDirectory() as temp_dir:
            with open(os.path.join(temp_dir, "model.py"), "w") as f:
                f.write("content")

            model_path = packager.create_model_package(
                temp_dir,
                "test_model",
                model_path_source_type=StorageType.LOCAL,
            )

        files = sorted(
            [
                str(
                    PurePath(os.path.join(dirpath, file)).relative_to(model_path),
                )
                for dirpath, _, filenames in os.walk(model_path)
                for file in filenames
            ],
        )
        self.assertEqual(files, ["0/download.yaml", "0/model.py", "0/user_model.py", "config.pbtxt"])

        gen = TritonTemplateRenderer()
        with open(os.path.join(model_path, "0", "user_model.py")) as f:
            self.assertEqual(f.read(), gen.render("vllm/user_model.py.tmpl"))

    def test_create_model_package_with_source_type_local_llm_type_t5(self):
        packager = LLMTritonPackager()

        with tempfile.TemporaryDirectory() as temp_dir:
            with open(os.path.join(temp_dir, "config.json"), "w") as f:
                json.dump({"architectures": ["T5ForConditionalGeneration"]}, f)

            model_path = packager.create_model_package(
                temp_dir,
                "test_model",
                model_path_source_type=StorageType.LOCAL,
            )

        files = sorted(
            [
                str(
                    PurePath(os.path.join(dirpath, file)).relative_to(model_path),
                )
                for dirpath, _, filenames in os.walk(model_path)
                for file in filenames
            ],
        )
        self.assertEqual(files, ["0/download.yaml", "0/model.py", "0/user_model.py", "config.pbtxt"])

        gen = TritonTemplateRenderer()
        with open(os.path.join(model_path, "0", "user_model.py")) as f:
            self.assertEqual(f.read(), gen.render("t5/user_model.py.tmpl"))

    def test_create_raw_model_package(self):
        packager = LLMTritonPackager()

        with tempfile.TemporaryDirectory() as temp_dir:
            with open(os.path.join(temp_dir, "model.py"), "w") as f:
                f.write("content")

            dest_model_path = packager.create_raw_model_package(
                temp_dir,
                model_path_source_type=StorageType.LOCAL,
            )

        files = sorted(
            [
                str(
                    PurePath(os.path.join(dirpath, file)).relative_to(dest_model_path),
                )
                for dirpath, _, filenames in os.walk(dest_model_path)
                for file in filenames
            ],
        )

        self.assertEqual(files, ["metadata/type.yaml", "model/model.py"])
        with open(os.path.join(dest_model_path, "metadata", "type.yaml")) as f:
            self.assertEqual(f.read(), "type: huggingface\n")
