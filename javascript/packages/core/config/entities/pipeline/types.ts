import type { NotificationEventType } from '#core/config/entities/run/types';

export interface Pipeline {
  metadata: {
    name: string;
    namespace: string;
  };
  spec: {
    owner: {
      name: string;
    };
  };
}

/** Notification recipient state for CreatePipelineRunForm, held as local React state. */
export type NotificationDetailsValue = {
  emails: string[];
  slackChannels: string[];
  eventTypes: NotificationEventType[];
};
