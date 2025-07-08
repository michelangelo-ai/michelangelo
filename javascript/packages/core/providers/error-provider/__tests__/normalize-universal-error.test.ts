import { GrpcStatusCode } from '#core/constants/grpc-status-codes';
import { normalizeUniversalError } from '../normalize-universal-error';

describe('normalizeUniversalError', () => {
  it('should handle null/undefined errors', () => {
    expect(normalizeUniversalError(null)).toEqual({
      message: 'Unknown error occurred',
      code: GrpcStatusCode.UNKNOWN,
      source: 'unknown',
    });

    expect(normalizeUniversalError(undefined)).toEqual({
      message: 'Unknown error occurred',
      code: GrpcStatusCode.UNKNOWN,
      source: 'unknown',
    });
  });

  it('should handle native JavaScript errors', () => {
    const nativeError = new Error('Native error message');

    expect(normalizeUniversalError(nativeError)).toEqual({
      message: 'Native error message',
      code: GrpcStatusCode.UNKNOWN,
      cause: nativeError,
      source: 'javascript',
    });
  });

  it('should handle string errors', () => {
    expect(normalizeUniversalError('Something went wrong')).toEqual({
      message: 'Something went wrong',
      code: GrpcStatusCode.UNKNOWN,
      source: 'string',
    });
  });

  it('should handle generic objects with message property', () => {
    const errorObj = { message: 'Object error message', code: 500 };

    expect(normalizeUniversalError(errorObj)).toEqual({
      message: 'Object error message',
      code: GrpcStatusCode.UNKNOWN,
      cause: errorObj,
      source: 'unknown',
    });
  });

  it('should handle objects without message property', () => {
    const errorObj = { code: 500, data: 'some data' };

    expect(normalizeUniversalError(errorObj)).toEqual({
      message: 'Unknown error occurred',
      code: GrpcStatusCode.UNKNOWN,
      meta: { originalError: errorObj },
      source: 'unknown',
    });
  });

  it('should handle primitive values', () => {
    expect(normalizeUniversalError(42)).toEqual({
      message: 'Unknown error occurred',
      code: GrpcStatusCode.UNKNOWN,
      meta: { originalError: 42 },
      source: 'unknown',
    });

    expect(normalizeUniversalError(true)).toEqual({
      message: 'Unknown error occurred',
      code: GrpcStatusCode.UNKNOWN,
      meta: { originalError: true },
      source: 'unknown',
    });
  });

  it('should handle empty string', () => {
    expect(normalizeUniversalError('')).toEqual({
      message: '',
      code: GrpcStatusCode.UNKNOWN,
      source: 'string',
    });
  });
});
