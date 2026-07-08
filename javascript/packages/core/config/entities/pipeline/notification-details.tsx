import { useStyletron } from 'baseui';
import { Checkbox } from 'baseui/checkbox';

import { FormControl } from '#core/components/form/components/form-control';
import { MultiInputField } from '#core/components/form/fields/multi-input/multi-input-field';
import { validateEmails } from '#core/config/entities/pipeline/notification-validation';
import { NOTIFICATION_EVENT_TYPES } from '#core/config/entities/run/types';

import type { NotificationEventType } from '#core/config/entities/run/types';

type NotificationDetailsProps = {
  enabled: boolean;
  eventTypes: NotificationEventType[];
  onEventTypesChange: (eventTypes: NotificationEventType[]) => void;
};

/**
 * Notification recipient inputs (emails, Slack, event types) for `CreatePipelineRunForm`.
 * Emails and Slack destinations are real form fields (registered via `MultiInputField`), so
 * `Form`'s built-in validation gating blocks submission on an invalid email automatically.
 * Event types are pure UI state, passed through to the parent to merge into the payload.
 */
export function NotificationDetails({
  enabled,
  eventTypes,
  onEventTypesChange,
}: NotificationDetailsProps) {
  const [css, theme] = useStyletron();

  const toggleEventType = (id: NotificationEventType) => {
    onEventTypesChange(
      eventTypes.includes(id) ? eventTypes.filter((t) => t !== id) : [...eventTypes, id]
    );
  };

  return (
    <>
      <MultiInputField
        name="notificationEmails"
        label="Email addresses"
        placeholder="e.g. user@example.com"
        disabled={!enabled}
        validate={validateEmails}
      />

      <MultiInputField
        name="notificationSlackChannels"
        label="Slack channels or users"
        caption="Use a channel name or @person for direct messages."
        placeholder="e.g. #channel or @username"
        disabled={!enabled}
      />

      <FormControl
        label="Notify on"
        caption="At least one state must be selected to receive notifications."
      >
        <div className={css({ display: 'flex', flexWrap: 'wrap', gap: theme.sizing.scale600 })}>
          {NOTIFICATION_EVENT_TYPES.map((option) => (
            <Checkbox
              key={option.id}
              checked={eventTypes.includes(option.id)}
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
