from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager._private.constants import LLMModelType
from michelangelo.lib.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from michelangelo.lib.model_manager._private.packager.llm_triton import generate_model_package_content
from michelangelo.lib.model_manager._private.packager.llm_triton.tests.fixtures.sample_config_pbtxt import SAMPLE_CONFIG_PBTXT


class ModelPackageTest(TestCase):
    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.list.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.packager.llm_triton.model_package.infer_llm_model_type")
    def test_generate_model_package_content(
        self,
        mock_infer_llm_model_type,
        mock_list_terrablob_dir,
    ):
        gen = TritonTemplateRenderer()
        mock_list_terrablob_dir.return_value = ["a", "b"]
        mock_infer_llm_model_type.return_value = LLMModelType.UNKNOWN

        model_path = "root"
        model_name = "model"
        model_revision = "0"

        content = generate_model_package_content(gen, model_path, model_name, model_revision, "LLAMA-2-7b", None, StorageType.TERRABLOB)
        self.assertEqual(content["0"]["download.yaml"], "assets:\n- a: root/a\n- b: root/b\nsource_type: terrablob\nsource_prefix: root/\n")
        self.assertIsNotNone(content["0"]["model.py"])
        self.assertIsNotNone(content["0"]["user_model.py"])
        self.assertEqual(content["config.pbtxt"], SAMPLE_CONFIG_PBTXT)

        content = generate_model_package_content(gen, model_path, "model-0", None, "t5-small", None, StorageType.TERRABLOB)
        self.assertEqual(content["0"]["download.yaml"], "assets:\n- a: root/a\n- b: root/b\nsource_type: terrablob\nsource_prefix: root/\n")
        self.assertIsNotNone(content["0"]["model.py"])
        self.assertIsNotNone(content["0"]["user_model.py"])
        self.assertEqual(content["config.pbtxt"], SAMPLE_CONFIG_PBTXT)

        content = generate_model_package_content(gen, None, "model-0", None, None, None, StorageType.TERRABLOB)
        self.assertTrue("download.yaml" not in content["0"])
        self.assertIsNotNone(content["0"]["model.py"])
        self.assertIsNotNone(content["0"]["user_model.py"])
        self.assertEqual(content["config.pbtxt"], SAMPLE_CONFIG_PBTXT)
