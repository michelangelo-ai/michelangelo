/**
 * Mirrors proto Notification.NotificationType enum (notification.proto).
 *
 * Numeric values matter here, not just the names: requests are built via `create()` from
 * `@bufbuild/protobuf`, which serializes enum fields by their proto number, not by name.
 */
export enum NotificationType {
  EMAIL = 1,
  SLACK = 2,
}

/**
 * Mirrors proto Notification.EventType enum (notification.proto), restricted to the events a
 * pipeline run notification can fire on. See {@link NotificationType} for why the numbers matter.
 */
export enum NotificationEventType {
  PIPELINE_RUN_STATE_SUCCEEDED = 1,
  PIPELINE_RUN_STATE_KILLED = 2,
  PIPELINE_RUN_STATE_FAILED = 3,
  PIPELINE_RUN_STATE_SKIPPED = 4,
  PIPELINE_RUN_STATE_STARTED = 11,
}

/**
 * Mirrors proto Notification.ResourceType enum (notification.proto).
 */
export enum NotificationResourceType {
  PIPELINE_RUN = 1,
}

/**
 * Mirrors proto message Notification (notification.proto).
 *
 * Field names must match the generated protobuf-es message's camelCase `localName`s exactly:
 * `create()` looks fields up by that name, and silently drops anything that doesn't match
 * (rather than erroring), so a wrong name here fails silently instead of at compile time.
 */
export type PipelineRunNotification = {
  notificationType: NotificationType;
  eventTypes: NotificationEventType[];
  resourceType: NotificationResourceType;
  emails: string[];
  slackDestinations: string[];
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

/** The subset of `PipelineRunStepInfo` the resume step picker and information tab read. */
export type PipelineRunStepInfo = {
  name?: string;
  displayName?: string;
  state?: number;
  startTime?: StepTimestamp;
  endTime?: StepTimestamp;
  logUrl?: string;
  subSteps?: PipelineRunStepInfo[];
};

/** The subset of a PipelineRun the resume fields and information tab read from list and get responses. */
export type PipelineRunSummary = {
  metadata?: {
    name?: string;
    namespace?: string;
    creationTimestamp?: StepTimestamp;
    labels?: Record<string, string>;
  };
  spec?: {
    pipeline?: {
      name?: string;
    };
    resume?: {
      pipelineRun?: {
        name?: string;
      };
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
