export type NotificationType = 'NOTIFICATION_TYPE_EMAIL' | 'NOTIFICATION_TYPE_SLACK';

export type NotificationEventType =
  | 'EVENT_TYPE_PIPELINE_RUN_STATE_STARTED'
  | 'EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED'
  | 'EVENT_TYPE_PIPELINE_RUN_STATE_FAILED'
  | 'EVENT_TYPE_PIPELINE_RUN_STATE_KILLED'
  | 'EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED';

export type PipelineRunNotification = {
  notification_type: NotificationType;
  event_types: NotificationEventType[];
  resource_type: 'RESOURCE_TYPE_PIPELINE_RUN';
  emails: string[];
  slack_destinations: string[];
};

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
    /**
     * Continues execution from a previous run instead of starting over.
     *
     * `resumeUpTo` and `forceResume` exist on the proto message but are read by
     * nothing in the platform, so they are deliberately unrepresented here.
     * See `proto/api/v2/pipeline_run.proto` (message `Resume`).
     */
    resume?: Resume;
    /** Inline pipeline spec for dev runs; regular runs reference a Pipeline instead. */
    pipelineSpec?: PipelineSnapshotSpec;
    notifications?: PipelineRunNotification[];
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

export type Resume = {
  /** The run being resumed from. Required whenever `resume` is set. */
  pipelineRun: {
    name: string;
    namespace: string;
  };
  /**
   * DAG tasks to re-execute, identified by sub-step `displayName` — never `name`,
   * which carries the task path. Empty means continue from wherever the source run
   * stopped, reusing every cached output.
   */
  resumeFrom?: string[];
};

/** Mirrors proto PipelineRunState enum (pipeline_run.proto). */
export enum PipelineRunState {
  QUEUED = 0,
  PENDING = 1,
  RUNNING = 2,
  SUCCEEDED = 3,
  KILLED = 4,
  FAILED = 5,
  SKIPPED = 6,
}

/**
 * States in which a run has stopped for good.
 *
 * A run must be terminal before it can be resumed from: the resume cache is built only
 * from sub-steps that already succeeded, so a queued, pending, or running run has
 * nothing settled to offer.
 */
export const TERMINAL_RUN_STATES: ReadonlySet<PipelineRunState> = new Set([
  PipelineRunState.SUCCEEDED,
  PipelineRunState.KILLED,
  PipelineRunState.FAILED,
  PipelineRunState.SKIPPED,
]);

/** Timestamp as returned by the API for pipeline run steps. */
type StepTimestamp = {
  seconds?: string;
};

/** The subset of `PipelineRunStepInfo` the resume step picker reads. */
export type PipelineRunStepInfo = {
  name?: string;
  displayName?: string;
  state?: number;
  startTime?: StepTimestamp;
  endTime?: StepTimestamp;
  subSteps?: PipelineRunStepInfo[];
};

/** The subset of a PipelineRun the resume fields read from list and get responses. */
export type PipelineRunSummary = {
  metadata?: {
    name?: string;
    namespace?: string;
    creationTimestamp?: StepTimestamp;
  };
  spec?: {
    pipeline?: {
      name?: string;
    };
  };
  status?: {
    state?: number;
    steps?: PipelineRunStepInfo[];
  };
};

export type ListPipelineRunResponse = {
  pipelineRunList?: {
    items?: PipelineRunSummary[];
  };
};

export type GetPipelineRunResponse = {
  pipelineRun?: PipelineRunSummary;
};
