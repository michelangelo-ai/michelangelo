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

export type TriggerRun = {
  metadata: {
    name: string;
    namespace: string;
  };
  spec: {
    pipeline: {
      name: string;
      namespace: string;
    };
    revision: {
      name: string;
      namespace: string;
    };
    actor: {
      name: string;
    };
    // Action field for trigger operations (replaces deprecated boolean fields)
    action?: TriggerRunAction;
    // DEPRECATED: Use 'action' field instead. Boolean field for kill operations
    kill?: boolean;
    sourceTriggerName?: string;
    autoFlip?: boolean;
  };
  status: {
    state: TriggerRunState;
    executionWorkflowId?: string;
    errorMessage?: string;
    logUrl?: string;
  };
};
