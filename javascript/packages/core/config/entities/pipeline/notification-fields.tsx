import { SelectField } from '#core/components/form/fields/select/select-field';
import { StringField } from '#core/components/form/fields/string/string-field';
import { useField } from '#core/components/form/hooks/use-field';
import { ArrayFormRow } from '#core/components/form/layout/array-form-row/array-form-row';
import { combineValidators } from '#core/components/form/validation/combine-validators';
import { regex, required } from '#core/components/form/validation/validators';

import type { NotificationType } from '#core/config/entities/run/types';

const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

const NOTIFICATION_TYPE_OPTIONS = [
  { id: 'NOTIFICATION_TYPE_EMAIL', label: 'Email' },
  { id: 'NOTIFICATION_TYPE_SLACK', label: 'Slack' },
];

const EVENT_TYPE_OPTIONS = [
  { id: 'EVENT_TYPE_PIPELINE_RUN_STATE_STARTED', label: 'Started' },
  { id: 'EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED', label: 'Succeeded' },
  { id: 'EVENT_TYPE_PIPELINE_RUN_STATE_FAILED', label: 'Failed' },
  { id: 'EVENT_TYPE_PIPELINE_RUN_STATE_KILLED', label: 'Killed' },
  { id: 'EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED', label: 'Skipped' },
];

export const NotificationFields = ({ indexedFieldPath }: { indexedFieldPath: string }) => {
  const { input: notificationType } = useField<NotificationType>(
    `${indexedFieldPath}.notification_type`
  );

  return (
    <>
      <SelectField
        name={`${indexedFieldPath}.notification_type`}
        label="Notification type"
        options={NOTIFICATION_TYPE_OPTIONS}
        required
        clearable={false}
      />
      <SelectField
        name={`${indexedFieldPath}.event_types`}
        label="Notify me when the run"
        options={EVENT_TYPE_OPTIONS}
        multi
        required
      />
      {notificationType.value === 'NOTIFICATION_TYPE_SLACK' ? (
        <ArrayFormRow
          key="slack"
          name="Slack channels"
          rootFieldPath={`${indexedFieldPath}.slack_destinations`}
          minItems={1}
          addLabel="Add Slack channel"
        >
          {(itemPath) => (
            <StringField
              name={`${itemPath}.value`}
              label="Slack channel"
              required
              placeholder="#channel"
            />
          )}
        </ArrayFormRow>
      ) : (
        <ArrayFormRow
          key="email"
          name="Emails"
          rootFieldPath={`${indexedFieldPath}.emails`}
          minItems={1}
          addLabel="Add email"
        >
          {(itemPath) => (
            <StringField
              name={`${itemPath}.value`}
              label="Email"
              validate={combineValidators(required(), regex(EMAIL_REGEX, 'Must be a valid email.'))}
              placeholder="name@example.com"
            />
          )}
        </ArrayFormRow>
      )}
    </>
  );
};
