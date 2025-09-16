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
  ListPipelineRun: SERVICES.PipelineRunService.listPipelineRun as ExtractUnaryRpc<
    typeof SERVICES.PipelineRunService.listPipelineRun
  >,
  GetPipelineRun: SERVICES.PipelineRunService.getPipelineRun as ExtractUnaryRpc<
    typeof SERVICES.PipelineRunService.getPipelineRun
  >,
  ListTriggerRun: SERVICES.TriggerRunService.listTriggerRun as ExtractUnaryRpc<
    typeof SERVICES.TriggerRunService.listTriggerRun
  >,
  GetTriggerRun: SERVICES.TriggerRunService.getTriggerRun as ExtractUnaryRpc<
    typeof SERVICES.TriggerRunService.getTriggerRun
  >,
} as const;
