/**
 * Mirrors generated types from @michelangelo-ai/rpc trigger_run_pb.
 * Update alongside proto/api/v2/trigger_run.proto.
 */

export type Trigger = {
  metadata: {
    name: string;
  };
  spec: {
    trigger: {
      triggerType: {
        case: 'cronSchedule' | 'batchRerun' | 'intervalSchedule';
      };
    };
  };
};

/**
 * A trigger declared in a pipeline's manifest (`PipelineManifest.trigger_map`), as
 * protobuf-es decodes the proto `Trigger` message: the `oneof trigger_type` becomes a
 * tagged `{ case, value }` union.
 *
 * The whole object is copied (and, for a backfill run, partially overridden) into
 * {@link RunTriggerPayload.spec.trigger} on submit, so fields not read directly by the UI
 * still survive the round trip via object spread — they just aren't typed here individually
 * unless a form field needs to read or override them (see `parametersMap`, `maxConcurrency`).
 */
export type ManifestTrigger = {
  triggerType?:
    | { case: 'cronSchedule'; value: { cron?: string } }
    | { case: 'intervalSchedule'; value: { interval?: { seconds?: bigint | number | string } } }
    | { case: 'batchRerun'; value: unknown };
  /** Dynamic pipeline parameters this trigger can run with, keyed by parameter ID. */
  parametersMap?: Record<string, unknown>;
  /** Default cap on concurrent runs for this trigger; overridable for a backfill run. */
  maxConcurrency?: number;
};

/**
 * Payload for `CreateTriggerRun` when running a pipeline from a manifest trigger — either on
 * its declared schedule, or as a one-off backfill over a time window.
 *
 * `spec.trigger` carries the schedule and is what actually drives execution — the
 * reconciler reads it directly (`GetTriggerType` in go/components/triggerrun/util.go) and
 * falls through to `TriggerTypeUnknown` if it is absent, leaving the TriggerRun inert.
 * `spec.sourceTriggerName` is provenance only; nothing on the backend resolves it.
 */
export type RunTriggerPayload = {
  metadata: {
    name: string;
    namespace: string;
  };
  spec: {
    pipeline: { name: string; namespace: string };
    trigger: ManifestTrigger;
    sourceTriggerName: string;
    autoFlip: boolean;
    /** Epoch-seconds window bounds; present only for a backfill run. */
    startTimestamp?: { seconds: string };
    endTimestamp?: { seconds: string };
  };
};

export type TriggerRun = {
  metadata: {
    name: string;
    namespace: string;
  };
  spec: {
    pipeline: { name: string; namespace: string };
    revision: { name: string; namespace: string };
    actor: { name: string };
    sourceTriggerName: string;
    autoFlip: boolean;
    notifications: unknown[];
    /** @deprecated Use action instead (proto field 11). */
    kill: boolean;
    /** proto field 11 — replaces deprecated kill boolean */
    action: TriggerRunAction;
  };
  status: {
    state: TriggerRunState;
  };
};

/** Mirrors proto TriggerRunAction enum (trigger_run.proto). */
export enum TriggerRunAction {
  NO_ACTION = 0,
  KILL = 1,
  PAUSE = 2,
  RESUME = 3,
}

/** Mirrors proto TriggerRunState enum (trigger_run.proto). */
export enum TriggerRunState {
  INVALID = 0,
  RUNNING = 1,
  KILLED = 2,
  FAILED = 3,
  SUCCEEDED = 4,
  PENDING_KILL = 5,
  PAUSED = 6,
}
