import React from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { RPC_HANDLERS } from '#rpc/handlers';
import { RpcContext } from './rpc-context';

const queryClient = new QueryClient();

/**
 * @description
 * Provides the RPC handlers to the application.
 *
 * @remarks
 * Uses handlers defined in {@link RPC_HANDLERS}.
 *
 * @example
 * ```tsx
 * <RpcProvider>
 *   <App />
 * </RpcProvider>
 * ```
 */
export const RpcProvider = ({ children }: { children: React.ReactNode }) => {
  return (
    <QueryClientProvider client={queryClient}>
      <RpcContext.Provider value={RPC_HANDLERS}>{children}</RpcContext.Provider>
    </QueryClientProvider>
  );
};
