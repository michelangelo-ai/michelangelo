import { SERVICES } from './services';

export const RPC_HANDLERS = {
  ListProject: SERVICES.ProjectService.listProject,
  GetProject: SERVICES.ProjectService.getProject,
  ListPipeline: SERVICES.PipelineService.listPipeline,
} as const;
