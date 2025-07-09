import React from 'react';

import { ErrorContext } from './error-context';

import type { ErrorContextValue } from './types';

/**
 * @description
 * Provider that provides the system for normalizing errors specific
 * to the application.
 *
 * **Error Normalization Flow:**
 * 1. `normalizeError` is called first with the raw error
 * 2. If `normalizeError` returns an `ApplicationError`, that result is used
 * 3. If `normalizeError` returns `null`, {@link normalizeUniversalError}
 *    handles common error types (JavaScript Error, strings, objects, etc.)
 * 4. Universal fallback ensures all errors are normalized to `ApplicationError`
 *
 * **Implementation Pattern:**
 * - Handle your specific error types and return `ApplicationError`
 * - Return `null` for unknown error types to enable universal fallback
 * - Never return `undefined` or throw errors in your normalizer
 *
 * @example
 * ```tsx
 * <ErrorProvider normalizeError={(error) => {
 *   if (error instanceof MyError) {
 *     return {
 *       message: error.message,
 *       code: error.statusCode,
 *       source: 'MyError',
 *     };
 *   }
 *
 *   return null; // Fallback to normalizeUniversalError
 * }}>
 *   <MyComponent />
 * </ErrorProvider>
 *
 * const applicationError = useApplicationError(myError);
 * console.log(applicationError);
 * // { message: myError.message, code: myError.statusCode, source: 'MyError' }
 * ```
 */
export function ErrorProvider({
  children,
  ...errorContext
}: {
  children: React.ReactNode;
} & ErrorContextValue) {
  return <ErrorContext.Provider value={errorContext}>{children}</ErrorContext.Provider>;
}
