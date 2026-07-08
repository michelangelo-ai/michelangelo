import { create, createRegistry, fromJson, toJson } from '@bufbuild/protobuf';

import { createFetchTransport } from './create-fetch-transport';
import { TypedStructSchema } from './gen/michelangelo/api/typed_struct_pb';
import { DeploymentService } from './gen/michelangelo/api/v2/deployment_svc_pb';
import { InferenceServerService } from './gen/michelangelo/api/v2/inference_server_svc_pb';
import { ModelService } from './gen/michelangelo/api/v2/model_svc_pb';
import { PipelineRunService } from './gen/michelangelo/api/v2/pipeline_run_svc_pb';
import { PipelineService } from './gen/michelangelo/api/v2/pipeline_svc_pb';
import { ProjectService } from './gen/michelangelo/api/v2/project_svc_pb';
import { TriggerRunService } from './gen/michelangelo/api/v2/trigger_run_svc_pb';
import { getRuntimeConfig } from './runtime-config';

import type { DescService } from '@bufbuild/protobuf';
import type { FetchTransport, ServiceClient, Services } from './types';

const typeRegistry = createRegistry(TypedStructSchema);

/**
 * Builds a service client whose methods JSON-encode the request, POST it
 * through the fetch transport, and decode the JSON response back into a
 * protobuf-es message. Envoy's grpc_json_transcoder handles the JSON<->binary
 * conversion on the wire, so this client only ever sees JSON.
 */
function createServiceClient<T extends DescService>(
  service: T,
  transport: FetchTransport
): ServiceClient<T> {
  const client: Record<string, (request: Record<string, unknown>) => Promise<unknown>> = {};

  for (const method of service.methods) {
    if (method.methodKind !== 'unary') continue;

    client[method.localName] = async (request) => {
      const message = create(method.input, request);
      const requestJson = toJson(method.input, message, { registry: typeRegistry });
      const responseJson = await transport.callUnary(service.typeName, method.name, requestJson);
      return fromJson(method.output, responseJson, { registry: typeRegistry });
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
    DeploymentService: createServiceClient(DeploymentService, transport),
    InferenceServerService: createServiceClient(InferenceServerService, transport),
    ProjectService: createServiceClient(ProjectService, transport),
    PipelineService: createServiceClient(PipelineService, transport),
    PipelineRunService: createServiceClient(PipelineRunService, transport),
    TriggerRunService: createServiceClient(TriggerRunService, transport),
    ModelService: createServiceClient(ModelService, transport),
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
