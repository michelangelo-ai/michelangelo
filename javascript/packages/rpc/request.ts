import { getRpcHandlers } from './handlers';

import type { OmitTypeName, RpcHandlerType } from './types';

/**
 * Makes a gRPC-web request to the Michelangelo API.
 *
 * Responses are plain proto3 JSON objects, exactly as Envoy's grpc_json_transcoder emits them.
 *
 * @param rpcId - The ID of the RPC handler to call.
 * @param args - The arguments to pass to the RPC handler.
 * @param headers - Optional HTTP headers to send with the request (e.g. user identity).
 * @returns A promise that resolves to the RPC response as a plain proto3 JSON object.
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
  args: OmitTypeName<Parameters<RpcHandlerType[RpcId]>[0]>,
  headers?: Record<string, string>
): Promise<OmitTypeName<Awaited<ReturnType<RpcHandlerType[RpcId]>>>> {
  const handlers = await getRpcHandlers();
  // cast: dynamic key lookup on handlers loses the specific RPC signature; we know RpcId is a valid
  // key with matching handler shape
  const handler = handlers[rpcId] as (a: unknown, h?: Record<string, string>) => Promise<unknown>;
  // cast: handler returns unknown via dynamic dispatch; RpcId determines the concrete return type
  return (await handler(args, headers)) as OmitTypeName<Awaited<ReturnType<RpcHandlerType[RpcId]>>>;
}
