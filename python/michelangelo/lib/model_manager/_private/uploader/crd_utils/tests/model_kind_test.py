from unittest import TestCase
from michelangelo.lib.model_manager.constants import ModelKind as ModelKindConst
from michelangelo.lib.model_manager._private.uploader.crd_utils import convert_model_kind
from uber.gen.code_uber_internal.uberai.michelangelo.api.v2beta1.model_proto import (
    MODEL_KIND_CUSTOM,
    MODEL_KIND_REGRESSION,
    MODEL_KIND_BINARY_CLASSIFICATION,
    MODEL_KIND_MULTICLASS_CLASSIFICATION,
    MODEL_KIND_LLM_COMPLETION,
    MODEL_KIND_LLM_CHAT_COMPLETION,
    MODEL_KIND_LLM_EMBEDDING,
)


class ModelKindTest(TestCase):
    def test_convert_model_kind(self):
        model_kind = convert_model_kind(ModelKindConst.CUSTOM)
        self.assertEqual(model_kind, MODEL_KIND_CUSTOM)

        model_kind = convert_model_kind(ModelKindConst.REGRESSION)
        self.assertEqual(model_kind, MODEL_KIND_REGRESSION)

        model_kind = convert_model_kind(ModelKindConst.BINARY_CLASSIFICATION)
        self.assertEqual(model_kind, MODEL_KIND_BINARY_CLASSIFICATION)

        model_kind = convert_model_kind(ModelKindConst.MULTICLASS_CLASSIFICATION)
        self.assertEqual(model_kind, MODEL_KIND_MULTICLASS_CLASSIFICATION)

        model_kind = convert_model_kind(ModelKindConst.LLM_COMPLETION)
        self.assertEqual(model_kind, MODEL_KIND_LLM_COMPLETION)

        model_kind = convert_model_kind(ModelKindConst.LLM_CHAT_COMPLETION)
        self.assertEqual(model_kind, MODEL_KIND_LLM_CHAT_COMPLETION)

        model_kind = convert_model_kind(ModelKindConst.LLM_EMBEDDING)
        self.assertEqual(model_kind, MODEL_KIND_LLM_EMBEDDING)

        model_kind = convert_model_kind("invalid")
        self.assertEqual(model_kind, MODEL_KIND_CUSTOM)
