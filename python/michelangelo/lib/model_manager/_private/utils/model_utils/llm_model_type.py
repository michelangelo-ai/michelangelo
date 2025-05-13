from michelangelo.lib.model_manager._private.constants import LLMModelType

LLAMA2_MODELS = {
    "LLAMA-2-7b",
    "LLAMA-2-13b",
    "LLAMA-2-70b",
    "LLAMA-2-7b-hf",
    "LLAMA-2-13b-hf",
    "LLAMA-2-70b-hf",
    "LLAMA-2-7b-chat",
    "LLAMA-2-13b-chat",
    "LLAMA-2-70b-chat",
    "LLAMA-2-7b-chat-hf",
    "LLAMA-2-13b-chat-hf",
    "LLAMA-2-70b-chat-hf",
}

T5_MODELS = {
    "t5-base",
    "t5-small",
    "t5-large",
    "t5-3b",
    "t5-11b",
}

MIXTRAL_MODELS = {
    "Mixtral-8x7B-v0.1",
    "Mixtral-8x7B-Instruct-v0.1",
    "Mixtral-8x22B-v0.1",
    "Mixtral-8x22B-Instruct-v0.1",
}


def get_llm_model_type_from_pretrained_model_name(
    pretrained_model_name: str,
) -> LLMModelType:
    """
    Get the LLM model type from the pretrained model name

    Args:
        pretrained_model_name: the model id of a pretrained model
            hosted inside a model repo on huggingface

    Returns:
        The LLM model type
    """
    if not pretrained_model_name:
        return LLMModelType.UNKNOWN

    model_name = pretrained_model_name.split("/")[-1]

    if model_name in LLAMA2_MODELS:
        return LLMModelType.LLAMA_FAMILY

    if model_name in T5_MODELS:
        return LLMModelType.T5

    if model_name in MIXTRAL_MODELS:
        return LLMModelType.MIXTRAL

    return LLMModelType.UNKNOWN
