import { parseISOString } from './time-utils';

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
  const uuid = crypto.randomUUID?.() ?? (() => {
    const h = Array.from(crypto.getRandomValues(new Uint8Array(16)), (b) => b.toString(16).padStart(2, '0'));
    return `${h.slice(0, 4).join('')}-${h.slice(4, 6).join('')}-${h.slice(6, 8).join('')}-${h.slice(8, 10).join('')}-${h.slice(10).join('')}`;
  })();
  const uuidSuffix = `${SUFFIX_DELIMITER}${uuid.substring(0, UUID_SUFFIX_LENGTH)}`;

  if (config.withDate) {
    const isoString = new Date().toISOString();
    const parsed = parseISOString(isoString);

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
