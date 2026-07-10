import { GrpcStatusCode } from '#core/constants/grpc-status-codes';
import { ApplicationError } from '#core/types/error-types';
import { isRecord } from '#core/utils/object-utils';
import { safeStringify } from '#core/utils/string-utils';

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
export function normalizeUniversalError(error: unknown): ApplicationError {
  if (error === null || error === undefined) {
    return new ApplicationError('Unknown error occurred', GrpcStatusCode.UNKNOWN, {
      source: 'unknown',
    });
  }

  if (error instanceof Error) {
    return new ApplicationError(error.message, GrpcStatusCode.UNKNOWN, {
      cause: error,
      source: 'javascript',
    });
  }

  if (typeof error === 'string') {
    return new ApplicationError(error, GrpcStatusCode.UNKNOWN, {
      source: 'string',
    });
  }

  if (isRecord(error)) {
    if (error.message) {
      return new ApplicationError(safeStringify(error.message), GrpcStatusCode.UNKNOWN, {
        cause: error,
        source: 'unknown',
      });
    }
  }

  return new ApplicationError('Unknown error occurred', GrpcStatusCode.UNKNOWN, {
    meta: { originalError: error },
    source: 'unknown',
  });
}
