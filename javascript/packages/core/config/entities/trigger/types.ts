/**
 * Mirrors generated types from @michelangelo-ai/rpc trigger_run_pb.
 * Update alongside proto/api/v2/trigger_run.proto.
 */

import type { TriggerRunState } from './shared';

export type Trigger = {
  metadata: {
    name: string;
  };
  spec: {
    trigger: Pick<ManifestTrigger, 'cronSchedule' | 'intervalSchedule' | 'batchRerun'>;
  };
};

/**
 * A trigger declared in a pipeline's manifest (`PipelineManifest.trigger_map`), in the proto
 * `Trigger` message's proto3 JSON form: the `oneof trigger_type` is whichever one of
 * `cronSchedule`, `intervalSchedule`, or `batchRerun` is set.
 *
 * The whole object is copied (and, for a backfill run, partially overridden) into
 * {@link RunTriggerPayload.spec.trigger} on submit, so fields not read directly by the UI
 * still survive the round trip via object spread — they just aren't typed here individually
 * unless a form field needs to read or override them (see `parametersMap`, `maxConcurrency`).
 */
export type ManifestTrigger = {
  cronSchedule?: { cron?: string };
  /** `interval` is a `google.protobuf.Duration`; its int64 `seconds` arrives as a string. */
  intervalSchedule?: { interval?: { seconds?: string | number } };
  batchRerun?: BatchRerun;
  /** Dynamic pipeline parameters this trigger can run with, keyed by parameter ID. */
  parametersMap?: Record<string, unknown>;
  /** Default cap on concurrent runs for this trigger; overridable for a backfill run. */
  maxConcurrency?: number;
  /**
   * Rate-limits how many pipeline runs this trigger creates at once. Mutually exclusive
   * with {@link ManifestTrigger.maxConcurrency} in practice — a trigger configured with
   * `maxConcurrency` ignores `batchPolicy` (see {@link TRIGGER_DETAIL_CONFIG}'s metadata,
   * which hides one group when the other is set).
   */
  batchPolicy?: {
    /** Number of pipeline runs created in one batch. */
    batchSize?: number;
    /** Wait time between batches, in seconds. */
    waitSeconds?: bigint | number | string;
  };
};

/** Reruns a set of existing pipeline runs, optionally from/up to specific DAG nodes (proto `BatchRerun`). */
export type BatchRerun = {
  /** The pipeline runs to rerun. */
  pipelineRuns?: { name?: string; namespace?: string }[];
  /** DAG nodes execution resumes from. */
  resumeFrom?: string[];
  /** DAG nodes execution runs up to (inclusive). */
  resumeUpTo?: string[];
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
    labels?: Record<string, string>;
  };
  spec: {
    pipeline: { name: string; namespace: string };
    revision?: { name: string; namespace: string };
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
    labels?: Record<string, string>;
  };
  spec: {
    pipeline: { name: string; namespace: string };
    revision: { name: string; namespace: string };
    actor: { name: string };
    /** The trigger definition this run executes; whichever `trigger_type` member is set decides the schedule. */
    trigger?: ManifestTrigger;
    sourceTriggerName: string;
    autoFlip: boolean;
    notifications: unknown[];
    /** @deprecated Use action instead (proto field 11). */
    kill: boolean;
    /** proto field 11 — replaces deprecated kill boolean */
    action: TriggerRunAction;
    /** Backfill window start; present only for a backfill-created trigger run. */
    startTimestamp?: { seconds: string };
    /** Backfill window end; present only for a backfill-created trigger run. */
    endTimestamp?: { seconds: string };
  };
  status: {
    state: (typeof TriggerRunState)[keyof typeof TriggerRunState];
    /** Populated when the run failed; drives the Information tab's error-message section. */
    errorMessage?: string;
    /** Trigger-run-level workflow log link; omitted from the Information tab's links when unset. */
    logUrl?: string;
  };
};

/**
 * Mirrors proto TriggerRunAction enum (trigger_run.proto). Requests are parsed as proto3 JSON,
 * which accepts the numeric value as well as the string name.
 */
export enum TriggerRunAction {
  NO_ACTION = 0,
  KILL = 1,
  PAUSE = 2,
  RESUME = 3,
}
