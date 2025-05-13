import os
import json
import tempfile
from collections.abc import Sequence
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager._private.constants import LLMModelType
from michelangelo.lib.model_manager._private.utils.model_utils import get_llm_model_type_from_pretrained_model_name
from michelangelo.lib.model_manager._private.packager.llm_triton.model_config import (
    download_model_config,
    CONFIG_FILE_NAME,
)


def infer_llm_model_type(
    model_path: str,
    pretrained_model_name: str,
    model_path_source_type: str = StorageType.TERRABLOB,
) -> LLMModelType:
    """
    Infer the LLM model type from the model path and the pretrained model name

    Args:
        model_path: The path of the model in Terrablob
        pretrained_model_name: The model id of a pretrained model hosted inside a model repo on huggingface

    Returns:
        The inferred LLM model type
    """
    model_type = get_llm_model_type_from_pretrained_model_name(pretrained_model_name)

    if model_type not in {LLMModelType.UNKNOWN, LLMModelType.INVALID}:
        return model_type

    with tempfile.TemporaryDirectory() as temp_dir:
        dest_file_path = os.path.join(temp_dir, CONFIG_FILE_NAME)
        config_file_path = download_model_config(
            model_path,
            dest_file_path=dest_file_path,
            model_path_source_type=model_path_source_type,
        )

        if not config_file_path:
            return model_type

        with open(config_file_path) as f:
            try:
                config = json.load(f)
            except json.JSONDecodeError:
                return model_type

            architectures = config.get("architectures", None)

            if not architectures or not isinstance(architectures, Sequence):
                return model_type

            if "LlamaForCausalLM" in architectures:
                return LLMModelType.LLAMA_FAMILY

            if "T5ForConditionalGeneration" in architectures:
                return LLMModelType.T5

            if "MixtralForCausalLM" in architectures:
                return LLMModelType.MIXTRAL

    return model_type
