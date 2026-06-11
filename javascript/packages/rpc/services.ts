import { createRegistry } from '@bufbuild/protobuf';
import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';

import { TypedStructSchema } from './gen/michelangelo/api/typed_struct_pb';
import { DeploymentService } from './gen/michelangelo/api/v2/deployment_svc_pb';
import { InferenceServerService } from './gen/michelangelo/api/v2/inference_server_svc_pb';
import { ModelService } from './gen/michelangelo/api/v2/model_svc_pb';
import { PipelineRunService } from './gen/michelangelo/api/v2/pipeline_run_svc_pb';
import { PipelineService } from './gen/michelangelo/api/v2/pipeline_svc_pb';
import { ProjectService } from './gen/michelangelo/api/v2/project_svc_pb';
import { TriggerRunService } from './gen/michelangelo/api/v2/trigger_run_svc_pb';
import { getRuntimeConfig } from './runtime-config';

const typeRegistry = createRegistry(TypedStructSchema);

import type { Interceptor } from '@connectrpc/connect';
import type { Services } from './types';

// This interceptor is used to set the headers for the RPC request to
// be compatible with the Michelangelo API yarpc server.
const callerInterceptor: Interceptor = (next) => async (req) => {
  req.header.set('context-Ttl-Ms', '10000');
  req.header.set('grpc-timeout', '1000000m');
  req.header.set('Rpc-Caller', 'ma-studio');
  req.header.set('Rpc-Service', 'ma-apiserver');

  return await next(req);
};

let servicesPromise: Promise<Services> | null = null;

async function createServices(): Promise<Services> {
  const { apiBaseUrl } = await getRuntimeConfig();

  const transport = createConnectTransport({
    baseUrl: apiBaseUrl,
    interceptors: [callerInterceptor],
    jsonOptions: { registry: typeRegistry },
  });

  return {
    DeploymentService: createClient(DeploymentService, transport),
    InferenceServerService: createClient(InferenceServerService, transport),
    ProjectService: createClient(ProjectService, transport),
    PipelineService: createClient(PipelineService, transport),
    PipelineRunService: createClient(PipelineRunService, transport),
    TriggerRunService: createClient(TriggerRunService, transport),
    ModelService: createClient(ModelService, transport),
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
