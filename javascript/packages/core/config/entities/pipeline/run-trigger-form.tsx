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
 * The dropdown is populated from a `GetPipeline` fetch made when the dialog opens, rather
 * than from the record the action was opened with: the whole trigger definition is copied
 * into the created TriggerRun, so it should come from the pipeline's current manifest, not
 * from a row that may have been sitting in a stale list.
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
      // Only reachable if the manifest changed under an open dialog. The backend accepts a
      // TriggerRun without a schedule and the reconciler leaves it inert, so it would never
      // error on its own — surface it here instead. FormDialog catches onSubmit rejections
      // and renders them in the dialog, same as a failed mutation.
      throw new Error(
        `Trigger "${values.sourceTriggerName}" is no longer defined on this pipeline. ` +
          'Close this dialog and try again.'
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
 * the "Triggered by" column. The prefix mirrors how the reconciler will actually classify
 * the run (`GetTriggerType` in go/components/triggerrun/util.go, same priority order): a
 * batch rerun stays a batch rerun even with a backfill window set, while a backfill window
 * on a cron or interval trigger makes the run a backfill.
 */
function buildTriggerRunName(trigger: ManifestTrigger, isBackfill: boolean | undefined): string {
  const typePrefix = resolveTriggerRunTypePrefix(trigger, isBackfill);
  return `${typePrefix}${generateSuffix({ withDate: true })}`;
}

function resolveTriggerRunTypePrefix(
  trigger: ManifestTrigger,
  isBackfill: boolean | undefined
): string {
  if (trigger.triggerType?.case === 'batchRerun') return 'batch-rerun';
  if (isBackfill) return 'backfill';
  return trigger.triggerType?.case === 'intervalSchedule' ? 'interval' : 'cron';
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
