import { getServices } from './services';

import type { PipelineRun } from './gen/michelangelo/api/v2/pipeline_run_pb';
import type { TriggerRun } from './gen/michelangelo/api/v2/trigger_run_pb';
import type { ExtractUnaryRpc } from './types';

let handlersPromise: Promise<Awaited<ReturnType<typeof createHandlers>>> | null = null;

async function createHandlers() {
  const services = await getServices();

  return {
    // cast: ConnectRPC generates service methods with proto-specific overloads; ExtractUnaryRpc extracts the generic unary call signature
    ListDeployment: services.DeploymentService.listDeployment as ExtractUnaryRpc<
      typeof services.DeploymentService.listDeployment
    >,
    // cast: ConnectRPC generates service methods with proto-specific overloads; ExtractUnaryRpc extracts the generic unary call signature
    GetDeployment: services.DeploymentService.getDeployment as ExtractUnaryRpc<
      typeof services.DeploymentService.getDeployment
    >,
    // cast: ConnectRPC generates service methods with proto-specific overloads; ExtractUnaryRpc extracts the generic unary call signature
    ListInferenceServer: services.InferenceServerService.listInferenceServer as ExtractUnaryRpc<
      typeof services.InferenceServerService.listInferenceServer
    >,
    // cast: ConnectRPC generates service methods with proto-specific overloads; ExtractUnaryRpc extracts the generic unary call signature
    GetInferenceServer: services.InferenceServerService.getInferenceServer as ExtractUnaryRpc<
      typeof services.InferenceServerService.getInferenceServer
    >,
    // cast: ConnectRPC generates service methods with proto-specific overloads; ExtractUnaryRpc extracts the generic unary call signature
    ListProject: services.ProjectService.listProject as ExtractUnaryRpc<
      typeof services.ProjectService.listProject
    >,
    // cast: ConnectRPC generates service methods with proto-specific overloads; ExtractUnaryRpc extracts the generic unary call signature
    GetProject: services.ProjectService.getProject as ExtractUnaryRpc<
      typeof services.ProjectService.getProject
    >,
    // cast: ConnectRPC generates service methods with proto-specific overloads; ExtractUnaryRpc extracts the generic unary call signature
    GetPipeline: services.PipelineService.getPipeline as ExtractUnaryRpc<
      typeof services.PipelineService.getPipeline
    >,
    // cast: ConnectRPC generates service methods with proto-specific overloads; ExtractUnaryRpc extracts the generic unary call signature
    ListPipeline: services.PipelineService.listPipeline as ExtractUnaryRpc<
      typeof services.PipelineService.listPipeline
    >,
    // cast: ConnectRPC generates service methods with proto-specific overloads; ExtractUnaryRpc extracts the generic unary call signature
    ListPipelineRun: services.PipelineRunService.listPipelineRun as ExtractUnaryRpc<
      typeof services.PipelineRunService.listPipelineRun
    >,
    // cast: ConnectRPC generates service methods with proto-specific overloads; ExtractUnaryRpc extracts the generic unary call signature
    GetPipelineRun: services.PipelineRunService.getPipelineRun as ExtractUnaryRpc<
      typeof services.PipelineRunService.getPipelineRun
    >,
    // cast: ConnectRPC generates service methods with proto-specific overloads; ExtractUnaryRpc extracts the generic unary call signature
    ListTriggerRun: services.TriggerRunService.listTriggerRun as ExtractUnaryRpc<
      typeof services.TriggerRunService.listTriggerRun
    >,
    // cast: ConnectRPC generates service methods with proto-specific overloads; ExtractUnaryRpc extracts the generic unary call signature
    GetTriggerRun: services.TriggerRunService.getTriggerRun as ExtractUnaryRpc<
      typeof services.TriggerRunService.getTriggerRun
    >,
    UpdateTriggerRun: (record: TriggerRun) =>
      services.TriggerRunService.updateTriggerRun({ triggerRun: record }),
    CreatePipelineRun: (record: PipelineRun) =>
      services.PipelineRunService.createPipelineRun({ pipelineRun: record }),
    UpdatePipelineRun: (record: PipelineRun) =>
      services.PipelineRunService.updatePipelineRun({ pipelineRun: record }),
    // cast: ConnectRPC generates service methods with proto-specific overloads; ExtractUnaryRpc extracts the generic unary call signature
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
