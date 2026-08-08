"""Model kind constants."""


class ModelKind:
    """Model kinds describing the prediction task a registered model performs.

    Attributes:
        CUSTOM: Model type not covered by the other kinds.
        REGRESSION: Regression model.
        BINARY_CLASSIFICATION: Binary classification model.
        MULTICLASS_CLASSIFICATION: Multiclass classification model.
        LLM_COMPLETION: LLM text-completion model.
        LLM_CHAT_COMPLETION: LLM chat-completion model.
        LLM_EMBEDDING: LLM embedding model.
    """

    CUSTOM = "custom"
    REGRESSION = "regression"
    BINARY_CLASSIFICATION = "binary-classification"
    MULTICLASS_CLASSIFICATION = "multiclass-classification"
    LLM_COMPLETION = "llm-completion"
    LLM_CHAT_COMPLETION = "llm-chat-completion"
    LLM_EMBEDDING = "llm-embedding"
