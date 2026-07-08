import { useState } from 'react';
import { Checkbox, LABEL_PLACEMENT, STYLE_TYPE } from 'baseui/checkbox';

import { FormControl } from '#core/components/form/components/form-control';
import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { StringField } from '#core/components/form/fields/string/string-field';
import { TextareaField } from '#core/components/form/fields/textarea/textarea-field';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { NotificationDetails } from '#core/config/entities/pipeline/notification-details';
import {
  ALL_NOTIFICATION_EVENT_TYPES,
  NOTIFICATION_EVENT_TYPES,
} from '#core/config/entities/run/types';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';
import { generateSuffix } from '#core/utils/name-utils';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { NotificationFormFields, Pipeline } from '#core/config/entities/pipeline/types';
import type { NotificationEventType, PipelineRun } from '#core/config/entities/run/types';

const EVENT_TYPE_TO_PROTO_VALUE: Record<NotificationEventType, number> = Object.fromEntries(
  NOTIFICATION_EVENT_TYPES.map(({ id, protoValue }) => [id, protoValue])
) as Record<NotificationEventType, number>;

export const CreatePipelineRunForm = ({ record, onClose }: ActionComponentProps<Pipeline>) => {
  const { projectId } = useStudioParams('base');

  const [notificationEnabled, setNotificationEnabled] = useState(false);
  const [eventTypes, setEventTypes] = useState<NotificationEventType[]>([
    ...ALL_NOTIFICATION_EVENT_TYPES,
  ]);

  const createPipelineRunMutation = useStudioMutation<PipelineRun, PipelineRun>({
    mutationName: 'CreatePipelineRun',
  });

  const handleNotificationEnabledChange = (event: React.FormEvent<HTMLInputElement>) => {
    setNotificationEnabled(event.currentTarget.checked);
  };

  const handleEventTypeSelectionChange = (types: NotificationEventType[]) => {
    setEventTypes(types);
  };

  const handleRunSubmit = async (values: PipelineRun & NotificationFormFields) => {
    if (createPipelineRunMutation.isPending) {
      return;
    }

    const { notificationEmails = [], notificationSlackChannels = [], ...pipelineRun } = values;

    const payload: PipelineRun = {
      ...pipelineRun,
      spec: {
        ...pipelineRun.spec,
        ...(notificationEnabled &&
        (notificationEmails.length > 0 || notificationSlackChannels.length > 0)
          ? {
              notifications: [
                {
                  emails: notificationEmails,
                  slackDestinations: notificationSlackChannels,
                  eventTypes: eventTypes.map((t) => EVENT_TYPE_TO_PROTO_VALUE[t]),
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
    <FormDialog<PipelineRun & NotificationFormFields>
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
          eventTypes={eventTypes}
          onEventTypesChange={handleEventTypeSelectionChange}
        />
      </FormGroup>
    </FormDialog>
  );
};
