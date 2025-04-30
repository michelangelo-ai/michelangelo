import { Message } from '@bufbuild/protobuf';
import { UseQueryResult } from '@tanstack/react-query';

import { RpcHandlers } from './handlers';

/**
 * @description
 * Picks the request type from the RPC handler.
 *
 * @remarks
 * Expects the request type to be the first parameter of the RPC handler.
 *
 * Leverages `OmitTypeName` to remove the `$typeName` and `$unknown` properties from the request type.
 * This is necessary because the protobuf-es library adds these properties to the request type.
 *
 * @example
 * ```ts
 * type MyRequest = RpcRequest<{ myRpc: (args: { myField: string }) => Promise<void> }, 'myRpc'>;
 * const myRequest: MyRequest = { myField: 'hello' };
 * ```
 */
export type RpcRequest<
  TRpcHandlers extends Record<string, (args: unknown) => Promise<unknown>>,
  RpcId extends keyof TRpcHandlers,
> = OmitTypeName<Parameters<TRpcHandlers[RpcId]>[0]>;

/**
 * @description
 * Picks the response type from the RPC handler.
 *
 * @remarks
 * Expects the response type to be wrapped in a `Promise`.
 *
 * @example
 * ```ts
 * type MyResponse = RpcResponse<{ myRpc: (args: any) => Promise<{ myField: string }> }, 'myRpc'>;
 * const myResponse: MyResponse = { myField: 'hello' };
 * ```
 */
export type RpcResponse<
  TRpcHandlers extends Record<string, (args: unknown) => Promise<unknown>>,
  RpcId extends keyof TRpcHandlers,
> = Awaited<ReturnType<TRpcHandlers[RpcId]>>;

export type BuildRPCQueryHooksReturn<
  TRpcHandlers extends Record<string, (args: unknown) => Promise<unknown>>,
> = {
  useQuery: <
    RpcId extends string,
    TData = RpcId extends keyof TRpcHandlers ? RpcResponse<TRpcHandlers, RpcId> : unknown,
  >(
    rpcId: RpcId,
    args: RpcId extends keyof TRpcHandlers ? RpcRequest<TRpcHandlers, RpcId> : unknown
  ) => UseQueryResult<TData>;
};

/**
 * @see {@link RpcHandlers}
 */
export type RpcHandlerType = ReturnType<typeof RpcHandlers>;

/**
 * @description
 * Removes the `$typeName` and `$unknown` properties from a message. These are properties
 * that are added by the protobuf-es library. We don't need them for our RPC calls.
 *
 * @example
 * ```ts
 * type MyMessage = {
 *   $typeName: string;
 *   $unknown: unknown;
 *   myField: string;
 * };
 *
 * type MyMessageWithoutTypeName = OmitTypeName<MyMessage>;
 * const message: MyMessageWithoutTypeName = { myField: 'hello' };
 * ```
 *
 * @see https://github.com/bufbuild/protobuf-es/issues/1016
 */
type OmitTypeName<T> = {
  [P in keyof T as P extends '$typeName' | '$unknown' ? never : P]: Recurse<T[P]>;
};

type Recurse<F> = F extends (infer U)[]
  ? Recurse<U>[]
  : F extends Message
    ? OmitTypeName<F>
    : F extends { case: infer C extends string; value: infer V extends Message }
      ? { case: C; value: OmitTypeName<V> }
      : F extends Record<string, infer V extends Message>
        ? Record<string, OmitTypeName<V>>
        : F extends Record<number, infer V extends Message>
          ? Record<number, OmitTypeName<V>>
          : F;
