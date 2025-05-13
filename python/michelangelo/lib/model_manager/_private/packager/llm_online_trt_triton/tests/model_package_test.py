from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.sdk.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from uber.ai.michelangelo.sdk.model_manager._private.packager.llm_online_trt_triton import generate_model_package_content


class ModelPackageTest(TestCase):
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.list.list_terrablob_dir")
    def test_generate_model_package_content(
        self,
        mock_list_terrablob_dir,
    ):
        mock_list_terrablob_dir.return_value = ["a", "b"]

        model_path = "root"
        model_name = "model"
        model_revision = "0"

        gen = TritonTemplateRenderer()

        def iter_dict(d):
            for k, v in d.items():
                if isinstance(v, dict):
                    yield from iter_dict(v)
                else:
                    yield k, v

        content = generate_model_package_content(
            gen,
            model_path,
            model_name,
            model_revision,
        )

        for _, v in iter_dict(content):
            self.assertIsNotNone(v)
