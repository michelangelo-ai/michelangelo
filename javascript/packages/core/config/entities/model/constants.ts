/**
 * Mirrors the generated proto ModelKind enum (model.proto). Colocated here until core has
 * access to the shared generated package — swapping the import path is the only change needed
 * then, since usage sites reference `ModelKind.REGRESSION` etc.
 */
export const ModelKind = {
  INVALID: 'MODEL_KIND_INVALID',
  CUSTOM: 'MODEL_KIND_CUSTOM',
  REGRESSION: 'MODEL_KIND_REGRESSION',
  BINARY_CLASSIFICATION: 'MODEL_KIND_BINARY_CLASSIFICATION',
  MULTICLASS_CLASSIFICATION: 'MODEL_KIND_MULTICLASS_CLASSIFICATION',
  CLUSTERING: 'MODEL_KIND_CLUSTERING',
  LLM_COMPLETION: 'MODEL_KIND_LLM_COMPLETION',
  LLM_CHAT_COMPLETION: 'MODEL_KIND_LLM_CHAT_COMPLETION',
  LLM_EMBEDDING: 'MODEL_KIND_LLM_EMBEDDING',
} as const;

/**
 * Human-readable labels for every `ModelKind` enum value on the `Model` proto (9 values).
 */
export const MODEL_KIND_TEXT_MAP: Record<string, string> = {
  [ModelKind.INVALID]: 'Unknown',
  [ModelKind.CUSTOM]: 'Custom',
  [ModelKind.REGRESSION]: 'Regression',
  [ModelKind.BINARY_CLASSIFICATION]: 'Binary Classification',
  [ModelKind.MULTICLASS_CLASSIFICATION]: 'Multi-class Classification',
  [ModelKind.CLUSTERING]: 'Clustering',
  [ModelKind.LLM_COMPLETION]: 'LLM Completion',
  [ModelKind.LLM_CHAT_COMPLETION]: 'LLM Chat',
  [ModelKind.LLM_EMBEDDING]: 'LLM Embedding',
};
