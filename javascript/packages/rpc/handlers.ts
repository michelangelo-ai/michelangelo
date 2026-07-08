import { getServices } from './services';

import type { PipelineRun } from './gen/michelangelo/api/v2/pipeline_run_pb';
import type { TriggerRun } from './gen/michelangelo/api/v2/trigger_run_pb';
import type { ExtractUnaryRpc } from './types';

let handlersPromise: Promise<Awaited<ReturnType<typeof createHandlers>>> | null = null;

function unary<Fn>(fn: Fn): ExtractUnaryRpc<Fn> {
  // cast: TS can't resolve ExtractUnaryRpc's conditional type against the unconstrained generic Fn
  // from within this function; fn always satisfies it at call sites
  return fn as unknown as ExtractUnaryRpc<Fn>;
}

async function createHandlers() {
  const services = await getServices();

  return {
    ListDeployment: unary(services.DeploymentService.listDeployment),
    GetDeployment: unary(services.DeploymentService.getDeployment),
    ListInferenceServer: unary(services.InferenceServerService.listInferenceServer),
    GetInferenceServer: unary(services.InferenceServerService.getInferenceServer),
    ListProject: unary(services.ProjectService.listProject),
    GetProject: unary(services.ProjectService.getProject),
    GetPipeline: unary(services.PipelineService.getPipeline),
    ListPipeline: unary(services.PipelineService.listPipeline),
    ListPipelineRun: unary(services.PipelineRunService.listPipelineRun),
    GetPipelineRun: unary(services.PipelineRunService.getPipelineRun),
    ListTriggerRun: unary(services.TriggerRunService.listTriggerRun),
    GetTriggerRun: unary(services.TriggerRunService.getTriggerRun),
    UpdateTriggerRun: (record: TriggerRun) =>
      services.TriggerRunService.updateTriggerRun({ triggerRun: record }),
    CreatePipelineRun: (record: PipelineRun) =>
      services.PipelineRunService.createPipelineRun({ pipelineRun: record }),
    UpdatePipelineRun: (record: PipelineRun) =>
      services.PipelineRunService.updatePipelineRun({ pipelineRun: record }),
    ListModel: unary(services.ModelService.listModel),
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
