import { createClient, Interceptor } from '@connectrpc/connect';
import { createGrpcWebTransport } from '@connectrpc/connect-web';

import { PipelineRunService } from './gen/michelangelo/api/v2/pipeline_run_svc_pb';
import { PipelineService } from './gen/michelangelo/api/v2/pipeline_svc_pb';
import { ProjectService } from './gen/michelangelo/api/v2/project_svc_pb';

// This interceptor is used to set the headers for the RPC request to
// be compatible with the Michelangelo API yarpc server.
const callerInterceptor: Interceptor = (next) => async (req) => {
  req.header.set('context-Ttl-Ms', '10000');
  req.header.set('grpc-timeout', '1000000m');
  req.header.set('Rpc-Caller', 'ma-studio');
  req.header.set('Rpc-Encoding', 'proto');
  req.header.set('Rpc-Service', 'ma-apiserver');

  return await next(req);
};

// This transport is used to connect to the Envoy proxy that proxies gRPC web requests
// to gRPC services.
const transport = createGrpcWebTransport({
  baseUrl: import.meta.env.VITE_API_BASE_URL || 'http://localhost:8081',
  interceptors: [callerInterceptor],
  useBinaryFormat: true,
});

export const SERVICES = {
  ProjectService: createClient(ProjectService, transport),
  PipelineService: createClient(PipelineService, transport),
  PipelineRunService: createClient(PipelineRunService, transport),
} as const;
