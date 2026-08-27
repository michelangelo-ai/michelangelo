import type { PipelineRun } from '#core/config/entities/run/types';

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
 * Form-only shape submitted by {@link CreatePipelineRunForm}.
 *
 * The notification fields here have no proto counterpart — `notifyOnCompletion` is a UI-only
 * toggle, and the email/Slack lists are the raw tag values from a `StringField multi`. All three
 * are read out of the form values and discarded; `handleRunSubmit` builds the real
 * `PipelineRunNotification[]` from them instead of forwarding them as-is.
 */
export type PipelineRunFormValues = PipelineRun & {
  notifyOnCompletion?: boolean;
  notificationEmails?: string[];
  notificationSlackDestinations?: string[];
};
