import { useStyletron } from 'baseui';
import { Checkbox } from 'baseui/checkbox';
import { Select } from 'baseui/select';

import { FormControl } from '#core/components/form/components/form-control';

import type { OnChangeParams } from 'baseui/select';
import type { NotificationDetailsValue } from '#core/config/entities/pipeline/types';
import type { NotificationEventType } from '#core/config/entities/run/types';

const EVENT_TYPE_OPTIONS: Array<{ id: NotificationEventType; label: string }> = [
  { id: 'EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED', label: 'Succeeded' },
  { id: 'EVENT_TYPE_PIPELINE_RUN_STATE_FAILED', label: 'Failed' },
  { id: 'EVENT_TYPE_PIPELINE_RUN_STATE_KILLED', label: 'Killed' },
  { id: 'EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED', label: 'Skipped' },
];

const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

type NotificationDetailsProps = {
  enabled: boolean;
  value: NotificationDetailsValue;
  onNotificationDetailsChange: (value: NotificationDetailsValue) => void;
};

const toSelectValue = (items: string[]) => items.map((item) => ({ id: item, label: item }));
const fromSelectParams = (params: OnChangeParams) => params.value.map((item) => String(item.id));

/**
 * Controlled notification recipient inputs (emails, Slack, event types).
 * Pure UI state driven entirely by props — not react-final-form — so the
 * caller decides when and whether to merge this into a submitted payload.
 */
export function NotificationDetails({
  enabled,
  value,
  onNotificationDetailsChange,
}: NotificationDetailsProps) {
  const [css, theme] = useStyletron();

  const emailError = value.emails.some((email) => !EMAIL_REGEX.test(email.trim()))
    ? 'Enter valid email addresses, e.g. user@example.com.'
    : undefined;

  const toggleEventType = (id: NotificationEventType) => {
    onNotificationDetailsChange({
      ...value,
      eventTypes: value.eventTypes.includes(id)
        ? value.eventTypes.filter((t) => t !== id)
        : [...value.eventTypes, id],
    });
  };

  return (
    <>
      <FormControl label="Email addresses" error={emailError}>
        <Select
          id="notificationEmails"
          value={toSelectValue(value.emails)}
          options={[]}
          onChange={(params) =>
            onNotificationDetailsChange({ ...value, emails: fromSelectParams(params) })
          }
          disabled={!enabled}
          multi
          creatable
          placeholder="user@example.com"
        />
      </FormControl>

      <FormControl
        label="Slack channels or users"
        caption="Use a channel name or @person for direct messages."
      >
        <Select
          id="notificationSlackChannels"
          value={toSelectValue(value.slackChannels)}
          options={[]}
          onChange={(params) =>
            onNotificationDetailsChange({ ...value, slackChannels: fromSelectParams(params) })
          }
          disabled={!enabled}
          multi
          creatable
          placeholder="e.g. #channel or @username"
        />
      </FormControl>

      <FormControl
        label="Notify on"
        caption="At least one state must be selected to receive notifications."
      >
        <div className={css({ display: 'flex', flexWrap: 'wrap', gap: theme.sizing.scale600 })}>
          {EVENT_TYPE_OPTIONS.map((option) => (
            <Checkbox
              key={option.id}
              checked={value.eventTypes.includes(option.id)}
              disabled={!enabled}
              onChange={() => toggleEventType(option.id)}
            >
              {option.label}
            </Checkbox>
          ))}
        </div>
      </FormControl>
    </>
  );
}
