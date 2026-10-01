import { createRegistry, fromJson, toJson } from '@bufbuild/protobuf';
import {
  BoolValueSchema,
  DoubleValueSchema,
  Int64ValueSchema,
  StringValueSchema,
} from '@bufbuild/protobuf/wkt';

import { timesToObjects, timesToStrings } from './convert-time-fields';
import { createFetchTransport } from './create-fetch-transport';
import { TypedStructSchema } from './gen/michelangelo/api/typed_struct_pb';
import { ClusterService } from './gen/michelangelo/api/v2/cluster_svc_pb';
import { DeploymentService } from './gen/michelangelo/api/v2/deployment_svc_pb';
import { InferenceServerService } from './gen/michelangelo/api/v2/inference_server_svc_pb';
import { ModelFamilyService } from './gen/michelangelo/api/v2/model_family_svc_pb';
import { ModelService } from './gen/michelangelo/api/v2/model_svc_pb';
import { PipelineSchema } from './gen/michelangelo/api/v2/pipeline_pb';
import { PipelineRunService } from './gen/michelangelo/api/v2/pipeline_run_svc_pb';
import { PipelineService } from './gen/michelangelo/api/v2/pipeline_svc_pb';
import { ProjectService } from './gen/michelangelo/api/v2/project_svc_pb';
import { RevisionService } from './gen/michelangelo/api/v2/revision_svc_pb';
import { TriggerRunService } from './gen/michelangelo/api/v2/trigger_run_svc_pb';
import { omitDeletionGracePeriod } from './omit-deletion-grace-period';
import { packAnyFields } from './pack-any-fields';
import { getRuntimeConfig } from './runtime-config';

import type { DescService, JsonValue } from '@bufbuild/protobuf';
import type { FetchTransport, ServiceClient, Services } from './types';

// Every message type that can appear inside a google.protobuf.Any must be registered here:
// requests with an unregistered `@type` fail (fromJson/toJson throw), and time fields inside
// an unregistered payload aren't converted. The wrapper types cover packed ListOptionsExt
// criteria; PipelineSchema covers Revision.spec.content for Pipeline revisions.
export const typeRegistry = createRegistry(
  TypedStructSchema,
  PipelineSchema,
  StringValueSchema,
  BoolValueSchema,
  Int64ValueSchema,
  DoubleValueSchema
);

/**
 * Builds a service client that sends and receives proto3 JSON. Responses are returned as
 * Envoy's grpc_json_transcoder emits them, default values included. Timestamp/Duration fields
 * are the exception in both directions: callers see `{ seconds, nanos }` instead of proto3
 * JSON strings (convert-time-fields.ts).
 */
function createServiceClient<T extends DescService>(
  service: T,
  transport: FetchTransport
): ServiceClient<T> {
  const client: Record<
    string,
    (request: Record<string, unknown>, headers?: Record<string, string>) => Promise<unknown>
  > = {};

  for (const method of service.methods) {
    if (method.methodKind !== 'unary') continue;

    client[method.localName] = async (request, headers) => {
      const withoutGracePeriod = omitDeletionGracePeriod(method.input, request, typeRegistry);
      const withTimeStrings = timesToStrings(method.input, withoutGracePeriod, typeRegistry);
      // cast: each step returns the JSON object it was given, rewritten
      const packedRequest = packAnyFields(method.input, withTimeStrings) as JsonValue;
      // Parsing and re-serializing checks every value against its field type and drops keys
      // that aren't fields. Callers pass whole records back (e.g. form state), so unknown keys
      // are expected rather than an error.
      const message = fromJson(method.input, packedRequest, {
        registry: typeRegistry,
        ignoreUnknownFields: true,
      });
      const requestJson = toJson(method.input, message, { registry: typeRegistry });
      const responseJson = await transport.callUnary(
        service.typeName,
        method.name,
        requestJson,
        headers
      );
      return timesToObjects(method.output, responseJson, typeRegistry);
    };
  }

  // cast: dynamic method construction can't be statically typed
  return client as ServiceClient<T>;
}

let servicesPromise: Promise<Services> | null = null;

async function createServices(): Promise<Services> {
  const { apiBaseUrl } = await getRuntimeConfig();

  const transport = createFetchTransport({ baseUrl: apiBaseUrl });

  return {
    ClusterService: createServiceClient(ClusterService, transport),
    DeploymentService: createServiceClient(DeploymentService, transport),
    InferenceServerService: createServiceClient(InferenceServerService, transport),
    ProjectService: createServiceClient(ProjectService, transport),
    PipelineService: createServiceClient(PipelineService, transport),
    PipelineRunService: createServiceClient(PipelineRunService, transport),
    TriggerRunService: createServiceClient(TriggerRunService, transport),
    ModelService: createServiceClient(ModelService, transport),
    ModelFamilyService: createServiceClient(ModelFamilyService, transport),
    RevisionService: createServiceClient(RevisionService, transport),
  } as const;
}

/**
 * Gets the RPC services, initializing them with runtime configuration on first call.
 */
export async function getServices(): Promise<Services> {
  // eslint-disable-next-line @typescript-eslint/prefer-nullish-coalescing
  if (!servicesPromise) {
    servicesPromise = createServices();
  }
  return servicesPromise;
}
