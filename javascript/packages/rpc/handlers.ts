import { SERVICES } from './services';
import { ExtractUnaryRpc } from './types';

export const RPC_HANDLERS = {
  ListProject: SERVICES.ProjectService.listProject as ExtractUnaryRpc<
    typeof SERVICES.ProjectService.listProject
  >,
  GetProject: SERVICES.ProjectService.getProject as ExtractUnaryRpc<
    typeof SERVICES.ProjectService.getProject
  >,
  ListPipeline: SERVICES.PipelineService.listPipeline as ExtractUnaryRpc<
    typeof SERVICES.PipelineService.listPipeline
  >,
} as const;
