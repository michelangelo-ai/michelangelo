import { create } from '@bufbuild/protobuf';

import { UserInfoSchema } from './gen/michelangelo/api/v2/user_pb';
import { getServices } from './services';

import type { Deployment } from './gen/michelangelo/api/v2/deployment_pb';
import type { InferenceServer } from './gen/michelangelo/api/v2/inference_server_pb';
import type { PipelineRun } from './gen/michelangelo/api/v2/pipeline_run_pb';
import type { TriggerRun } from './gen/michelangelo/api/v2/trigger_run_pb';
import type { ExtractUnaryRpc } from './types';

let handlersPromise: Promise<Awaited<ReturnType<typeof createHandlers>>> | null = null;

function unary<Fn>(fn: Fn): ExtractUnaryRpc<Fn> {
  // cast: TS can't resolve ExtractUnaryRpc's conditional type against the unconstrained generic Fn
  // from within this function; fn always satisfies it at call sites
  return fn as unknown as ExtractUnaryRpc<Fn>;
}

// Delete<CRD>Request messages are uniformly { name, namespace, deleteOptions? } across every
// service, while actions send the full CRD record (metadata/spec/status/typeMeta). Wrapping a
// generated deleteX method here lets every Delete* handler share this reshape instead of each
// entity config repeating it as mutation middleware.
function deleteCrd<Req extends { name: string; namespace: string }, Res>(
  deleteFn: (request: Req, headers?: Record<string, string>) => Promise<Res>
) {
  return (
    record: { metadata: { name: string; namespace: string } },
    headers?: Record<string, string>
  ) =>
    // cast: Req may carry additional optional fields (e.g. deleteOptions) beyond name/namespace
    deleteFn({ name: record.metadata.name, namespace: record.metadata.namespace } as Req, headers);
}

async function createHandlers() {
  const services = await getServices();

  return {
    ListDeployment: unary(services.DeploymentService.listDeployment),
    GetDeployment: unary(services.DeploymentService.getDeployment),
    CreateDeployment: (record: Deployment, headers?: Record<string, string>) => {
      const actorName = headers?.['x-user-name'];
      if (actorName && record.spec) {
        record.spec.owner = create(UserInfoSchema, { name: actorName });
      }
      return services.DeploymentService.createDeployment({ deployment: record }, headers);
    },
    UpdateDeployment: (record: Deployment, headers?: Record<string, string>) => {
      const actorName = headers?.['x-user-name'];
      if (actorName && record.spec) {
        record.spec.owner = create(UserInfoSchema, { name: actorName });
      }
      return services.DeploymentService.updateDeployment({ deployment: record }, headers);
    },
    DeleteDeployment: deleteCrd(services.DeploymentService.deleteDeployment),
    ListInferenceServer: unary(services.InferenceServerService.listInferenceServer),
    GetInferenceServer: unary(services.InferenceServerService.getInferenceServer),
    CreateInferenceServer: (record: InferenceServer, headers?: Record<string, string>) =>
      services.InferenceServerService.createInferenceServer({ inferenceServer: record }, headers),
    ListProject: unary(services.ProjectService.listProject),
    GetProject: unary(services.ProjectService.getProject),
    GetPipeline: unary(services.PipelineService.getPipeline),
    ListPipeline: unary(services.PipelineService.listPipeline),
    DeletePipeline: deleteCrd(services.PipelineService.deletePipeline),
    ListPipelineRun: unary(services.PipelineRunService.listPipelineRun),
    GetPipelineRun: unary(services.PipelineRunService.getPipelineRun),
    ListTriggerRun: unary(services.TriggerRunService.listTriggerRun),
    GetTriggerRun: unary(services.TriggerRunService.getTriggerRun),
    CreateTriggerRun: (record: TriggerRun, headers?: Record<string, string>) => {
      const actorName = headers?.['x-user-name'];
      if (actorName && record.spec) {
        record.spec.actor = create(UserInfoSchema, { name: actorName });
      }
      return services.TriggerRunService.createTriggerRun({ triggerRun: record }, headers);
    },
    UpdateTriggerRun: (record: TriggerRun, headers?: Record<string, string>) =>
      services.TriggerRunService.updateTriggerRun({ triggerRun: record }, headers),
    CreatePipelineRun: (record: PipelineRun, headers?: Record<string, string>) => {
      const actorName = headers?.['x-user-name'];
      if (actorName && record.spec) {
        record.spec.actor = create(UserInfoSchema, { name: actorName });
      }
      return services.PipelineRunService.createPipelineRun({ pipelineRun: record }, headers);
    },
    UpdatePipelineRun: (record: PipelineRun, headers?: Record<string, string>) =>
      services.PipelineRunService.updatePipelineRun({ pipelineRun: record }, headers),
    ListModel: unary(services.ModelService.listModel),
    GetModel: unary(services.ModelService.getModel),
    ListModelFamily: unary(services.ModelFamilyService.listModelFamily),
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
