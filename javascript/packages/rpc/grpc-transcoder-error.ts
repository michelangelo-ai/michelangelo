import type { GoogleRpcStatus } from './types';

export class GrpcTranscoderError extends Error {
  readonly code: number;
  readonly details: unknown[];

  constructor(message: string, code: number, details: unknown[] = []) {
    super(message);
    this.name = 'GrpcTranscoderError';
    this.code = code;
    this.details = details;
  }
}

export function toTranscoderError(response: Response, body: unknown): GrpcTranscoderError {
  if (isGoogleRpcStatus(body)) {
    return new GrpcTranscoderError(body.message, body.code, body.details ?? []);
  }
  // Plain YARPC errors leave the body empty. Envoy surfaces the real code
  // and message via grpc-status/grpc-message headers.
  const headerStatus = response.headers.get('grpc-status');
  const headerMessage = response.headers.get('grpc-message');
  if (headerStatus !== null) {
    return new GrpcTranscoderError(
      headerMessage ? safeDecode(headerMessage) : response.statusText,
      Number(headerStatus)
    );
  }
  return new GrpcTranscoderError(
    response.statusText || `Request failed with status ${response.status}`,
    2
  );
}

function isGoogleRpcStatus(value: unknown): value is GoogleRpcStatus {
  if (typeof value !== 'object' || value === null) return false;
  // cast: narrowing unknown to check shape
  const candidate = value as GoogleRpcStatus;
  return typeof candidate.code === 'number' && typeof candidate.message === 'string';
}

// grpc-message headers are percent-encoded per the gRPC HTTP/2 spec.
function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}
