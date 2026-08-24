export type PipelineRun = {
  metadata: {
    name: string;
    namespace: string;
  };
  spec: {
    /** Populated server-side from the `x-user-name` request header, not set by the client. */
    actor?: {
      name: string;
    };
    pipeline: {
      name: string;
      namespace: string;
    };
    /** Optional human-readable description for this run. */
    description?: string;
    /** Inline pipeline spec for dev runs; regular runs reference a Pipeline instead. */
    pipelineSpec?: PipelineSnapshotSpec;
  };
  status?: {
    /** Snapshot of the pipeline this run executes, captured when the run starts. */
    sourcePipeline?: {
      pipeline?: { spec?: PipelineSnapshotSpec };
      draftPipeline?: { spec?: PipelineSnapshotSpec };
    };
  };
};

export type PipelineSnapshotSpec = {
  manifest?: PipelineManifest;
};

export type PipelineManifest = {
  /** PipelineManifest.Type enum, decoded to its numeric discriminant. */
  type?: number;
  filePath?: string;
  /**
   * Pipeline configuration, unpacked from its Any/TypedStruct envelope by the rpc layer:
   * typeUrl names the config type, value is the config itself as plain JSON.
   */
  content?: {
    typeUrl?: string;
    value?: Record<string, unknown>;
  };
};
