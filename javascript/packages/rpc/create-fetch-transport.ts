import { toTranscoderError } from './grpc-transcoder-error';

import type { JsonValue } from '@bufbuild/protobuf';
import type { FetchTransport, FetchTransportOptions } from './types';

export function createFetchTransport(options: FetchTransportOptions): FetchTransport {
  const baseUrl = options.baseUrl.replace(/\/+$/, '');
  const headers = {
    'Content-Type': 'application/json',
    'context-Ttl-Ms': '10000',
    'grpc-timeout': '1000000m',
    'Rpc-Caller': 'ma-studio',
    'Rpc-Service': 'ma-apiserver',
    'Rpc-Encoding': 'proto',
    ...options.headers,
  };

  return {
    async callUnary(serviceName, methodName, request) {
      const response = await fetch(`${baseUrl}/${serviceName}/${methodName}`, {
        method: 'POST',
        headers,
        body: JSON.stringify(request),
      });

      // cast: response.json() returns Promise<any> per DOM types
      const body = (await response.json().catch(() => null)) as JsonValue | null;

      if (response.status < 200 || response.status >= 300) {
        throw toTranscoderError(response, body);
      }

      return body;
    },
  };
}
