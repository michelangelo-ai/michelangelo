import { GrpcStatusCode } from '@uber/michelangelo-core';

import type { ConnectError } from '@connectrpc/connect';
import type { ApplicationError, ErrorNormalizer } from '@uber/michelangelo-core';

/**
 * Normalizes Connect RPC errors to ApplicationError format
 *
 * @param error - The error to normalize
 * @returns ApplicationError if it's a Connect error, null otherwise
 *
 * @example
 * ```ts
 * // Usage in error provider
 * const errorProvider = (
 *   <ErrorProvider normalizeError={normalizeConnectError}>
 *     {children}
 *   </ErrorProvider>
 * );
 * ```
 */
export const normalizeConnectError: ErrorNormalizer = (
  error: ConnectError
): ApplicationError | null => {
  return {
    message: error.message,
    code: mapConnectCodeToGrpc(error.code),
    source: 'connect-rpc',
    meta: {
      connectErrorName: error.name,
      details: error.details,
      metadata: error.metadata,
    },
    cause: error.cause ?? error,
  };
};

/**
 * Maps Connect RPC status codes to gRPC status codes
 * Connect uses the same numeric codes as gRPC
 */
function mapConnectCodeToGrpc(code: number): GrpcStatusCode {
  // Connect uses the same numeric codes as gRPC, so we can map directly
  switch (code) {
    case 0:
      return GrpcStatusCode.OK;
    case 1:
      return GrpcStatusCode.CANCELLED;
    case 2:
      return GrpcStatusCode.UNKNOWN;
    case 3:
      return GrpcStatusCode.INVALID_ARGUMENT;
    case 4:
      return GrpcStatusCode.DEADLINE_EXCEEDED;
    case 5:
      return GrpcStatusCode.NOT_FOUND;
    case 6:
      return GrpcStatusCode.ALREADY_EXISTS;
    case 7:
      return GrpcStatusCode.PERMISSION_DENIED;
    case 8:
      return GrpcStatusCode.RESOURCE_EXHAUSTED;
    case 9:
      return GrpcStatusCode.FAILED_PRECONDITION;
    case 10:
      return GrpcStatusCode.ABORTED;
    case 11:
      return GrpcStatusCode.OUT_OF_RANGE;
    case 12:
      return GrpcStatusCode.UNIMPLEMENTED;
    case 13:
      return GrpcStatusCode.INTERNAL;
    case 14:
      return GrpcStatusCode.UNAVAILABLE;
    case 15:
      return GrpcStatusCode.DATA_LOSS;
    case 16:
      return GrpcStatusCode.UNAUTHENTICATED;
    default:
      return GrpcStatusCode.UNKNOWN;
  }
}
