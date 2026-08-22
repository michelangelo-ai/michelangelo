import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { StringField } from '#core/components/form/fields/string/string-field';
import { TextareaField } from '#core/components/form/fields/textarea/textarea-field';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import {
  ALL_PIPELINE_RUN_EVENT_TYPES,
  NotificationSection,
} from '#core/config/entities/pipeline/notification-section';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation/use-studio-mutation';
import { generateSuffix } from '#core/utils/name-utils';
import { ResumeRunFields } from './resume-run-fields';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { Pipeline, PipelineRunFormValues } from '#core/config/entities/pipeline/types';
import type { PipelineRun, PipelineRunNotification } from '#core/config/entities/run/types';

export const CreatePipelineRunForm = ({ record, onClose }: ActionComponentProps<Pipeline>) => {
  const { projectId } = useStudioParams('base');
  const pipelineName = record?.metadata?.name ?? '';

  const createPipelineRunMutation = useStudioMutation<PipelineRun, PipelineRun>({
    mutationName: 'CreatePipelineRun',
  });

  const handleRunSubmit = async (values: PipelineRunFormValues) => {
    if (createPipelineRunMutation.isPending) {
      return;
    }

    const payload = buildPayload(values, projectId);
    await createPipelineRunMutation.mutateAsync({
      ...payload,
      spec: {
        ...payload.spec,
        notifications: values.notifyOnCompletion ? buildNotifications(values) : [],
      },
    });
  };

  const initialValues: PipelineRunFormValues = {
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
    <FormDialog<PipelineRunFormValues>
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

      <FormGroup title="Set Up Notifications (Optional)">
        <NotificationSection />
      </FormGroup>
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

/**
 * Turns the form's email/Slack lists into the `Notification` messages the API expects: one
 * per destination type that has at least one non-empty entry, each covering every event type
 * since the form has no event-type picker.
 */
function buildNotifications(values: PipelineRunFormValues): PipelineRunNotification[] {
  // An untouched ArrayFormRow item is pushed as `{}`, not `{ value: '' }` — `value` can be
  // undefined here even though the field type says otherwise.
  const nonEmptyValues = (entries: { value?: string }[] | undefined): string[] =>
    entries?.map(({ value }) => value ?? '').filter((value) => value.trim() !== '') ?? [];

  const emails = nonEmptyValues(values.notificationEmails);
  const slackDestinations = nonEmptyValues(values.notificationSlackDestinations);

  const notifications: PipelineRunNotification[] = [];

  if (emails.length > 0) {
    notifications.push({
      notification_type: 'NOTIFICATION_TYPE_EMAIL',
      event_types: ALL_PIPELINE_RUN_EVENT_TYPES,
      resource_type: 'RESOURCE_TYPE_PIPELINE_RUN',
      emails,
      slack_destinations: [],
    });
  }

  if (slackDestinations.length > 0) {
    notifications.push({
      notification_type: 'NOTIFICATION_TYPE_SLACK',
      event_types: ALL_PIPELINE_RUN_EVENT_TYPES,
      resource_type: 'RESOURCE_TYPE_PIPELINE_RUN',
      emails: [],
      slack_destinations: slackDestinations,
    });
  }

  return notifications;
}
