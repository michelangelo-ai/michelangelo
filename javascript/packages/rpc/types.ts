import type {
  DescMethodUnary,
  DescService,
  JsonValue,
  Message,
  MessageInitShape,
  MessageShape,
} from '@bufbuild/protobuf';
import type { DeploymentService } from './gen/michelangelo/api/v2/deployment_svc_pb';
import type { InferenceServerService } from './gen/michelangelo/api/v2/inference_server_svc_pb';
import type { ModelService } from './gen/michelangelo/api/v2/model_svc_pb';
import type { PipelineRunService } from './gen/michelangelo/api/v2/pipeline_run_svc_pb';
import type { PipelineService } from './gen/michelangelo/api/v2/pipeline_svc_pb';
import type { ProjectService } from './gen/michelangelo/api/v2/project_svc_pb';
import type { TriggerRunService } from './gen/michelangelo/api/v2/trigger_run_svc_pb';
import type { getRpcHandlers } from './handlers';

export interface RuntimeConfig {
  apiBaseUrl: string;
}

/**
 * Shape of a `google.rpc.Status` error body, as produced by Envoy's
 * grpc_json_transcoder filter when a unary RPC returns a non-OK gRPC status.
 */
export interface GoogleRpcStatus {
  code: number;
  message: string;
  details?: unknown[];
}

export interface FetchTransportOptions {
  /** Base URL of the Envoy-fronted API server, e.g. `https://api.example.com`. */
  baseUrl: string;
  /** Additional headers to send with every request, merged over the static defaults. */
  headers?: Record<string, string>;
}

export interface FetchTransport {
  /**
   * Calls a unary RPC through Envoy's grpc_json_transcoder by POSTing JSON to
   * `/{serviceName}/{methodName}` and returning the parsed JSON response.
   */
  callUnary(
    serviceName: string,
    methodName: string,
    request: unknown,
    headers?: Record<string, string>
  ): Promise<JsonValue>;
}

/**
 * Maps a service's generated method descriptors to a client object shaped
 * like Connect's `Client<T>` — one async function per unary RPC.
 */
export type ServiceClient<T extends DescService> = {
  [K in keyof T['method']]: T['method'][K] extends DescMethodUnary<infer I, infer O>
    ? (request: MessageInitShape<I>, headers?: Record<string, string>) => Promise<MessageShape<O>>
    : never;
};

export type Services = {
  DeploymentService: ServiceClient<typeof DeploymentService>;
  InferenceServerService: ServiceClient<typeof InferenceServerService>;
  ProjectService: ServiceClient<typeof ProjectService>;
  PipelineService: ServiceClient<typeof PipelineService>;
  PipelineRunService: ServiceClient<typeof PipelineRunService>;
  TriggerRunService: ServiceClient<typeof TriggerRunService>;
  ModelService: ServiceClient<typeof ModelService>;
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
export type ExtractUnaryRpc<T> = T extends (
  args: Record<string, unknown>,
  headers?: Record<string, string>
) => Promise<infer R>
  ? (args: Record<string, unknown>, headers?: Record<string, string>) => Promise<R>
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
