from unittest import TestCase
from michelangelo.lib.model_manager._private.constants import LLMModelType


class LLMModelTypeTest(TestCase):
    def test_llm_model_type(self):
        for llm_model_type in LLMModelType:
            self.assertEqual(llm_model_type, LLMModelType(llm_model_type.value))
