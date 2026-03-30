import { getServices } from './services';

import type { ExtractUnaryRpc } from './types';

let handlersPromise: Promise<Awaited<ReturnType<typeof createHandlers>>> | null = null;

async function createHandlers() {
  const services = await getServices();

  return {
    ListProject: services.ProjectService.listProject as ExtractUnaryRpc<
      typeof services.ProjectService.listProject
    >,
    GetProject: services.ProjectService.getProject as ExtractUnaryRpc<
      typeof services.ProjectService.getProject
    >,
    ListPipeline: services.PipelineService.listPipeline as ExtractUnaryRpc<
      typeof services.PipelineService.listPipeline
    >,
    ListPipelineRun: services.PipelineRunService.listPipelineRun as ExtractUnaryRpc<
      typeof services.PipelineRunService.listPipelineRun
    >,
    GetPipelineRun: services.PipelineRunService.getPipelineRun as ExtractUnaryRpc<
      typeof services.PipelineRunService.getPipelineRun
    >,
    ListTriggerRun: services.TriggerRunService.listTriggerRun as ExtractUnaryRpc<
      typeof services.TriggerRunService.listTriggerRun
    >,
    GetTriggerRun: services.TriggerRunService.getTriggerRun as ExtractUnaryRpc<
      typeof services.TriggerRunService.getTriggerRun
    >,
    CreatePipelineRun: services.PipelineRunService.createPipelineRun as ExtractUnaryRpc<
      typeof services.PipelineRunService.createPipelineRun
    >,
    UpdatePipelineRun: services.PipelineRunService.updatePipelineRun as ExtractUnaryRpc<
      typeof services.PipelineRunService.updatePipelineRun
    >,
    ListModel: services.ModelService.listModel as ExtractUnaryRpc<
      typeof services.ModelService.listModel
    >,
  } as const;
}

/** Gets the RPC handlers, initializing them with runtime configuration on first call. */
export async function getRpcHandlers() {
  // eslint-disable-next-line @typescript-eslint/prefer-nullish-coalescing
  if (!handlersPromise) {
    handlersPromise = createHandlers();
  }
  return handlersPromise;
}
