from michelangelo.lib.model_manager.constants import ModelKind as ModelKindConst
from uber.gen.code_uber_internal.uberai.michelangelo.api.v2beta1.model_proto import (
    ModelKind,
    MODEL_KIND_CUSTOM,
    MODEL_KIND_REGRESSION,
    MODEL_KIND_BINARY_CLASSIFICATION,
    MODEL_KIND_MULTICLASS_CLASSIFICATION,
    MODEL_KIND_LLM_COMPLETION,
    MODEL_KIND_LLM_CHAT_COMPLETION,
    MODEL_KIND_LLM_EMBEDDING,
)


def convert_model_kind(model_kind: str) -> ModelKind:
    """
    Convert the model kind from the model schema to the proto model kind in Unified API.

    Args:
        model_kind: Model kind from the model schema.

    Returns:
        Proto model kind in Unified API.
    """
    mapping = {
        ModelKindConst.CUSTOM: MODEL_KIND_CUSTOM,
        ModelKindConst.REGRESSION: MODEL_KIND_REGRESSION,
        ModelKindConst.BINARY_CLASSIFICATION: MODEL_KIND_BINARY_CLASSIFICATION,
        ModelKindConst.MULTICLASS_CLASSIFICATION: MODEL_KIND_MULTICLASS_CLASSIFICATION,
        ModelKindConst.LLM_COMPLETION: MODEL_KIND_LLM_COMPLETION,
        ModelKindConst.LLM_CHAT_COMPLETION: MODEL_KIND_LLM_CHAT_COMPLETION,
        ModelKindConst.LLM_EMBEDDING: MODEL_KIND_LLM_EMBEDDING,
    }
    return mapping.get(model_kind, MODEL_KIND_CUSTOM)
