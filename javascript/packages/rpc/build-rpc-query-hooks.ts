import { useMemo } from 'react';
import {
  useQuery as useTanstackQuery,
  UseQueryOptions,
  UseQueryResult,
} from '@tanstack/react-query';

import { useRpcProvider } from '#rpc/providers/rpc-provider/use-rpc-provider';
import { extractEntityFromResponse } from '#rpc/transformations/common';
import { isSingularResponse } from '#rpc/transformations/guards';
import { RpcHandlers } from './handlers';

import type { ExtractEntityFromResponse } from '#rpc/transformations/types';
import type { RpcRequest, RpcResponse } from './types';

/**
 * @description
 * Builds a set of Tanstack Query-wrapped hooks for a given set of RPC handlers.
 *
 * @remarks
 * Leverages {@link useRpcProvider} to access the available RPC handlers. If the RPC ID is not registered,
 * an error will be thrown when the hook is called.
 *
 * @returns
 * A set of query hooks for the given RPC handler.
 */
export function buildRPCQueryHooks<
  TRpcHandlers extends Record<string, (args: unknown) => Promise<unknown>> = ReturnType<
    typeof RpcHandlers
  >,
>(): {
  // This explicit specification of the return type is necessary because the `useQuery` hook
  // is generic and cannot be inferred.
  useQuery: <
    RpcId extends string,
    TData = RpcId extends keyof TRpcHandlers ? RpcResponse<TRpcHandlers, RpcId> : unknown,
    TTransformedData = ExtractEntityFromResponse<TData>,
  >(
    rpcId: RpcId,
    args: RpcId extends keyof TRpcHandlers ? RpcRequest<TRpcHandlers, RpcId> : unknown,
    options?: Partial<
      Omit<UseQueryOptions<TData, Error, TTransformedData>, 'queryKey' | 'queryFn' | 'select'>
    >
  ) => UseQueryResult<TTransformedData>;
} {
  const useRpcQueryKey = <RpcId extends string>(
    rpcId: RpcId,
    args: RpcId extends keyof TRpcHandlers ? RpcRequest<TRpcHandlers, RpcId> : unknown
  ) => {
    return useMemo(() => [rpcId, args] as const, [rpcId, args]);
  };

  const useQuery = <
    RpcId extends string,
    TData = RpcId extends keyof TRpcHandlers ? RpcResponse<TRpcHandlers, RpcId> : unknown,
    TTransformedData = ExtractEntityFromResponse<TData>,
  >(
    rpcId: RpcId,
    args: RpcId extends keyof TRpcHandlers ? RpcRequest<TRpcHandlers, RpcId> : unknown,
    options?: Partial<
      Omit<UseQueryOptions<TData, Error, TTransformedData>, 'queryKey' | 'queryFn' | 'select'>
    >
  ): UseQueryResult<TTransformedData> => {
    const rpcProvider = useRpcProvider();
    const rpcHandler = rpcProvider[String(rpcId)];
    if (!rpcHandler) {
      throw new Error(`RPC ID "${String(rpcId)}" not registered in RpcProvider`);
    }

    const queryKey = useRpcQueryKey(rpcId, args);
    return useTanstackQuery({
      queryKey,
      queryFn: () => rpcHandler(args) as Promise<TData>,
      ...options,
      select: (data: TData) => {
        if (isSingularResponse(data)) {
          return extractEntityFromResponse(data) as TTransformedData;
        }
        return data as unknown as TTransformedData;
      },
    });
  };

  return { useQuery };
}
