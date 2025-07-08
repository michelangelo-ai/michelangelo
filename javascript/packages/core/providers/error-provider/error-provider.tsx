import React from 'react';

import { ErrorContext } from './error-context';

import type { ErrorContextValue } from './types';

/**
 * @description
 * Provider that provides the system for normalizing errors specific
 * to the application.
 *
 * @remarks
 * normalizeError is required, but {@link normalizeUniversalError} will be
 * used as a fallback if normalizeError does not return an {@link ApplicationError}.
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
