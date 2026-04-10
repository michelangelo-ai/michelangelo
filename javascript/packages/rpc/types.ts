import type { Message } from '@bufbuild/protobuf';
import type { createClient } from '@connectrpc/connect';
import type { ModelService } from './gen/michelangelo/api/v2/model_svc_pb';
import type { PipelineRunService } from './gen/michelangelo/api/v2/pipeline_run_svc_pb';
import type { PipelineService } from './gen/michelangelo/api/v2/pipeline_svc_pb';
import type { ProjectService } from './gen/michelangelo/api/v2/project_svc_pb';
import type { TriggerRunService } from './gen/michelangelo/api/v2/trigger_run_svc_pb';
import type { getRpcHandlers } from './handlers';

export type Services = {
  ProjectService: ReturnType<typeof createClient<typeof ProjectService>>;
  PipelineService: ReturnType<typeof createClient<typeof PipelineService>>;
  PipelineRunService: ReturnType<typeof createClient<typeof PipelineRunService>>;
  TriggerRunService: ReturnType<typeof createClient<typeof TriggerRunService>>;
  ModelService: ReturnType<typeof createClient<typeof ModelService>>;
};

/**
 * @see {@link getRpcHandlers}
 */
export type RpcHandlerType = Awaited<ReturnType<typeof getRpcHandlers>>;

/**
 * @description
 * Extracts the unary-unary function type from the RPC handler type.
 *
 * @remarks
 * The Connect Client type generates a type that includes unary-unary, unary-server-streaming,
 * unary-client-streaming, and unary-bidi-streaming functions.  We want to extract the
 * unary-unary function type from the RPC handler type.
 *
 * @example
 * ```ts
 * getProject: (args: { projectId: string }) => Promise<Project> | AsyncIterable<Project>;
 * ExtractUnaryRpc<getProject>
 * // => (args: { projectId: string }) => Promise<Project>
 * ```
 */
export type ExtractUnaryRpc<T> = T extends (args: Record<string, unknown>) => Promise<infer R>
  ? (args: Record<string, unknown>) => Promise<R>
  : never;

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
