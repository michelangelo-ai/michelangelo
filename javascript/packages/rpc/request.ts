import { RPC_HANDLERS } from './handlers';
import { OmitTypeName, RpcHandlerType } from './types';

/**
 * Makes a gRPC-web request to the Michelangelo API.
 *
 * @param rpcId - The ID of the RPC handler to call.
 * @param args - The arguments to pass to the RPC handler.
 * @returns A promise that resolves to the RPC response.
 *
 * @example
 * ```ts
 * const response = await request('ListProject', { /* project list args *\/ });
 *
 * // response is of type ListProjectResponse
 * ```
 */
export function request<RpcId extends keyof RpcHandlerType>(
  rpcId: RpcId,
  args: OmitTypeName<Parameters<RpcHandlerType[RpcId]>[0]>
): ReturnType<RpcHandlerType[RpcId]> {
  return RPC_HANDLERS[rpcId](args) as ReturnType<RpcHandlerType[RpcId]>;
}
