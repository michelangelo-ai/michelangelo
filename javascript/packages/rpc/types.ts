import { Message } from '@bufbuild/protobuf';

import { RPC_HANDLERS } from './handlers';

/**
 * @see {@link RPC_HANDLERS}
 */
export type RpcHandlerType = typeof RPC_HANDLERS;

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
export type OmitTypeName<T> = {
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
