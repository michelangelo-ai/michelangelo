import { ApplicationError, GrpcStatusCode } from '@michelangelo-ai/core';

import { GrpcTranscoderError } from './grpc-transcoder-error';

import type { ErrorNormalizer } from '@michelangelo-ai/core';

/**
 * Normalizes errors thrown by the fetch transport into ApplicationError format.
 *
 * @example
 * ```ts
 * <ErrorProvider normalizeError={normalizeTranscoderError}>
 *   {children}
 * </ErrorProvider>
 * ```
 */
export const normalizeTranscoderError: ErrorNormalizer = (
  error: unknown
): ApplicationError | null => {
  if (!(error instanceof GrpcTranscoderError)) {
    return null;
  }

  // cast: gRPC status codes 0-16 map directly to GrpcStatusCode enum values
  const code = (
    error.code in GrpcStatusCode ? error.code : GrpcStatusCode.UNKNOWN
  ) as GrpcStatusCode;

  return new ApplicationError(error.message, code, {
    source: 'grpc-transcoder',
    meta: {
      details: error.details,
    },
    cause: error,
  });
};
