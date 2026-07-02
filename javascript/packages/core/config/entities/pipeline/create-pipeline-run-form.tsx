import { useState } from 'react';
import { useStyletron } from 'baseui';
import { Checkbox, LABEL_PLACEMENT, STYLE_TYPE } from 'baseui/checkbox';

import { FormControl } from '#core/components/form/components/form-control';
import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { StringField } from '#core/components/form/fields/string/string-field';
import { TextareaField } from '#core/components/form/fields/textarea/textarea-field';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { NotificationDetails } from '#core/config/entities/pipeline/notification-details';
import { ALL_NOTIFICATION_EVENT_TYPES } from '#core/config/entities/run/types';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';
import { generateSuffix } from '#core/utils/name-utils';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { NotificationDetailsValue, Pipeline } from '#core/config/entities/pipeline/types';
import type { NotificationEventType, PipelineRun } from '#core/config/entities/run/types';

// Proto enum Notification.EventType numeric values (notification.proto).
const EVENT_TYPE_TO_NUMBER: Record<NotificationEventType, number> = {
  EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED: 1,
  EVENT_TYPE_PIPELINE_RUN_STATE_KILLED: 2,
  EVENT_TYPE_PIPELINE_RUN_STATE_FAILED: 3,
  EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED: 4,
};

export const CreatePipelineRunForm = ({ record, onClose }: ActionComponentProps<Pipeline>) => {
  const [css, theme] = useStyletron();
  const { projectId } = useStudioParams('base');

  const [notificationEnabled, setNotificationEnabled] = useState(false);
  const [notificationDetails, setNotificationDetails] = useState<NotificationDetailsValue>({
    emails: [],
    slackChannels: [],
    eventTypes: [...ALL_NOTIFICATION_EVENT_TYPES],
  });

  const createPipelineRunMutation = useStudioMutation<PipelineRun, PipelineRun>({
    mutationName: 'CreatePipelineRun',
  });

  const handleNotificationEnabledChange = (event: React.FormEvent<HTMLInputElement>) => {
    setNotificationEnabled(event.currentTarget.checked);
  };

  const handleNotificationDetailsChange = (details: NotificationDetailsValue) => {
    setNotificationDetails(details);
  };

  const handleRunSubmit = async (values: PipelineRun) => {
    if (createPipelineRunMutation.isPending) {
      return;
    }

    const { emails, slackChannels, eventTypes } = notificationDetails;

    const payload: PipelineRun = {
      ...values,
      spec: {
        ...values.spec,
        ...(notificationEnabled && (emails.length > 0 || slackChannels.length > 0)
          ? {
              notifications: [
                {
                  emails,
                  slackDestinations: slackChannels,
                  eventTypes: eventTypes.map((t) => EVENT_TYPE_TO_NUMBER[t]),
                },
              ],
            }
          : {}),
      },
    };

    await createPipelineRunMutation.mutateAsync(payload);
  };

  const initialValues: PipelineRun = {
    metadata: {
      name: `run${generateSuffix({ withDate: true })}`,
      namespace: projectId,
    },
    spec: {
      actor: {
        name: 'mastudio-user',
      },
      pipeline: {
        name: record?.metadata?.name ?? '',
        namespace: projectId,
      },
    },
  };

  return (
    <FormDialog<PipelineRun>
      isOpen
      onDismiss={onClose}
      heading="Start new pipeline run"
      onSubmit={handleRunSubmit}
      submitLabel={'Run'}
      initialValues={initialValues}
    >
      <StringField name="spec.pipeline.name" label="Pipeline to run" readOnly />

      <TextareaField
        name="spec.description"
        label="Description"
        placeholder="Enter a description for this run…"
        description="Optional. Helps identify this run in the pipeline run list."
      />

      <FormGroup
        title="Set Up Notifications"
        description="Receive an alert when this run completes, fails, or is killed."
      >
        <div
          className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale600 })}
        >
          <FormControl label="Send notifications">
            <Checkbox
              checked={notificationEnabled}
              onChange={handleNotificationEnabledChange}
              checkmarkType={STYLE_TYPE.toggle_round}
              labelPlacement={LABEL_PLACEMENT.right}
            >
              {notificationEnabled ? 'Enabled' : 'Disabled'}
            </Checkbox>
          </FormControl>
          <NotificationDetails
            enabled={notificationEnabled}
            value={notificationDetails}
            onChange={handleNotificationDetailsChange}
          />
        </div>
      </FormGroup>
    </FormDialog>
  );
};
