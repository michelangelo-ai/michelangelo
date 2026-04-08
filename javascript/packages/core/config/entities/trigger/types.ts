/**
 * Mirrors generated types from @michelangelo/rpc trigger_run_pb.
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
