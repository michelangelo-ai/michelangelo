import { isNil } from 'lodash';

import { UserTimeZone } from '#core/providers/user-provider/types';

/**
 * Converts a timestamp to a string.
 * If timezone kind is specified, it adjust the time and also adds a timezone info.
 *
 * @example
 * - timestampToString(1720656638) -> '2024/07/11 02:10:38'
 * - timestampToString(1720656639, 'utc') -> '2024/07/11 00:10:39 (UTC)'
 * - timestampToString(1720656639, 'local') -> '2024/07/11 02:10:39 (GMT+2)'
 */
export function timestampToString(
  timestampRaw?: string | number,
  timeZone?: UserTimeZone
): string | null {
  if (isNil(timestampRaw)) {
    return null;
  }

  const date = new Date(Number(timestampRaw) * 1000);
  if (isNaN(date.getTime())) {
    return 'Invalid date';
  }

  const formatter = new Intl.DateTimeFormat('en-US', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
    timeZone: timeZone === UserTimeZone.UTC ? 'UTC' : undefined,
  });

  const formattedDate = formatter
    .format(date)
    .replace(/(\d+)\/(\d+)\/(\d+)/, '$3/$1/$2') // Convert MM/DD/YYYY to YYYY/MM/DD
    .replace(/,/g, ''); // Remove commas

  const timeZoneFormatter = new Intl.DateTimeFormat('en-US', {
    timeZoneName: 'short',
    timeZone: timeZone === UserTimeZone.UTC ? 'UTC' : undefined,
  });

  const timeZoneString = timeZoneFormatter.format(date).split(' ').pop() ?? '';

  return `${formattedDate} (${timeZoneString})`;
}
