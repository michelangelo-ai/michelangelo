import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { StringField } from '#core/components/form/fields/string/string-field';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';
import { TriggerRunAction } from './types';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { TriggerRun } from './types';

const ACTION_TO_ENUM = {
  kill: TriggerRunAction.KILL,
  pause: TriggerRunAction.PAUSE,
  resume: TriggerRunAction.RESUME,
} as const;

const ACTION_CONFIG = {
  kill: { heading: 'Kill Trigger Run', submitLabel: 'Kill' },
  pause: { heading: 'Pause Trigger Run', submitLabel: 'Pause' },
  resume: { heading: 'Resume Trigger Run', submitLabel: 'Resume' },
} as const;

type Action = keyof typeof ACTION_CONFIG;

function TriggerRunActionForm({
  record,
  isOpen,
  onClose,
  action,
}: ActionComponentProps<TriggerRun> & { action: Action }) {
  const config = ACTION_CONFIG[action];

  const mutation = useStudioMutation<{ triggerRun: TriggerRun }, { triggerRun: TriggerRun }>({
    mutationName: 'UpdateTriggerRun',
  });

  const handleSubmit = async (values: TriggerRun) => {
    if (mutation.isPending) return;
    await mutation.mutateAsync({ triggerRun: values });
  };

  const initialValues: TriggerRun = {
    ...record,
    spec: { ...record.spec, action: ACTION_TO_ENUM[action] },
  };

  return (
    <FormDialog<TriggerRun>
      isOpen={isOpen}
      onDismiss={onClose}
      heading={config.heading}
      onSubmit={handleSubmit}
      submitLabel={config.submitLabel}
      initialValues={initialValues}
    >
      <StringField name="metadata.name" label="Trigger Run Name" readOnly />
    </FormDialog>
  );
}

export const KillTriggerRunForm = (props: ActionComponentProps<TriggerRun>) => (
  <TriggerRunActionForm {...props} action="kill" />
);

export const PauseTriggerRunForm = (props: ActionComponentProps<TriggerRun>) => (
  <TriggerRunActionForm {...props} action="pause" />
);

export const ResumeTriggerRunForm = (props: ActionComponentProps<TriggerRun>) => (
  <TriggerRunActionForm {...props} action="resume" />
);
