import React from 'react';
import { Interceptor } from '@connectrpc/connect';
import { createGrpcWebTransport } from '@connectrpc/connect-web';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { RpcHandlers } from '#rpc/handlers';
import { RpcContext } from './rpc-context';

// This interceptor is used to set the headers for the RPC request to
// be compatible with the Michelangelo API yarpc server.
const callerInterceptor: Interceptor = (next) => async (req) => {
  req.header.set('context-Ttl-Ms', '10000');
  req.header.set('grpc-timeout', '1000000m');
  req.header.set('Rpc-Caller', 'ma-studio');
  req.header.set('Rpc-Encoding', 'proto');
  req.header.set('Rpc-Service', 'ma-apiserver');

  return await next(req);
};

// This transport is used to connect to the Envoy proxy that proxies gRPC web requests
// to gRPC services.
const transport = createGrpcWebTransport({
  baseUrl: 'http://localhost:8081',
  interceptors: [callerInterceptor],
  useBinaryFormat: true,
});

const queryClient = new QueryClient();

/**
 * @description
 * Provides the RPC handlers to the application.
 *
 * @remarks
 * Leverages {@link RpcHandlers} to create the RPC handlers.
 *
 * @example
 * ```tsx
 * <RpcProvider>
 *   <App />
 * </RpcProvider>
 * ```
 */
export const RpcProvider = ({ children }: { children: React.ReactNode }) => {
  const handlers = RpcHandlers(transport);

  return (
    <QueryClientProvider client={queryClient}>
      <RpcContext.Provider value={handlers}>{children}</RpcContext.Provider>
    </QueryClientProvider>
  );
};
