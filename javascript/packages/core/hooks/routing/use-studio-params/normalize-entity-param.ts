import { StudioParamsBase } from '#core/hooks/routing/use-studio-params/types';
import pluralize from 'pluralize';

const ALWAYS_SINGULAR_ENTITIES = [
  'model-performance',
  'feature-consistency',
  'model-excellence-score',
  'offline-feature-quality',
  'fairness-estimator',
  'data-quality',
  'chat',
];

export function normalizeEntityParam(params: Partial<StudioParamsBase>): Partial<StudioParamsBase> {
  const entity = params.entity;

  if (!entity || entity === '') {
    return params;
  }

  if (ALWAYS_SINGULAR_ENTITIES.some((e) => entity === e)) {
    return params;
  }

  return { ...params, entity: pluralize(entity) };
}
