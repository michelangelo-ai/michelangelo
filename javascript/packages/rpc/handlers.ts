import { createClient, Transport } from '@connectrpc/connect';

import { PipelineService } from './gen/michelangelo/api/v2/pipeline_svc_pb';
import { ProjectService } from './gen/michelangelo/api/v2/project_svc_pb';

export const RpcHandlers = (transport: Transport) => {
  const ProjectServiceClient = createClient(ProjectService, transport);
  const PipelineServiceClient = createClient(PipelineService, transport);

  return {
    ListProject: ProjectServiceClient.listProject,
    GetProject: ProjectServiceClient.getProject,
    ListPipeline: PipelineServiceClient.listPipeline,
  };
};
