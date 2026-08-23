import { BooleanField } from '#core/components/form/fields/boolean/boolean-field';
import { DateField } from '#core/components/form/fields/date/date-field';
import { DateFormat } from '#core/components/form/fields/date/types';
import { NumberField } from '#core/components/form/fields/number/number-field';
import { RadioField } from '#core/components/form/fields/radio/radio-field';
import { SelectField } from '#core/components/form/fields/select/select-field';
import { useField } from '#core/components/form/hooks/use-field';

import type { SelectOption } from '#core/components/form/fields/select/types';
import type { ManifestTrigger } from '#core/config/entities/trigger/types';

/**
 * Fields whose behavior depends on the currently-selected trigger and the backfill toggle,
 * split out so those reads don't force the whole dialog (including the async trigger fetch)
 * to re-render on every keystroke.
 *
 * `autoFlip` is shown but always disabled — that feature hasn't launched yet — so the form
 * always sends `autoFlip: false` (see run-trigger-form.tsx) regardless of the selected radio.
 */
export function RunTriggerFields({ triggerMap }: { triggerMap: Record<string, ManifestTrigger> }) {
  const { input: sourceTriggerNameField } = useField<string>('sourceTriggerName');
  const { input: isBackfillField } = useField<boolean>('isBackfill');
  const { input: startTimestampField } = useField<string>('startTimestamp');
  const { input: endTimestampField } = useField<string>('endTimestamp');

  const selectedTrigger = triggerMap[sourceTriggerNameField.value];
  const isBackfill = !!isBackfillField.value;

  const parameterOptions: SelectOption<string>[] = Object.keys(
    selectedTrigger?.parametersMap ?? {}
  ).map((paramId) => ({ id: paramId, label: paramId }));

  return (
    <>
      <RadioField
        name="autoFlip"
        label="Automatically switch to the latest revision once changes are applied? (Coming soon)"
        caption="Coming soon — not yet available."
        options={[
          { value: true, label: 'Yes' },
          { value: false, label: 'No' },
        ]}
        initialValue={false}
        disabled
      />

      <BooleanField name="isBackfill" checkboxLabel="Is this a backfill run?" toggle />

      {isBackfill ? (
        <>
          <DateField
            name="startTimestamp"
            label="Execution start date & time"
            dateFormat={DateFormat.EPOCH_SECONDS}
            noFutureDate
            required
            validate={(value) =>
              endTimestampField.value && Number(value) > Number(endTimestampField.value)
                ? 'Start date & time must be before end date & time.'
                : undefined
            }
          />

          <DateField
            name="endTimestamp"
            label="Execution end date & time"
            dateFormat={DateFormat.EPOCH_SECONDS}
            noFutureDate
            required
            validate={(value) =>
              startTimestampField.value && Number(value) < Number(startTimestampField.value)
                ? 'End date & time must be after start date & time.'
                : undefined
            }
          />

          <SelectField
            name="selectedParams"
            label="Parameter IDs"
            multi
            options={parameterOptions}
            caption="Selecting no parameters runs every parameter."
          />

          <NumberField
            name="maxConcurrencyOverride"
            label="Max concurrency"
            initialValue={selectedTrigger?.maxConcurrency}
            description="Overrides the trigger's default max concurrency for this run only."
          />
        </>
      ) : null}
    </>
  );
}
