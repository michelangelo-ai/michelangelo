import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { StringField } from '#core/components/form/fields/string/string-field';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';
import { generateSuffix } from '#core/utils/name-utils';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { Pipeline } from '#core/config/entities/pipeline/types';
import type { PipelineRun } from '#core/config/entities/run/types';

export const CreatePipelineRunForm = ({
  record: pipeline,
  isOpen,
  onClose,
}: ActionComponentProps<Pipeline>) => {
  const { projectId } = useStudioParams('base');

  const createPipelineRunMutation = useStudioMutation<
    { pipelineRun: PipelineRun },
    { pipelineRun: PipelineRun }
  >({ mutationName: 'CreatePipelineRun' });

  const handleRunSubmit = async (values: PipelineRun) => {
    if (createPipelineRunMutation.isPending) {
      return;
    }

    await createPipelineRunMutation.mutateAsync({ pipelineRun: values });
  };

  const initialValues = {
    metadata: {
      name: `run${generateSuffix({ withDate: true })}`,
      namespace: projectId,
    },
    spec: {
      actor: {
        name: 'mastudio-user',
      },
      pipeline: {
        name: pipeline?.metadata?.name || '',
        namespace: projectId,
      },
    },
  };

  return (
    <FormDialog<PipelineRun>
      isOpen={isOpen}
      onDismiss={onClose}
      heading="Start new pipeline run"
      onSubmit={handleRunSubmit}
      submitLabel="Run"
      initialValues={initialValues}
    >
      <StringField name="spec.pipeline.name" label="Pipeline to run" readOnly />
    </FormDialog>
  );
};
