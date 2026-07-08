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
 * `notificationEmails`/`notificationSlackChannels` aren't real `PipelineRun` fields — they're
 * registered by `MultiInputField` (inside `NotificationDetails`) purely so Form's built-in
 * validation gating gates submission on an invalid email, matching the Field API used
 * everywhere else in this form instead of a hand-rolled check in the submit handler.
 */
export type NotificationFormFields = {
  notificationEmails?: string[];
  notificationSlackChannels?: string[];
};
