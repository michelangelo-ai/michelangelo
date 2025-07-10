import { useContext } from 'react';

import { ErrorNormalizer } from '#core/types/error-types';
import { ErrorContext } from './error-context';
import { normalizeUniversalError } from './normalize-universal-error';

/**
 * Hook to normalize any error to ApplicationError. Must be used within
 * an {@link ErrorContext}. Falls back to {@link normalizeUniversalError}
 * if the error is not a {@link ApplicationError}.
 *
 * @returns The error normalizer function
 */
export function useErrorNormalizer(): ErrorNormalizer {
  const context = useContext(ErrorContext);
  if (!context) {
    throw new Error('useErrorNormalizer must be used within an ErrorProvider');
  }

  return (error: unknown) => {
    if (!error) return null;

    const normalizedError = context.normalizeError(error);
    return normalizedError ?? normalizeUniversalError(error);
  };
}
