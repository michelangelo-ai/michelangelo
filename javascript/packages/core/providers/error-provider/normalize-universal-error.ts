import { GrpcStatusCode } from '#core/constants/grpc-status-codes';
import { safeStringify } from '#core/utils/string-utils';

import type { ApplicationError } from '#core/types/error-types';

/**
 * Normalizes any error to an {@link ApplicationError}: Michelangelo's standard
 * error format, modeled after gRPC status codes.
 *
 * @param error - The error to normalize
 * @returns The normalized error or null if the error is null/undefined
 *
 * @example
 * ```ts
 * const error = new Error('Something went wrong');
 * const normalizedError = normalizeUniversalError(error);
 * console.log(normalizedError);
 * // { message: 'Something went wrong', code: 2, source: 'javascript' }
 * ```
 */
export function normalizeUniversalError(error: unknown): ApplicationError | null {
  if (error === null || error === undefined) {
    return {
      message: 'Unknown error occurred',
      code: GrpcStatusCode.UNKNOWN,
      source: 'unknown',
    };
  }

  if (error instanceof Error) {
    return {
      message: error.message,
      code: GrpcStatusCode.UNKNOWN,
      cause: error,
      source: 'javascript',
    };
  }

  if (typeof error === 'string') {
    return {
      message: error,
      code: GrpcStatusCode.UNKNOWN,
      source: 'string',
    };
  }

  if (typeof error === 'object' && error !== null) {
    const errorObj = error as Record<string, unknown>;

    if (errorObj.message) {
      return {
        message: safeStringify(errorObj.message),
        code: GrpcStatusCode.UNKNOWN,
        cause: error,
        source: 'unknown',
      };
    }
  }

  return {
    message: 'Unknown error occurred',
    code: GrpcStatusCode.UNKNOWN,
    meta: { originalError: error },
    source: 'unknown',
  };
}
