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
    action?: TriggerRunAction;
  };
  status: {
    state: TriggerRunState;
  };
};

export enum TriggerRunAction {
  NO_ACTION = 0,
  KILL = 1,
  PAUSE = 2,
  RESUME = 3,
}

export enum TriggerRunState {
  INVALID = 0,
  RUNNING = 1,
  KILLED = 2,
  FAILED = 3,
  SUCCEEDED = 4,
  PENDING_KILL = 5,
  PAUSED = 6,
}
