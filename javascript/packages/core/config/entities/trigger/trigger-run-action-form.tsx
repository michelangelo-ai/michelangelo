import { useQueryClient } from '@tanstack/react-query';

import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { StringField } from '#core/components/form/fields/string/string-field';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';
import { TriggerRunAction } from './types';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { TriggerRun } from './types';

const ACTION_TO_ENUM = {
  kill: TriggerRunAction.KILL,
} as const;

const ACTION_CONFIG = {
  kill: { heading: 'Kill Trigger Run', submitLabel: 'Kill' },
} as const;

function TriggerRunActionForm({
  record,
  onClose,
  action,
}: ActionComponentProps<TriggerRun> & { action: keyof typeof ACTION_CONFIG }) {
  const queryClient = useQueryClient();

  const config = ACTION_CONFIG[action];

  const mutation = useStudioMutation<TriggerRun, TriggerRun>({
    mutationName: 'UpdateTriggerRun',
  });

  const initialValues: TriggerRun = {
    ...record,
    spec: { ...record.spec, action: ACTION_TO_ENUM[action] },
  };

  const handleSubmit = async (values: TriggerRun) => {
    await mutation.mutateAsync(values);

    // useStudioMutation auto-invalidates GetTriggerRun + ListTriggerRun on settle.
    // Re-invalidate after a delay so the refetch happens once the backend has had
    // time to process the kill action — auto-invalidation alone refetches stale state.
    setTimeout(() => {
      void queryClient.invalidateQueries({
        queryKey: [
          'GetTriggerRun',
          { name: record.metadata.name, namespace: record.metadata.namespace },
        ],
      });
      void queryClient.invalidateQueries({ queryKey: ['ListTriggerRun'] });
    }, 2000);
  };

  return (
    <FormDialog<TriggerRun>
      isOpen
      onDismiss={onClose}
      heading={config.heading}
      onSubmit={handleSubmit}
      submitLabel={config.submitLabel}
      initialValues={initialValues}
    >
      <p>
        Kill run <strong>{record.metadata.name}</strong> in pipeline{' '}
        <strong>{record.spec.pipeline.name}</strong>? This action cannot be undone.
      </p>
      <StringField name="metadata.name" label="Trigger Run Name" readOnly />
    </FormDialog>
  );
}

export const KillTriggerRunForm = (props: ActionComponentProps<TriggerRun>) => (
  <TriggerRunActionForm {...props} action="kill" />
);
