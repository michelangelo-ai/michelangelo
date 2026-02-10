export type PipelineRunStatus = {
  state: number;
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
