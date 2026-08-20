import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { SelectField } from '#core/components/form/fields/select/select-field';
import { StringField } from '#core/components/form/fields/string/string-field';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation/use-studio-mutation';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { generateSuffix } from '#core/utils/name-utils';
import { formatTriggerSchedule } from './format-trigger-schedule';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { SelectOption } from '#core/components/form/fields/select/types';
import type { ScheduledTriggerRun } from '#core/config/entities/trigger/types';
import type { Pipeline, SchedulePipelineFormValues } from './types';

/**
 * Schedules a pipeline to run regularly by starting one of the triggers declared in its
 * manifest.
 *
 * The trigger list is not part of the row data the action was opened from — only a
 * `GetPipeline` response carries `spec.manifest` — so the dropdown is populated from a
 * fetch made when the dialog opens.
 */
export const SchedulePipelineForm = ({ record, onClose }: ActionComponentProps<Pipeline>) => {
  const { projectId } = useStudioParams('base');
  const pipelineName = record?.metadata?.name ?? '';

  const { data, isLoading } = useStudioQuery<{ pipeline: Pipeline }>({
    queryName: 'GetPipeline',
    serviceOptions: { name: pipelineName },
    clientOptions: { enabled: !!pipelineName },
  });

  const triggerMap = data?.pipeline?.spec?.manifest?.triggerMap ?? {};

  const triggerOptions: SelectOption<string>[] = Object.keys(triggerMap)
    .sort()
    .map((name) => {
      const schedule = formatTriggerSchedule(triggerMap[name]);
      return { id: name, label: schedule ? `${name} — ${schedule}` : name };
    });

  const createTriggerRunMutation = useStudioMutation<ScheduledTriggerRun, ScheduledTriggerRun>({
    mutationName: 'CreateTriggerRun',
    successOperations: [{ type: 'invalidate', targets: ['ListTriggerRun'] }],
  });

  const handleScheduleSubmit = async ({ sourceTriggerName }: SchedulePipelineFormValues) => {
    if (createTriggerRunMutation.isPending) {
      return;
    }

    const trigger = triggerMap[sourceTriggerName];
    if (!trigger) {
      // Only reachable if the manifest changed under an open dialog. Surfacing it beats
      // sending a TriggerRun with no schedule, which the reconciler would leave inert.
      throw new Error(`Trigger "${sourceTriggerName}" is no longer defined on this pipeline.`);
    }

    await createTriggerRunMutation.mutateAsync({
      metadata: {
        name: buildTriggerRunName(sourceTriggerName),
        namespace: projectId,
      },
      spec: {
        pipeline: { name: pipelineName, namespace: projectId },
        trigger,
        sourceTriggerName,
      },
    });
  };

  return (
    <FormDialog<SchedulePipelineFormValues>
      isOpen
      onDismiss={onClose}
      heading="Schedule pipeline"
      onSubmit={handleScheduleSubmit}
      submitLabel="Schedule"
    >
      <StringField name="pipelineName" label="Pipeline" initialValue={pipelineName} readOnly />

      <SelectField
        name="sourceTriggerName"
        label="Trigger"
        placeholder="Select a trigger…"
        options={triggerOptions}
        isLoading={isLoading}
        required
        caption={
          !isLoading && triggerOptions.length === 0
            ? 'This pipeline declares no triggers. Add one to its manifest to schedule it.'
            : 'The pipeline runs on the schedule this trigger defines.'
        }
      />
    </FormDialog>
  );
};

/**
 * CRD names must be RFC 1123 labels, but manifest trigger names are free-form map keys.
 * Folding the trigger name into the prefix keeps the run identifiable in the "Triggered by"
 * column; a generic prefix covers names with nothing usable left after sanitizing.
 */
function buildTriggerRunName(sourceTriggerName: string): string {
  const prefix = sourceTriggerName
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 40);

  return `${prefix || 'trigger'}${generateSuffix({ withDate: true })}`;
}
