export type PipelineRunStatus = {
  state: string;
  workflowId: string;
  workflowRunId: string;
};

export type PipelineRunData = {
  pipelineRun: {
    spec: Record<string, unknown>;
    status: PipelineRunStatus;
    [key: string]: unknown;
  };
};
