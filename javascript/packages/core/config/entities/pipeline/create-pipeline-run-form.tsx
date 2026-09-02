import { Field } from 'react-final-form';

import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { BooleanField } from '#core/components/form/fields/boolean/boolean-field';
import { InlineRadioField } from '#core/components/form/fields/radio/inline-radio-field';
import { StringField } from '#core/components/form/fields/string/string-field';
import { TextareaField } from '#core/components/form/fields/textarea/textarea-field';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { validateEmails } from '#core/config/entities/pipeline/utils/validate-emails';
import {
  NotificationEventType,
  NotificationResourceType,
  NotificationType,
} from '#core/config/entities/run/types';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation/use-studio-mutation';
import { ENVIRONMENT_LABEL_KEY } from '#core/utils/environment-utils';
import { generateSuffix } from '#core/utils/name-utils';
import { ResumeRunFields } from './resume-run-fields';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { Pipeline, PipelineRunFormValues } from '#core/config/entities/pipeline/types';
import type { PipelineRun, PipelineRunNotification } from '#core/config/entities/run/types';

/**
 * Every implemented event a pipeline run can notify on. The form has no event-type picker —
 * whoever opts in gets notified for all of them — so this is the fixed `eventTypes` value
 * for every notification the form produces.
 */
export const ALL_PIPELINE_RUN_EVENT_TYPES: NotificationEventType[] = [
  NotificationEventType.PIPELINE_RUN_STATE_STARTED,
  NotificationEventType.PIPELINE_RUN_STATE_SUCCEEDED,
  NotificationEventType.PIPELINE_RUN_STATE_FAILED,
  NotificationEventType.PIPELINE_RUN_STATE_KILLED,
  NotificationEventType.PIPELINE_RUN_STATE_SKIPPED,
];

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
      metadata: payload.metadata,
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

      <InlineRadioField
        name={`metadata.labels.${ENVIRONMENT_LABEL_KEY}`}
        label="Which environment do you want to use?"
        required
        options={[
          { value: 'development', label: 'Development' },
          { value: 'production', label: 'Production' },
        ]}
      />

      <TextareaField
        name="spec.description"
        label="Description"
        placeholder="Enter a description for this run…"
        description="Optional. Helps identify this run in the pipeline run list."
      />

      <ResumeRunFields pipelineName={pipelineName} />

      <FormGroup title="Set Up Notifications (Optional)">
        <BooleanField
          name="notifyOnCompletion"
          checkboxLabel="Do you want to receive notifications when pipeline run completed?"
          toggle
        />

        <Field name="notifyOnCompletion" subscription={{ value: true }}>
          {({ input }) =>
            input.value ? (
              <>
                <StringField
                  name="notificationEmails"
                  label="Emails"
                  multi
                  validate={validateEmails}
                  placeholder="e.g., name@example.com"
                />

                <StringField
                  name="notificationSlackDestinations"
                  label="Slack Channels or Users"
                  multi
                  placeholder="e.g., #channel or @user"
                />
              </>
            ) : null
          }
        </Field>
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
  const nonEmptyValues = (entries: string[] | undefined): string[] =>
    entries?.filter((value) => value.trim() !== '') ?? [];

  const emails = nonEmptyValues(values.notificationEmails);
  const slackDestinations = nonEmptyValues(values.notificationSlackDestinations);

  const notifications: PipelineRunNotification[] = [];

  if (emails.length > 0) {
    notifications.push({
      notificationType: NotificationType.EMAIL,
      eventTypes: ALL_PIPELINE_RUN_EVENT_TYPES,
      resourceType: NotificationResourceType.PIPELINE_RUN,
      emails,
      slackDestinations: [],
    });
  }

  if (slackDestinations.length > 0) {
    notifications.push({
      notificationType: NotificationType.SLACK,
      eventTypes: ALL_PIPELINE_RUN_EVENT_TYPES,
      resourceType: NotificationResourceType.PIPELINE_RUN,
      emails: [],
      slackDestinations,
    });
  }

  return notifications;
}
