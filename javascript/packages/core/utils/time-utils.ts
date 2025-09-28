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

/**
 * Converts epoch seconds to a JavaScript Date object
 * @param epochSeconds - Unix timestamp in seconds
 * @returns Date object
 */
export function getDateFromEpochSeconds(epochSeconds: number): Date {
  return new Date(epochSeconds * 1000);
}

/**
 * Converts a JavaScript Date object to epoch seconds
 * @param date - Date object to convert
 * @returns Unix timestamp in seconds
 */
export function getEpochSecondsFromDate(date: Date): number {
  return Math.floor(date.getTime() / 1000);
}

/**
 * Parses an ISO string into date and time components.
 *
 * @param isoString - ISO date string like "2024-01-01T12:00:00.000Z"
 * @returns Object with compact date and time strings, or null if invalid
 *
 * @example
 * parseISOString("2024-01-01T12:00:00.000Z")
 * // { date: "2024-01-01", time: "12:00:00" }
 *
 * parseISOString("invalid") // null
 */
export function parseISOString(isoString: string): { date: string; time: string } | null {
  if (isNaN(Date.parse(isoString))) {
    return null;
  }

  // "2024-01-01T12:00:00.000Z" -> ["2024-01-01", "12:00:00.000Z"]
  const parts = isoString.split('T');
  if (parts.length !== 2) {
    return null;
  }

  return { date: parts[0], time: parts[1] };
}
