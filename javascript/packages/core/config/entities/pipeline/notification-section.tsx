import { BooleanField } from '#core/components/form/fields/boolean/boolean-field';
import { StringField } from '#core/components/form/fields/string/string-field';
import { useField } from '#core/components/form/hooks/use-field';

import type { FieldValidator } from '#core/components/form/validation/types';
import type { NotificationEventType } from '#core/config/entities/run/types';

const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

const validateEmails: FieldValidator = (value) => {
  const emails = Array.isArray(value) ? value : [];
  const allValid = emails.every((email) => typeof email === 'string' && EMAIL_REGEX.test(email));
  return allValid ? undefined : 'Must be a valid email.';
};

/**
 * Every implemented event a pipeline run can notify on. The form has no event-type picker —
 * whoever opts in gets notified for all of them — so this is the fixed `event_types` value
 * for every notification the form produces.
 */
export const ALL_PIPELINE_RUN_EVENT_TYPES: NotificationEventType[] = [
  'EVENT_TYPE_PIPELINE_RUN_STATE_STARTED',
  'EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED',
  'EVENT_TYPE_PIPELINE_RUN_STATE_FAILED',
  'EVENT_TYPE_PIPELINE_RUN_STATE_KILLED',
  'EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED',
];

export const NotificationSection = () => {
  const { input: notifyOnCompletion } = useField<boolean>('notifyOnCompletion');

  return (
    <>
      <BooleanField
        name="notifyOnCompletion"
        checkboxLabel="Do you want to receive notifications when pipeline run completed?"
        toggle
      />

      {notifyOnCompletion.value ? (
        <>
          <StringField
            name="notificationEmails"
            label="Emails"
            multi
            validate={validateEmails}
            placeholder="name@example.com"
          />

          <StringField
            name="notificationSlackDestinations"
            label="Slack Channels or Users"
            multi
            placeholder="#channel or @user"
          />
        </>
      ) : null}
    </>
  );
};
