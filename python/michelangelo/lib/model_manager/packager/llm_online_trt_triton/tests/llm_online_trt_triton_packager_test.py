import os
from unittest import TestCase
from unittest.mock import patch
from pathlib import Path
from michelangelo.lib.model_manager.packager.llm_online_trt_triton import LLMOnlineTRTTritonPackager


class LLMOnlineTRTTritonPackagerTest(TestCase):
    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.list.list_terrablob_dir")
    def test_llm_online_trt_triton_packager(self, mock_list_terrablob_dir):
        mock_list_terrablob_dir.return_value = ["a", "b"]

        packager = LLMOnlineTRTTritonPackager()
        model_path = packager.create_model_package("model_path")

        files = sorted(
            [
                str(
                    Path(os.path.join(dirpath, file)).relative_to(model_path),
                )
                for dirpath, _, filenames in os.walk(model_path)
                for file in filenames
            ],
        )

        self.assertEqual(
            files,
            [
                "download.yaml",
                "ensemble/config.pbtxt",
                "postprocessing/1/model.py",
                "postprocessing/config.pbtxt",
                "preprocessing/1/model.py",
                "preprocessing/config.pbtxt",
                "tensorrt_llm/config.pbtxt",
            ],
        )
