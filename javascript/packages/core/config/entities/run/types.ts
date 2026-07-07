/**
 * Single source of truth for pipeline run notification trigger conditions: each entry's
 * proto enum id, UI display label, and proto `Notification.EventType` numeric value
 * (see notification.proto). Adding a new trigger condition only requires adding one
 * entry here — the id list, UI options, and proto-value lookup all derive from it.
 */
export const NOTIFICATION_EVENT_TYPES = [
  { id: 'EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED', label: 'Succeeded', protoValue: 1 },
  { id: 'EVENT_TYPE_PIPELINE_RUN_STATE_KILLED', label: 'Killed', protoValue: 2 },
  { id: 'EVENT_TYPE_PIPELINE_RUN_STATE_FAILED', label: 'Failed', protoValue: 3 },
  { id: 'EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED', label: 'Skipped', protoValue: 4 },
] as const;

/** Trigger condition for a pipeline run notification. */
export type NotificationEventType = (typeof NOTIFICATION_EVENT_TYPES)[number]['id'];

/** All trigger conditions for pipeline run notifications, used as the default selection. */
export const ALL_NOTIFICATION_EVENT_TYPES: NotificationEventType[] = NOTIFICATION_EVENT_TYPES.map(
  (eventType) => eventType.id
);

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
