import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { StringField } from '#core/components/form/fields/string/string-field';
import { TextareaField } from '#core/components/form/fields/textarea/textarea-field';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';
import { generateSuffix } from '#core/utils/name-utils';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { Pipeline } from '#core/config/entities/pipeline/types';
import type { PipelineRun } from '#core/config/entities/run/types';

export const CreatePipelineRunForm = ({ record, onClose }: ActionComponentProps<Pipeline>) => {
  const { projectId } = useStudioParams('base');

  const createPipelineRunMutation = useStudioMutation<PipelineRun, PipelineRun>({
    mutationName: 'CreatePipelineRun',
  });

  const handleRunSubmit = async (values: PipelineRun) => {
    if (createPipelineRunMutation.isPending) {
      return;
    }

    await createPipelineRunMutation.mutateAsync(values);
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
    </FormDialog>
  );
};
