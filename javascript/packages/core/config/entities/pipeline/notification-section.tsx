import { BooleanField } from '#core/components/form/fields/boolean/boolean-field';
import { StringField } from '#core/components/form/fields/string/string-field';
import { useField } from '#core/components/form/hooks/use-field';
import { ArrayFormRow } from '#core/components/form/layout/array-form-row/array-form-row';
import { regex } from '#core/components/form/validation/validators';

import type { NotificationEventType } from '#core/config/entities/run/types';

const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

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
          <ArrayFormRow
            name="Emails"
            rootFieldPath="notificationEmails"
            minItems={1}
            addLabel="Add email"
          >
            {(itemPath) => (
              <StringField
                name={`${itemPath}.value`}
                validate={regex(EMAIL_REGEX, 'Must be a valid email.')}
                placeholder="name@example.com"
              />
            )}
          </ArrayFormRow>

          <ArrayFormRow
            name="Slack Channels or Users"
            rootFieldPath="notificationSlackDestinations"
            minItems={1}
            addLabel="Add Slack channel or user"
          >
            {(itemPath) => (
              <StringField name={`${itemPath}.value`} placeholder="#channel or @user" />
            )}
          </ArrayFormRow>
        </>
      ) : null}
    </>
  );
};
