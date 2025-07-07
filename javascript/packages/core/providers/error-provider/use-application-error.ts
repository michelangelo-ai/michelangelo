import { useContext } from 'react';

import { ErrorContext } from './error-context';
import { normalizeUniversalError } from './normalize-universal-error';

import type { ApplicationError } from '#core/types/error-types';

/**
 * Hook to normalize any error to ApplicationError. Must be used within
 * an {@link ErrorContext}.
 *
 * @param error - The error to normalize
 * @returns The normalized error or null if the error is null/undefined
 */
export function useApplicationError(error: unknown): ApplicationError | null {
  const context = useContext(ErrorContext);
  if (!context) {
    throw new Error('useErrorSystem must be used within an ErrorProvider');
  }
  const { normalizeError } = context;

  if (!error) return null;

  const normalizedError = normalizeError(error);
  return normalizedError ?? normalizeUniversalError(error);
}
