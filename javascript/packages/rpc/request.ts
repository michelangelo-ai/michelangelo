import { getRpcHandlers } from './handlers';

import type { RpcHandlerType } from './types';

/**
 * Makes a gRPC-web request to the Michelangelo API.
 *
 * Responses are proto3 JSON as Envoy's grpc_json_transcoder emits it, except Timestamp/Duration
 * fields, which arrive as `{ seconds, nanos }`.
 *
 * @param rpcId - The ID of the RPC handler to call.
 * @param args - The arguments to pass to the RPC handler.
 * @param headers - Optional HTTP headers to send with the request (e.g. user identity).
 * @returns The RPC response.
 *
 * @example
 * ```ts
 * const response = await request('ListProject', { /* project list args *\/ });
 *
 * // response is of type ListProjectResponse
 * ```
 */
export async function request<RpcId extends keyof RpcHandlerType>(
  rpcId: RpcId,
  args: Parameters<RpcHandlerType[RpcId]>[0],
  headers?: Record<string, string>
): Promise<Awaited<ReturnType<RpcHandlerType[RpcId]>>> {
  const handlers = await getRpcHandlers();
  // cast: dynamic key lookup on handlers loses the specific RPC signature; we know RpcId is a valid
  // key with matching handler shape
  const handler = handlers[rpcId] as (a: unknown, h?: Record<string, string>) => Promise<unknown>;
  // cast: handler returns unknown via dynamic dispatch; RpcId determines the concrete return type
  return (await handler(args, headers)) as Awaited<ReturnType<RpcHandlerType[RpcId]>>;
}
