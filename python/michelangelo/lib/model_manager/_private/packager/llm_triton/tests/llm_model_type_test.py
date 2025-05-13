import json
from typing import Optional
from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.sdk.model_manager._private.constants import LLMModelType
from uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton import infer_llm_model_type


class LLMModelTypeTest(TestCase):
    def test_infer_llm_model_type_with_pretrained_model_name(self):
        model_path = "model_path"
        pretrained_model_name = "t5-small"
        model_type = infer_llm_model_type(model_path, pretrained_model_name)
        self.assertEqual(model_type, LLMModelType.T5)

    def test_infer_llm_model_type_llama_family_with_unknown_pretrained_model_name(self):
        def download_model_config(
            model_path: str,  # noqa: ARG001
            dest_file_path: Optional[str] = None,
            model_path_source_type: Optional[str] = None,  # noqa: ARG001
        ):
            with open(dest_file_path, "w") as f:
                json.dump({"architectures": ["LlamaForCausalLM"]}, f)
            return dest_file_path

        with patch(
            ("uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.llm_model_type.download_model_config"),
            wraps=download_model_config,
        ) as mock_download_model_config:
            model_path = "model_path"
            pretrained_model_name = "unknown_model"
            model_type = infer_llm_model_type(model_path, pretrained_model_name)
            self.assertEqual(model_type, LLMModelType.LLAMA_FAMILY)

            args, kwargs = mock_download_model_config.call_args
            self.assertEqual(args[0], model_path)
            self.assertTrue(kwargs["dest_file_path"].endswith("config.json"))

    def test_infer_llm_model_type_t5_with_unknown_pretrained_model_name(self):
        def download_model_config(
            model_path: str,  # noqa: ARG001
            dest_file_path: Optional[str] = None,
            model_path_source_type: Optional[str] = None,  # noqa: ARG001
        ):
            with open(dest_file_path, "w") as f:
                json.dump({"architectures": ["T5ForConditionalGeneration"]}, f)
            return dest_file_path

        with patch(
            ("uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.llm_model_type.download_model_config"),
            wraps=download_model_config,
        ) as mock_download_model_config:
            model_path = "model_path"
            pretrained_model_name = "unknown_model"
            model_type = infer_llm_model_type(model_path, pretrained_model_name)
            self.assertEqual(model_type, LLMModelType.T5)

            args, kwargs = mock_download_model_config.call_args
            self.assertEqual(args[0], model_path)
            self.assertTrue(kwargs["dest_file_path"].endswith("config.json"))

    def test_infer_llm_model_type_mixtral_with_unknown_pretrained_model_name(self):
        def download_model_config(
            model_path: str,  # noqa: ARG001
            dest_file_path: Optional[str] = None,
            model_path_source_type: Optional[str] = None,  # noqa: ARG001
        ):
            with open(dest_file_path, "w") as f:
                json.dump({"architectures": ["MixtralForCausalLM"]}, f)
            return dest_file_path

        with patch(
            ("uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.llm_model_type.download_model_config"),
            wraps=download_model_config,
        ) as mock_download_model_config:
            model_path = "model_path"
            pretrained_model_name = "unknown_model"
            model_type = infer_llm_model_type(model_path, pretrained_model_name)
            self.assertEqual(model_type, LLMModelType.MIXTRAL)

            args, kwargs = mock_download_model_config.call_args
            self.assertEqual(args[0], model_path)
            self.assertTrue(kwargs["dest_file_path"].endswith("config.json"))

    def test_infer_llm_model_type_unknonw_architecture(self):
        def download_model_config(
            model_path: str,  # noqa: ARG001
            dest_file_path: Optional[str] = None,
            model_path_source_type: Optional[str] = None,  # noqa: ARG001
        ):
            with open(dest_file_path, "w") as f:
                json.dump({"architectures": ["Unkonwn"]}, f)
            return dest_file_path

        with patch(
            ("uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.llm_model_type.download_model_config"),
            wraps=download_model_config,
        ) as mock_download_model_config:
            model_path = "model_path"
            pretrained_model_name = "unknown_model"
            model_type = infer_llm_model_type(model_path, pretrained_model_name)
            self.assertEqual(model_type, LLMModelType.UNKNOWN)

            args, kwargs = mock_download_model_config.call_args
            self.assertEqual(args[0], model_path)
            self.assertTrue(kwargs["dest_file_path"].endswith("config.json"))

    @patch(
        "uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.llm_model_type.download_model_config",
        return_value=None,
    )
    def test_infer_llm_model_type_no_config_file(self, mock_download_model_config):
        model_path = "model_path"
        pretrained_model_name = "unknown_model"
        model_type = infer_llm_model_type(model_path, pretrained_model_name)
        self.assertEqual(model_type, LLMModelType.UNKNOWN)

        args, kwargs = mock_download_model_config.call_args
        self.assertEqual(args[0], model_path)
        self.assertTrue(kwargs["dest_file_path"].endswith("config.json"))

    def test_infer_llm_model_type_invalid_json(self):
        def download_model_config(
            model_path: str,  # noqa: ARG001
            dest_file_path: Optional[str] = None,
            model_path_source_type: Optional[str] = None,  # noqa: ARG001
        ):
            with open(dest_file_path, "w") as f:
                f.write("invalid_json")
            return dest_file_path

        with patch(
            ("uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.llm_model_type.download_model_config"),
            wraps=download_model_config,
        ) as mock_download_model_config:
            model_path = "model_path"
            pretrained_model_name = "unknown_model"
            model_type = infer_llm_model_type(model_path, pretrained_model_name)
            self.assertEqual(model_type, LLMModelType.UNKNOWN)

            args, kwargs = mock_download_model_config.call_args
            self.assertEqual(args[0], model_path)
            self.assertTrue(kwargs["dest_file_path"].endswith("config.json"))

    def test_infer_llm_model_type_no_architectures(self):
        def download_model_config(
            model_path: str,  # noqa: ARG001
            dest_file_path: Optional[str] = None,
            model_path_source_type: Optional[str] = None,  # noqa: ARG001
        ):
            with open(dest_file_path, "w") as f:
                json.dump({}, f)
            return dest_file_path

        with patch(
            ("uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.llm_model_type.download_model_config"),
            wraps=download_model_config,
        ) as mock_download_model_config:
            model_path = "model_path"
            pretrained_model_name = "unknown_model"
            model_type = infer_llm_model_type(model_path, pretrained_model_name)
            self.assertEqual(model_type, LLMModelType.UNKNOWN)

            args, kwargs = mock_download_model_config.call_args
            self.assertEqual(args[0], model_path)
            self.assertTrue(kwargs["dest_file_path"].endswith("config.json"))

    def test_infer_llm_model_type_not_sequence_architectures(self):
        def download_model_config(
            model_path: str,  # noqa: ARG001
            dest_file_path: Optional[str] = None,
            model_path_source_type: Optional[str] = None,  # noqa: ARG001
        ):
            with open(dest_file_path, "w") as f:
                json.dump({"architectures": 1}, f)
            return dest_file_path

        with patch(
            ("uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.llm_model_type.download_model_config"),
            wraps=download_model_config,
        ) as mock_download_model_config:
            model_path = "model_path"
            pretrained_model_name = "unknown_model"
            model_type = infer_llm_model_type(model_path, pretrained_model_name)
            self.assertEqual(model_type, LLMModelType.UNKNOWN)

            args, kwargs = mock_download_model_config.call_args
            self.assertEqual(args[0], model_path)
            self.assertTrue(kwargs["dest_file_path"].endswith("config.json"))
