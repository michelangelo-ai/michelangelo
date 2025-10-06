import React, { useState } from 'react';
import { Button, KIND, SIZE } from 'baseui/button';

import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { StringField } from '#core/components/form/fields/string/string-field';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';
import { generateSuffix } from '#core/utils/name-utils';

import type { Pipeline } from '#core/config/entities/pipeline/types';

export const CreatePipelineRunDialog: React.FC<{ record: Pipeline }> = ({ record }) => {
  const [showRunModal, setShowRunModal] = useState(false);
  const { projectId } = useStudioParams('base');

  const createPipelineRunMutation = useStudioMutation({ mutationName: 'CreatePipelineRun' });

  const handleRunSubmit = async (values: Record<string, unknown>) => {
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
        name: record?.metadata?.name || '',
        namespace: projectId,
      },
    },
  };

  return (
    <>
      <Button size={SIZE.compact} kind={KIND.secondary} onClick={() => setShowRunModal(true)}>
        Run
      </Button>

      <FormDialog
        isOpen={showRunModal}
        onDismiss={() => setShowRunModal(false)}
        heading="Start new pipeline run"
        onSubmit={handleRunSubmit}
        submitLabel={'Run'}
        initialValues={initialValues}
      >
        <StringField name="spec.pipeline.name" label="Pipeline to run" readOnly />
      </FormDialog>
    </>
  );
};
