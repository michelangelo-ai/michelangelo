export type PipelineRun = {
  metadata: {
    name: string;
    namespace: string;
  };
  spec: {
    actor: {
      name: string;
    };
    pipeline: {
      name: string;
      namespace: string;
    };
    /** Optional human-readable description for this run. */
    description?: string;
  };
};
