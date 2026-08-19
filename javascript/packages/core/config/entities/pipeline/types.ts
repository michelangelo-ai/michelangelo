import type { PipelineRun, PipelineRunNotification } from '#core/config/entities/run/types';

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

/**
 * Form-only shape of a notification entry: `emails`/`slack_destinations` are wrapped
 * in `{ value }` objects because `ArrayFormRow` items must be objects, not scalars.
 * Flattened back to plain string arrays before submitting.
 */
export type NotificationFormValue = Omit<
  PipelineRunNotification,
  'emails' | 'slack_destinations'
> & {
  emails?: { value: string }[];
  slack_destinations?: { value: string }[];
};

export type PipelineRunFormValues = Omit<PipelineRun, 'spec'> & {
  spec: Omit<PipelineRun['spec'], 'notifications'> & {
    notifications?: NotificationFormValue[];
  };
};
