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
import { packAnyFields } from './pack-any-fields';
import { getRuntimeConfig } from './runtime-config';

import type { DescService, JsonValue } from '@bufbuild/protobuf';
import type { FetchTransport, ServiceClient, Services } from './types';

// Every message type that can appear inside a google.protobuf.Any on the wire must be
// registered here — protobuf-es resolves an Any's typeUrl against this registry both when
// parsing and re-encoding requests (fromJson/toJson throw on an unregistered typeUrl), e.g.
// ListOptionsExt criteria packed by packAnyFields. PipelineSchema covers
// Revision.spec.content for Pipeline revisions sent back on update.
export const typeRegistry = createRegistry(
  TypedStructSchema,
  PipelineSchema,
  StringValueSchema,
  BoolValueSchema,
  Int64ValueSchema,
  DoubleValueSchema
);

/**
 * Builds a service client that speaks proto3 JSON in both directions: requests are
 * proto3 JSON objects (string enums, oneof members set directly, Any as `@type` plus
 * fields), validated and normalized through the message schema before being POSTed
 * through the fetch transport; responses are returned as Envoy's grpc_json_transcoder
 * emits them. The one exception, both ways, is Timestamp/Duration fields: the UI works
 * with `{ seconds, nanos }` rather than proto3 JSON's strings (see convert-time-fields).
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
      // cast: packAnyFields recurses generically over `unknown`; called with a JSON object it
      // returns one, just with Any fields packed into their proto3 JSON form
      const packedRequest = packAnyFields(
        method.input,
        timesToStrings(method.input, request, typeRegistry)
      ) as JsonValue;
      // ignoreUnknownFields: callers pass whole records back (e.g. form state), and
      // extra keys shouldn't fail the request
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
