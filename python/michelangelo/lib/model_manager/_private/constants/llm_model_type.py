from enum import Enum


class LLMModelType(Enum):
    """
    Types of LLM models
    """

    INVALID = 0
    UNKNOWN = 1
    LLAMA_FAMILY = 2  # Type for all models in the LLAMA family, including LLAMA-2 and LLAMA-3
    T5 = 3  # Type for all models in the T5 family
    MIXTRAL = 4  # Type for all models in the Mixtral family
