/** All trigger conditions for pipeline run notifications, used as the default selection. */
export const ALL_NOTIFICATION_EVENT_TYPES = [
  'EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED',
  'EVENT_TYPE_PIPELINE_RUN_STATE_FAILED',
  'EVENT_TYPE_PIPELINE_RUN_STATE_KILLED',
  'EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED',
] as const;

/** Trigger condition for a pipeline run notification. */
export type NotificationEventType = (typeof ALL_NOTIFICATION_EVENT_TYPES)[number];

/**
 * A single notification channel attached to a pipeline run.
 * Sent when any of the listed event types fires.
 * Mirrors the proto `Notification` message (see notification.proto).
 */
export type Notification = {
  emails: string[];
  slackDestinations: string[];
  /** Proto enum Notification.EventType numeric values (see notification.proto). */
  eventTypes: number[];
};

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
    /** Optional human-readable description for this run. */
    description?: string;
    /** Optional notification channels. Omitted when the user leaves the section blank. */
    notifications?: Notification[];
  };
};
