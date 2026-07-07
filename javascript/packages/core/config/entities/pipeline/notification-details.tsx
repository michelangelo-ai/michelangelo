import { useStyletron } from 'baseui';
import { Checkbox } from 'baseui/checkbox';
import { Select } from 'baseui/select';

import { FormControl } from '#core/components/form/components/form-control';
import { getEmailValidationError } from '#core/config/entities/pipeline/notification-validation';
import { NOTIFICATION_EVENT_TYPES } from '#core/config/entities/run/types';

import type { OnChangeParams } from 'baseui/select';
import type { NotificationDetailsValue } from '#core/config/entities/pipeline/types';
import type { NotificationEventType } from '#core/config/entities/run/types';

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

  const emailError = getEmailValidationError(value.emails);

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
          placeholder="e.g. user@example.com"
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
          {NOTIFICATION_EVENT_TYPES.map((option) => (
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
