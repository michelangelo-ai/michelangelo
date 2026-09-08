import { fromBinary, toJson } from '@bufbuild/protobuf';

import { TypedStructSchema } from './gen/michelangelo/api/typed_struct_pb';
import { getRpcHandlers } from './handlers';

import type { Any } from '@bufbuild/protobuf/wkt';
import type { OmitTypeName, RpcHandlerType } from './types';

/**
 * Makes a gRPC-web request to the Michelangelo API.
 *
 * Responses are decoded into plain objects — protobuf internals ($typeName, $unknown)
 * are stripped recursively.
 *
 * @param rpcId - The ID of the RPC handler to call.
 * @param args - The arguments to pass to the RPC handler.
 * @param headers - Optional HTTP headers to send with the request (e.g. user identity).
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
  args: OmitTypeName<Parameters<RpcHandlerType[RpcId]>[0]>,
  headers?: Record<string, string>
): Promise<OmitTypeName<Awaited<ReturnType<RpcHandlerType[RpcId]>>>> {
  const handlers = await getRpcHandlers();
  // cast: dynamic key lookup on handlers loses the specific RPC signature; we know RpcId is a valid
  // key with matching handler shape
  const handler = handlers[rpcId] as (a: unknown, h?: Record<string, string>) => Promise<unknown>;
  // cast: handler returns unknown via dynamic dispatch; RpcId determines the concrete return type
  const response = (await handler(args, headers)) as Awaited<ReturnType<RpcHandlerType[RpcId]>>;
  // cast: toPlainObject returns unknown; RpcId determines the proto shape after stripping $typeName
  // fields
  return toPlainObject(response) as OmitTypeName<Awaited<ReturnType<RpcHandlerType[RpcId]>>>;
}

// Recursively strips protobuf internals ($typeName, $unknown) from a response,
// returning a plain object tree that is safe to spread and reconstruct into new requests.
function toPlainObject(value: unknown): unknown {
  if (value === null || typeof value !== 'object') return value;
  if (value instanceof Uint8Array) return value; // preserve bytes fields (e.g. google.protobuf.Any.value)
  if (Array.isArray(value)) return value.map(toPlainObject);

  const typedStruct = unpackTypedStructAny(value);
  if (typedStruct) return typedStruct;

  const result: Record<string, unknown> = {};
  for (const [key, val] of Object.entries(value)) {
    if (key === '$typeName' || key === '$unknown') continue;
    result[key] = toPlainObject(val);
  }
  return result;
}

const TYPED_STRUCT_TYPE_URL = `type.googleapis.com/${TypedStructSchema.typeName}`;

// google.protobuf.Any fields survive fromJson as { typeUrl, value: Uint8Array } — the binary
// payload is useless to consumers. When the payload is a michelangelo.api.TypedStruct
// (e.g. PipelineManifest.content), expand it to { typeUrl, value } where typeUrl names the
// inner config type and value is its plain JSON. Other Any payloads are left untouched.
function unpackTypedStructAny(value: object): unknown {
  if (!('$typeName' in value) || value.$typeName !== 'google.protobuf.Any') return undefined;
  // cast: the $typeName check above identifies this as a google.protobuf.Any message
  const any = value as Any;
  if (any.typeUrl !== TYPED_STRUCT_TYPE_URL) return undefined;
  return toJson(TypedStructSchema, fromBinary(TypedStructSchema, any.value));
}
