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
  };
};
