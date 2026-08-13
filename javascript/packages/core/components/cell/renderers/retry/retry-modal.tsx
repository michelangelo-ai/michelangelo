import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { TextareaField } from '#core/components/form/fields/textarea/textarea-field';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation/use-studio-mutation';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { RetryActionRecord, RetryFormValues } from './types';

export const RetryModal = ({ record, onClose }: ActionComponentProps<RetryActionRecord>) => {
  const { projectId, entityId } = useStudioParams('detail');

  const retryMutation = useStudioMutation<Record<string, unknown>, RetryFormValues>({
    mutationName: 'UpdatePipelineRun',
    successOperations: [
      {
        type: 'invalidate',
        targets: [
          { name: 'GetPipelineRun', serviceOptions: { namespace: projectId, name: entityId } },
        ],
      },
    ],
  });

  const initialValues: RetryFormValues = {
    metadata: record.metadata,
    spec: {
      ...record.spec,
      retryInfo: {
        activityId: record.activityId,
        workflowId: record.status.workflowId,
        workflowRunId: record.status.workflowRunId,
        reason: 'Manual retry from UI',
      },
    },
  };

  return (
    <FormDialog<RetryFormValues>
      isOpen
      onDismiss={onClose}
      heading="Retry Task"
      onSubmit={(values) => retryMutation.mutateAsync(values)}
      submitLabel="Retry Task"
      initialValues={initialValues}
    >
      <div>Are you sure you want to retry this task?</div>
      <TextareaField name="spec.retryInfo.reason" label="Retry Reason" />
    </FormDialog>
  );
};
