import { useContext } from 'react';

import { ErrorNormalizer } from '#core/types/error-types';
import { ErrorContext } from './error-context';
import { normalizeUniversalError } from './normalize-universal-error';

/**
 * Returns a function that normalizes any error to ApplicationError format for
 * consistent error handling across the application.
 *
 * The normalizer attempts to convert errors using these strategies in order:
 * 1. Custom error normalizer from ErrorProvider context (e.g., for gRPC/Connect errors)
 * 2. Universal error normalizer as fallback (handles standard Error objects)
 *
 * This ensures all errors are consistently structured with message, code, and metadata
 * regardless of their source (RPC calls, JavaScript errors, network errors, etc.).
 *
 * @returns Error normalizer function that converts any error to ApplicationError or null
 *   if the error is falsy. Returns null if the error cannot be normalized.
 *
 * @throws Will throw if used outside of an ErrorProvider
 *
 * @example
 * ```typescript
 * function MyComponent() {
 *   const normalizeError = useErrorNormalizer();
 *
 *   const handleRequest = async () => {
 *     try {
 *       await riskyOperation();
 *     } catch (error) {
 *       const normalized = normalizeError(error);
 *       if (normalized) {
 *         console.error('Error code:', normalized.code);
 *         console.error('Error message:', normalized.message);
 *         // Access error metadata if available
 *         console.error('Metadata:', normalized.meta);
 *       }
 *     }
 *   };
 * }
 *
 * // Used internally by useStudioQuery for automatic error normalization
 * ```
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
