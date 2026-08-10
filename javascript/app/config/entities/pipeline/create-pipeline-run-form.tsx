import {
  FormDialog,
  StringField,
  TextareaField,
  useStudioMutation,
  useStudioParams,
} from '@michelangelo-ai/core';

import { generateSuffix } from './name-utils';

import type { ActionComponentProps } from '@michelangelo-ai/core';
import type { Pipeline } from '@michelangelo-ai/rpc/resources/pipeline';
import type { PipelineRun } from '@michelangelo-ai/rpc/resources/pipeline-run';

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
