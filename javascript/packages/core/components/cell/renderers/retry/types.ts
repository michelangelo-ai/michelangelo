export type RetryActionRecord = {
  spec: Record<string, unknown>;
  status: { state: number; workflowId: string; workflowRunId: string };
  activityId: string;
  [key: string]: unknown;
};

export type RetryFormValues = {
  metadata: unknown;
  spec: Record<string, unknown> & {
    retryInfo: {
      activityId: string;
      workflowId: string;
      workflowRunId: string;
      reason: string;
    };
  };
};
