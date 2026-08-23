import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { SelectField } from '#core/components/form/fields/select/select-field';
import { StringField } from '#core/components/form/fields/string/string-field';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation/use-studio-mutation';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { generateSuffix } from '#core/utils/name-utils';
import { formatTriggerSchedule } from './format-trigger-schedule';
import { RunTriggerFields } from './run-trigger-fields';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { SelectOption } from '#core/components/form/fields/select/types';
import type { ManifestTrigger, RunTriggerPayload } from '#core/config/entities/trigger/types';
import type { Pipeline, RunTriggerFormValues } from './types';

/**
 * Runs a pipeline from one of the triggers declared in its manifest — either on the
 * trigger's own schedule, or as a one-off backfill over a chosen time window.
 *
 * The trigger list is not part of the row data the action was opened from — only a
 * `GetPipeline` response carries `spec.manifest` — so the dropdown is populated from a
 * fetch made when the dialog opens.
 */
export const RunTriggerForm = ({ record, onClose }: ActionComponentProps<Pipeline>) => {
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

  const createTriggerRunMutation = useStudioMutation<RunTriggerPayload, RunTriggerPayload>({
    mutationName: 'CreateTriggerRun',
    successOperations: [{ type: 'invalidate', targets: ['ListTriggerRun'] }],
  });

  const handleRunSubmit = async (values: RunTriggerFormValues) => {
    if (createTriggerRunMutation.isPending) {
      return;
    }

    const sourceTrigger = triggerMap[values.sourceTriggerName];
    if (!sourceTrigger) {
      // Only reachable if the manifest changed under an open dialog. Surfacing it beats
      // sending a TriggerRun with no schedule, which the reconciler would leave inert.
      throw new Error(
        `Trigger "${values.sourceTriggerName}" is no longer defined on this pipeline.`
      );
    }

    await createTriggerRunMutation.mutateAsync({
      metadata: {
        name: buildTriggerRunName(sourceTrigger, values.isBackfill),
        namespace: projectId,
      },
      spec: {
        pipeline: { name: pipelineName, namespace: projectId },
        trigger: buildTriggerOverride(sourceTrigger, values),
        sourceTriggerName: values.sourceTriggerName,
        autoFlip: !!values.autoFlip,
        ...buildBackfillWindow(values),
      },
    });
  };

  return (
    <FormDialog<RunTriggerFormValues>
      isOpen
      onDismiss={onClose}
      heading="Run trigger"
      onSubmit={handleRunSubmit}
      submitLabel="Run"
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
            ? 'This pipeline declares no triggers. Add one to its manifest to run it.'
            : 'The pipeline runs on the schedule this trigger defines.'
        }
      />

      <RunTriggerFields triggerMap={triggerMap} />
    </FormDialog>
  );
};

/**
 * Names the created TriggerRun after the type of run it represents, so it's identifiable in
 * the "Triggered by" column: a backfill takes priority over the trigger's own schedule type,
 * since a backfill run of a cron trigger is still a backfill, not a cron run.
 */
function buildTriggerRunName(trigger: ManifestTrigger, isBackfill: boolean | undefined): string {
  const typePrefix = resolveTriggerRunTypePrefix(trigger, isBackfill);
  return `${typePrefix}${generateSuffix({ withDate: true })}`;
}

function resolveTriggerRunTypePrefix(
  trigger: ManifestTrigger,
  isBackfill: boolean | undefined
): string {
  if (isBackfill) return 'backfill';
  switch (trigger.triggerType?.case) {
    case 'batchRerun':
      return 'batch-rerun';
    case 'intervalSchedule':
      return 'interval';
    default:
      return 'cron';
  }
}

function buildTriggerOverride(
  sourceTrigger: ManifestTrigger,
  values: RunTriggerFormValues
): ManifestTrigger {
  if (!values.isBackfill) {
    return sourceTrigger;
  }

  const trigger: ManifestTrigger = { ...sourceTrigger };

  if (values.maxConcurrencyOverride != null) {
    trigger.maxConcurrency = values.maxConcurrencyOverride;
  }

  if (values.selectedParams?.length && sourceTrigger.parametersMap) {
    trigger.parametersMap = Object.fromEntries(
      Object.entries(sourceTrigger.parametersMap).filter(([paramId]) =>
        values.selectedParams?.includes(paramId)
      )
    );
  }

  return trigger;
}

function buildBackfillWindow(
  values: RunTriggerFormValues
): Pick<RunTriggerPayload['spec'], 'startTimestamp' | 'endTimestamp'> {
  if (!values.isBackfill || !values.startTimestamp || !values.endTimestamp) {
    return {};
  }

  return {
    startTimestamp: { seconds: values.startTimestamp },
    endTimestamp: { seconds: values.endTimestamp },
  };
}
