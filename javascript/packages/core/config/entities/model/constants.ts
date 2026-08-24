/**
 * Numeric values for the `ModelKind` enum on the `Model` proto (model.proto).
 *
 * The generated proto client decodes enum fields to their numeric discriminant, not the
 * string enum name — e.g. `spec.kind` arrives in the browser as `2`, not
 * `"MODEL_KIND_REGRESSION"`. `typeTextMap` lookups below must be keyed accordingly (see the
 * same convention in `DEPLOYMENT_STAGE` / `DEPLOYMENT_STAGE_CELL` in
 * `config/entities/deployment/shared.ts`).
 */
export const MODEL_KIND = {
  INVALID: 0,
  CUSTOM: 1,
  REGRESSION: 2,
  BINARY_CLASSIFICATION: 3,
  MULTICLASS_CLASSIFICATION: 4,
  CLUSTERING: 5,
  LLM_COMPLETION: 6,
  LLM_CHAT_COMPLETION: 7,
  LLM_EMBEDDING: 8,
} as const;

/**
 * Human-readable labels for every `ModelKind` enum value on the `Model` proto (9 values).
 */
export const MODEL_KIND_TEXT_MAP: Record<number, string> = {
  [MODEL_KIND.INVALID]: 'Unknown',
  [MODEL_KIND.CUSTOM]: 'Custom',
  [MODEL_KIND.REGRESSION]: 'Regression',
  [MODEL_KIND.BINARY_CLASSIFICATION]: 'Binary Classification',
  [MODEL_KIND.MULTICLASS_CLASSIFICATION]: 'Multi-class Classification',
  [MODEL_KIND.CLUSTERING]: 'Clustering',
  [MODEL_KIND.LLM_COMPLETION]: 'LLM Completion',
  [MODEL_KIND.LLM_CHAT_COMPLETION]: 'LLM Chat',
  [MODEL_KIND.LLM_EMBEDDING]: 'LLM Embedding',
};
