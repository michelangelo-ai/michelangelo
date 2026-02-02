type CronSchedule = {
  cron: string;
};

type IntervalSchedule = {
  interval?: {
    seconds: number;
  };
};

type BatchPolicy = {
  batchSize: number;
  wait?: {
    seconds: number;
  };
};

type TriggerType =
  | { case: 'cronSchedule'; value: CronSchedule }
  | { case: 'intervalSchedule'; value: IntervalSchedule }
  | { case: 'batchRerun'; value: unknown }
  | { case: undefined; value?: undefined };

/**
 * TriggerRun represents a single execution of a pipeline trigger.
 * Based on proto/api/v2/trigger_run.proto
 *
 * Note: This is a simplified UI type. The trigger.triggerType uses protobuf-es
 * discriminated union format: { case: 'cronSchedule', value: CronSchedule }
 */
export type TriggerRun = {
  metadata: {
    name: string;
    creationTimestamp?: {
      seconds: number;
    };
  };
  spec: {
    actor?: {
      name: string;
    };
    pipeline?: {
      name: string;
    };
    revision?: {
      name: string;
    };
    trigger: {
      triggerType: TriggerType;
      batchPolicy?: BatchPolicy;
      maxConcurrency?: number;
      parametersMap?: Record<string, unknown>;
    };
    startTimestamp?: {
      seconds: number;
    };
    endTimestamp?: {
      seconds: number;
    };
  };
  status?: {
    state: number;
    logUrl?: string;
    errorMessage?: string;
  };
};
