import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { StringField } from '#core/components/form/fields/string/string-field';
import { TextareaField } from '#core/components/form/fields/textarea/textarea-field';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation/use-studio-mutation';
import { generateSuffix } from '#core/utils/name-utils';
import { ResumeRunFields } from './resume-run-fields';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { Pipeline } from '#core/config/entities/pipeline/types';
import type { PipelineRun } from '#core/config/entities/run/types';

export const CreatePipelineRunForm = ({ record, onClose }: ActionComponentProps<Pipeline>) => {
  const { projectId } = useStudioParams('base');
  const pipelineName = record?.metadata?.name ?? '';

  const createPipelineRunMutation = useStudioMutation<PipelineRun, PipelineRun>({
    mutationName: 'CreatePipelineRun',
  });

  const handleRunSubmit = async (values: PipelineRun) => {
    if (createPipelineRunMutation.isPending) {
      return;
    }

    await createPipelineRunMutation.mutateAsync(buildPayload(values, projectId));
  };

  const initialValues: PipelineRun = {
    metadata: {
      name: `run${generateSuffix({ withDate: true })}`,
      namespace: projectId,
    },
    spec: {
      pipeline: {
        name: pipelineName,
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

      <ResumeRunFields pipelineName={pipelineName} />

      <TextareaField
        name="spec.description"
        label="Description"
        placeholder="Enter a description for this run…"
        description="Optional. Helps identify this run in the pipeline run list."
      />
    </FormDialog>
  );
};

/**
 * Normalizes the resume spec before submission.
 *
 * The resume fields live in a collapsible group the user may open and close without
 * choosing anything, so the form can carry a `resume` branch holding no source run.
 * `Resume.pipeline_run` is a required field, so that branch has to be dropped entirely
 * rather than sent half-populated. The namespace is attached here instead of being bound
 * to a hidden field, since it is always the current project.
 */
function buildPayload(values: PipelineRun, projectId: string): PipelineRun {
  const sourceRunName = values.spec?.resume?.pipelineRun?.name;
  const { resume: _resume, ...spec } = values.spec;

  if (!sourceRunName) {
    return { ...values, spec };
  }

  const resumeFrom = values.spec.resume?.resumeFrom ?? [];

  return {
    ...values,
    spec: {
      ...spec,
      resume: {
        pipelineRun: { name: sourceRunName, namespace: projectId },
        ...(resumeFrom.length > 0 && { resumeFrom }),
      },
    },
  };
}
