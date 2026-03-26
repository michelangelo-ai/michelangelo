import { getRpcHandlers } from './handlers';

import type { OmitTypeName, RpcHandlerType } from './types';

/**
 * Makes a gRPC-web request to the Michelangelo API.
 *
 * Responses are decoded into plain objects — protobuf internals ($typeName, $unknown)
 * are stripped recursively.
 *
 * @param rpcId - The ID of the RPC handler to call.
 * @param args - The arguments to pass to the RPC handler.
 * @returns A promise that resolves to the RPC response as a plain object.
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
  args: OmitTypeName<Parameters<RpcHandlerType[RpcId]>[0]>
): Promise<OmitTypeName<Awaited<ReturnType<RpcHandlerType[RpcId]>>>> {
  const handlers = await getRpcHandlers();
  const response = (await handlers[rpcId](args)) as Awaited<ReturnType<RpcHandlerType[RpcId]>>;
  return toPlainObject(response) as OmitTypeName<Awaited<ReturnType<RpcHandlerType[RpcId]>>>;
}

// Recursively strips protobuf internals ($typeName, $unknown) from a response,
// returning a plain object tree that is safe to spread and reconstruct into new requests.
function toPlainObject(value: unknown): unknown {
  if (value === null || typeof value !== 'object') return value;
  if (value instanceof Uint8Array) return value; // preserve bytes fields (e.g. google.protobuf.Any.value)
  if (Array.isArray(value)) return value.map(toPlainObject);

  const result: Record<string, unknown> = {};
  for (const [key, val] of Object.entries(value as Record<string, unknown>)) {
    if (key === '$typeName' || key === '$unknown') continue;
    result[key] = toPlainObject(val);
  }
  return result;
}
