import type { ErrorNormalizer } from '#core/types/error-types';

export interface ErrorContextValue {
  normalizeError: ErrorNormalizer;
}
