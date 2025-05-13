import os
import json
import tempfile
from typing import Optional
from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.sdk.model_manager.constants import StorageType
from uber.ai.michelangelo.shared.errors.terrablob_error import TerrablobFileNotFoundError
from uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton import download_model_config


def download_model(
    model_path: str,  # noqa: ARG001
    dest_model_path: Optional[str] = None,
    model_path_source_type: Optional[str] = StorageType.HDFS,  # noqa: ARG001
    include: Optional[list[str]] = None,
) -> str:
    for sub_path in include:
        config_file = os.path.join(dest_model_path, sub_path)
        with open(config_file, "w") as f:
            json.dump({"architectures": ["LlamaForCausalLM"]}, f)


class ModelConfigTest(TestCase):
    def test_download_model_config(self):
        with patch(
            ("uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.model_config.download_model"),
            wraps=download_model,
        ) as mock_download_model:
            config_file = download_model_config("model_path")

            mock_download_model.assert_called_once()
            args, kwargs = mock_download_model.call_args
            self.assertEqual(args[0], "model_path")
            self.assertIn("/tmp", args[1])
            self.assertEqual(kwargs["model_path_source_type"], StorageType.TERRABLOB)
            self.assertEqual(kwargs["include"], ["config.json"])

            self.assertIn("config.json", config_file)

    def test_download_model_config_with_dest_file_path(self):
        with patch(
            ("uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.model_config.download_model"),
            wraps=download_model,
        ) as mock_download_model:
            with tempfile.NamedTemporaryFile() as fp:
                config_file = download_model_config("model_path", dest_file_path=fp.name)

                mock_download_model.assert_called_with(
                    "model_path",
                    os.path.dirname(fp.name),
                    model_path_source_type=StorageType.TERRABLOB,
                    include=["config.json"],
                )

                self.assertEqual(config_file, fp.name)

    def test_download_model_config_not_found(self):
        with patch(
            ("uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.model_config.download_model"),
            side_effect=TerrablobFileNotFoundError("error"),
        ):
            config_file = download_model_config("model_path")

            self.assertIsNone(config_file)
