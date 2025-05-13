from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.constants import LLMModelType
from uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils import get_llm_model_type_from_pretrained_model_name

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


class LLMModelTypeTest(TestCase):
    def test_get_llm_model_type_from_pretrained_model_name(self):
        for model in LLAMA2_MODELS:
            llm_model_type = get_llm_model_type_from_pretrained_model_name(model)
            self.assertEqual(llm_model_type, LLMModelType.LLAMA_FAMILY)

            llm_model_type = get_llm_model_type_from_pretrained_model_name(f"meta-llama/{model}")
            self.assertEqual(llm_model_type, LLMModelType.LLAMA_FAMILY)

        for model in T5_MODELS:
            llm_model_type = get_llm_model_type_from_pretrained_model_name(model)
            self.assertEqual(llm_model_type, LLMModelType.T5)

            llm_model_type = get_llm_model_type_from_pretrained_model_name(f"google/{model}")
            self.assertEqual(llm_model_type, LLMModelType.T5)

        for model in MIXTRAL_MODELS:
            llm_model_type = get_llm_model_type_from_pretrained_model_name(model)
            self.assertEqual(llm_model_type, LLMModelType.MIXTRAL)

            llm_model_type = get_llm_model_type_from_pretrained_model_name(f"mistralai/{model}")
            self.assertEqual(llm_model_type, LLMModelType.MIXTRAL)

        llm_model_type = get_llm_model_type_from_pretrained_model_name("unknown")
        self.assertEqual(llm_model_type, LLMModelType.UNKNOWN)

        llm_model_type = get_llm_model_type_from_pretrained_model_name(None)
        self.assertEqual(llm_model_type, LLMModelType.UNKNOWN)
