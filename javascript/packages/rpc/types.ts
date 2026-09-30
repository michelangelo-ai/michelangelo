import type {
  DescMessage,
  DescMethodUnary,
  DescService,
  JsonValue,
  MessageJsonType,
} from '@bufbuild/protobuf';
import type { DurationSchema, TimestampSchema } from '@bufbuild/protobuf/wkt';
import type { ClusterService } from './gen/michelangelo/api/v2/cluster_svc_pb';
import type { DeploymentService } from './gen/michelangelo/api/v2/deployment_svc_pb';
import type { InferenceServerService } from './gen/michelangelo/api/v2/inference_server_svc_pb';
import type { ModelFamilyService } from './gen/michelangelo/api/v2/model_family_svc_pb';
import type { ModelService } from './gen/michelangelo/api/v2/model_svc_pb';
import type { PipelineRunService } from './gen/michelangelo/api/v2/pipeline_run_svc_pb';
import type { PipelineService } from './gen/michelangelo/api/v2/pipeline_svc_pb';
import type { ProjectService } from './gen/michelangelo/api/v2/project_svc_pb';
import type { RevisionService } from './gen/michelangelo/api/v2/revision_svc_pb';
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
 * like Connect's `Client<T>`: one async function per unary RPC, taking and
 * returning the generated `FooJson` type for its messages.
 *
 * Timestamp/Duration fields are `{ seconds, nanos }` at runtime (convert-time-fields.ts)
 * but typed as strings. The generated `TimestampJson`/`DurationJson` are plain `string`
 * aliases, so this type can't remap them.
 */
export type ServiceClient<T extends DescService> = {
  [K in keyof T['method']]: T['method'][K] extends DescMethodUnary<infer I, infer O>
    ? (request: MessageJsonType<I>, headers?: Record<string, string>) => Promise<MessageJsonType<O>>
    : never;
};

export type Services = {
  ClusterService: ServiceClient<typeof ClusterService>;
  DeploymentService: ServiceClient<typeof DeploymentService>;
  InferenceServerService: ServiceClient<typeof InferenceServerService>;
  ProjectService: ServiceClient<typeof ProjectService>;
  PipelineService: ServiceClient<typeof PipelineService>;
  PipelineRunService: ServiceClient<typeof PipelineRunService>;
  TriggerRunService: ServiceClient<typeof TriggerRunService>;
  ModelService: ServiceClient<typeof ModelService>;
  ModelFamilyService: ServiceClient<typeof ModelFamilyService>;
  RevisionService: ServiceClient<typeof RevisionService>;
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
 * Called by `walkMessage` on each message-typed value it reaches. Returns the value to put in
 * its place; call `descend` to walk the message's own fields instead of replacing it.
 */
export type MessageVisitor = (
  desc: DescMessage,
  value: unknown,
  descend: (desc: DescMessage, value: unknown) => unknown
) => unknown;

/** Rewrites one Timestamp/Duration value; see `convert-time-fields.ts`. */
export type ConvertTime = (
  schema: typeof TimestampSchema | typeof DurationSchema,
  value: unknown
) => unknown;
