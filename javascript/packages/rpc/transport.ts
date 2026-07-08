import type { FetchTransport, FetchTransportOptions, GoogleRpcStatus } from './types';

function isGoogleRpcStatus(value: unknown): value is GoogleRpcStatus {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as GoogleRpcStatus).code === 'number' &&
    typeof (value as GoogleRpcStatus).message === 'string'
  );
}

/**
 * Error thrown by {@link createFetchTransport} when the transcoder responds
 * with a non-2xx status. Mirrors the fields of a `google.rpc.Status` message.
 */
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

// Headers required by the Michelangelo API yarpc server, previously set by a
// Connect interceptor. Envoy's grpc_json_transcoder has no equivalent hook,
// so every request carries them directly.
const STATIC_HEADERS: Record<string, string> = {
  'context-Ttl-Ms': '10000',
  'grpc-timeout': '1000000m',
  'Rpc-Caller': 'ma-studio',
  'Rpc-Service': 'ma-apiserver',
  // YARPC's gRPC transport requires this to decode the request body — it's
  // not inferred from content-type the way a native gRPC client would set it.
  'Rpc-Encoding': 'proto',
};

/**
 * Creates a thin fetch-based transport that replaces Connect. Envoy's
 * grpc_json_transcoder filter performs the JSON<->binary proto transcoding,
 * so this transport only needs to speak JSON over HTTP — it has no knowledge
 * of protobuf message shapes.
 */
export function createFetchTransport(options: FetchTransportOptions): FetchTransport {
  const baseUrl = options.baseUrl.replace(/\/+$/, '');
  const headers = { ...STATIC_HEADERS, ...options.headers };

  return {
    async callUnary(serviceName, methodName, request) {
      const response = await fetch(`${baseUrl}/${serviceName}/${methodName}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...headers },
        body: JSON.stringify(request),
      });

      const body: unknown = await response.json().catch(() => null);

      if (response.status < 200 || response.status >= 300) {
        throw toTranscoderError(response, body);
      }

      return body;
    },
  };
}

function toTranscoderError(response: Response, body: unknown): GrpcTranscoderError {
  if (isGoogleRpcStatus(body)) {
    return new GrpcTranscoderError(body.message, body.code, body.details ?? []);
  }
  // Not every gRPC error carries a google.rpc.Status body — application
  // errors raised via plain yarpcerrors (rather than a full Status proto)
  // leave the body empty. Envoy still surfaces the real code and message via
  // the grpc-status/grpc-message headers (convert_grpc_status: true), so fall
  // back to those before giving up and reporting UNKNOWN.
  const headerStatus = response.headers.get('grpc-status');
  const headerMessage = response.headers.get('grpc-message');
  if (headerStatus !== null) {
    return new GrpcTranscoderError(
      headerMessage ? safeDecode(headerMessage) : response.statusText,
      Number(headerStatus)
    );
  }
  // UNKNOWN — no google.rpc.Status body and no grpc-status header.
  return new GrpcTranscoderError(
    response.statusText || `Request failed with status ${response.status}`,
    2
  );
}

// grpc-message headers are percent-encoded per the gRPC HTTP/2 spec, but
// upstream servers don't always encode strictly — fall back to the raw
// value rather than throwing on a malformed sequence.
function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}
