import { parseIsoString } from './time-utils';

const SUFFIX_DELIMITER = '-';
const UUID_SUFFIX_LENGTH = 8;

/**
 * Generates a suffix to use when generating CRD names. Optionally includes a date suffix
 * before the UUID suffix. The UUID suffix is eight characters long.
 *
 * @example
 * ```ts
 * // Assuming the current date is 2024-01-01 12:00:00 UTC
 * generateSuffix() // returns '-abcd1234'
 * generateSuffix({ withDate: true }) // returns '-20240101-120000-abcd1234'
 *
 * // Usage when creating a pipeline run
 * const pipelineRunName = `run-${generateSuffix({ withDate: true })}`;
 * expect(pipelineRunName).toBe('run-20240101-120000-abcd1234');
 * ```
 */
export const generateSuffix = (config: { withDate: boolean } = { withDate: false }): string => {
  const uuidSuffix = `${SUFFIX_DELIMITER}${crypto.randomUUID().substring(0, UUID_SUFFIX_LENGTH)}`;

  if (config.withDate) {
    const isoString = new Date().toISOString();
    const parsed = parseIsoString(isoString);

    if (!parsed) {
      console.warn('Date.toISOString() returned an invalid ISO string', isoString);
      return uuidSuffix;
    }

    const { date, time } = parsed;
    const compactDate = date.replace(/-/g, ''); // "2024-01-01" -> "20240101"

    // Remove milliseconds/timezone: "12:00:00.000Z" -> "12:00:00" -> "120000"
    const compactTime = time.replace(/\..*$/, '').replace(/:/g, '');

    return `${SUFFIX_DELIMITER}${compactDate}-${compactTime}${uuidSuffix}`;
  }

  return uuidSuffix;
};

/**
 * Classifies the generated-name prefix for a trigger run from the schedule type it runs on
 * and whether it's a backfill, mirroring the priority order the reconciler itself uses to
 * classify a run (`GetTriggerType` in go/components/triggerrun/util.go): a batch rerun stays
 * a batch rerun even with a backfill window set, a backfill window on a cron or interval
 * trigger makes the run a backfill, and otherwise the run is named for its own schedule type.
 *
 * Shared by every place a TriggerRun name is generated — starting one fresh from a trigger's
 * manifest and replaying an existing run as a rerun both need the same classification.
 */
export function resolveTriggerRunTypePrefix(
  triggerTypeCase: string | undefined,
  isBackfill: boolean
): string {
  if (triggerTypeCase === 'batchRerun') return 'batch-rerun';
  if (isBackfill) return 'backfill';
  return triggerTypeCase === 'intervalSchedule' ? 'interval' : 'cron';
}
