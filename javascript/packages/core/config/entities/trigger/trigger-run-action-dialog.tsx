import React, { useState } from 'react';
import { Button, KIND, SIZE } from 'baseui/button';

import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { StringField } from '#core/components/form/fields/string/string-field';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';

import type { TriggerRun } from '#core/config/entities/trigger/types';
import { TriggerRunAction } from '#core/config/entities/trigger/types';

export interface TriggerRunActionDialogProps {
  record: TriggerRun;
  action: 'kill' | 'pause' | 'resume';
}

const ACTION_CONFIG = {
  kill: {
    label: 'Kill',
    heading: 'Kill Trigger Run',
    description: 'This will terminate the trigger run and any associated pipeline runs.',
    buttonKind: KIND.primary,
    submitLabel: 'Kill',
  },
  pause: {
    label: 'Pause',
    heading: 'Pause Trigger Run',
    description: 'This will pause the recurring trigger to prevent new executions.',
    buttonKind: KIND.secondary,
    submitLabel: 'Pause',
  },
  resume: {
    label: 'Resume',
    heading: 'Resume Trigger Run',
    description: 'This will resume the paused trigger to allow new executions.',
    buttonKind: KIND.secondary,
    submitLabel: 'Resume',
  },
} as const;

export const TriggerRunActionDialog: React.FC<TriggerRunActionDialogProps> = ({ record, action }) => {
  const [showModal, setShowModal] = useState(false);
  const { projectId } = useStudioParams('base');

  const config = ACTION_CONFIG[action];

  const updateTriggerRunMutation = useStudioMutation<
    { triggerRun: TriggerRun },
    { triggerRun: TriggerRun }
  >({ mutationName: 'UpdateTriggerRun' });

  const handleActionSubmit = async (values: TriggerRun) => {
    if (updateTriggerRunMutation.isPending) {
      return;
    }

    await updateTriggerRunMutation.mutateAsync({ triggerRun: values });
  };

  // Create the trigger run spec with the appropriate action
  const createTriggerRunWithAction = (baseRecord: TriggerRun): TriggerRun => {
    return {
      ...baseRecord,
      spec: {
        ...baseRecord.spec,
        // Use only the action field (clean approach, deprecated boolean fields not set)
        action: action === 'kill' ? TriggerRunAction.KILL :
                action === 'pause' ? TriggerRunAction.PAUSE :
                TriggerRunAction.RESUME,
      },
    };
  };

  const initialValues = createTriggerRunWithAction(record);

  return (
    <>
      <Button
        size={SIZE.compact}
        kind={config.buttonKind}
        onClick={() => setShowModal(true)}
      >
        {config.label}
      </Button>

      <FormDialog<TriggerRun>
        isOpen={showModal}
        onDismiss={() => setShowModal(false)}
        heading={config.heading}
        onSubmit={handleActionSubmit}
        submitLabel={config.submitLabel}
        initialValues={initialValues}
      >
        <StringField
          name="metadata.name"
          label="Trigger Run Name"
          readOnly
        />
        <div style={{ marginTop: '16px', fontSize: '14px', color: '#666' }}>
          {config.description}
        </div>
      </FormDialog>
    </>
  );
};