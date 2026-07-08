import { ApplicationError, GrpcStatusCode } from '@michelangelo-ai/core';

import { GrpcTranscoderError } from './grpc-transcoder-error';

import type { ErrorNormalizer } from '@michelangelo-ai/core';

export const normalizeTranscoderError: ErrorNormalizer = (
  error: unknown
): ApplicationError | null => {
  if (!(error instanceof GrpcTranscoderError)) {
    return null;
  }

  return new ApplicationError(error.message, mapGrpcCode(error.code), {
    source: 'grpc-transcoder',
    meta: {
      details: error.details,
    },
    cause: error,
  });
};

function mapGrpcCode(code: number): GrpcStatusCode {
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
