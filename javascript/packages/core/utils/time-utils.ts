import { isNil } from 'lodash';

import { TimeZone } from '#core/types/time-types';

/**
 * Normalizes a timestamp to epoch seconds. Accepts epoch seconds — a number, or a numeric string
 * as k8s `Time.seconds` arrives in proto3 JSON — or an RFC 3339 string, as a
 * `google.protobuf.Timestamp` arrives in proto3 JSON. Returns NaN when neither parses.
 *
 * @example
 * - toEpochSeconds('1720656638') -> 1720656638
 * - toEpochSeconds('2024-07-11T00:10:38Z') -> 1720656638
 */
export function toEpochSeconds(timestamp: string | number): number {
  const seconds = Number(timestamp);
  return isNaN(seconds) ? Date.parse(String(timestamp)) / 1000 : seconds;
}

/**
 * Parses a proto3 JSON `google.protobuf.Duration` (e.g. "3600s", "1.5s") into seconds.
 * Returns undefined when the duration is absent or unparseable.
 *
 * @example
 * - durationToSeconds('3600s') -> 3600
 * - durationToSeconds(undefined) -> undefined
 */
export function durationToSeconds(duration?: string): number | undefined {
  const seconds = parseFloat(duration ?? '');
  return isNaN(seconds) ? undefined : seconds;
}

/**
 * Converts a timestamp (see {@link toEpochSeconds} for accepted forms) to a string.
 * If timezone kind is specified, it adjust the time and also adds a timezone info.
 *
 * @example
 * - timestampToString(1720656638) -> '2024/07/11 02:10:38'
 * - timestampToString('2024-07-11T00:10:38Z', 'utc') -> '2024/07/11 00:10:38 (UTC)'
 * - timestampToString(1720656639, 'utc') -> '2024/07/11 00:10:39 (UTC)'
 * - timestampToString(1720656639, 'local') -> '2024/07/11 02:10:39 (GMT+2)'
 */
export function timestampToString(
  timestampRaw?: string | number,
  timeZone?: TimeZone
): string | null {
  if (isNil(timestampRaw)) {
    return null;
  }

  const date = new Date(toEpochSeconds(timestampRaw) * 1000);
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
    timeZone: timeZone === TimeZone.UTC ? 'UTC' : undefined,
  });

  const formattedDate = formatter
    .format(date)
    .replace(/(\d+)\/(\d+)\/(\d+)/, '$3/$1/$2') // Convert MM/DD/YYYY to YYYY/MM/DD
    .replace(/,/g, ''); // Remove commas

  const timeZoneFormatter = new Intl.DateTimeFormat('en-US', {
    timeZoneName: 'short',
    timeZone: timeZone === TimeZone.UTC ? 'UTC' : undefined,
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
 * Formats the elapsed time between two timestamps (see {@link toEpochSeconds} for accepted forms).
 *
 * Returns null when either bound is missing — a step that has not started, or one
 * still running — so callers can render their own placeholder.
 *
 * @example
 * formatElapsedSeconds('1720656638', '1720656690') // "52s"
 * formatElapsedSeconds('2024-07-11T00:10:38Z', '2024-07-11T00:11:30Z') // "52s"
 * formatElapsedSeconds('1720656638', undefined) // null
 */
export function formatElapsedSeconds(
  startSeconds?: string | number,
  endSeconds?: string | number
): string | null {
  if (isNil(startSeconds) || isNil(endSeconds)) {
    return null;
  }

  const start = toEpochSeconds(startSeconds);
  const end = toEpochSeconds(endSeconds);

  if (isNaN(start) || isNaN(end)) {
    return null;
  }

  return `${Math.round(end - start)}s`;
}

/**
 * Parses an ISO string into date and time components.
 *
 * @param isoString - ISO date string like "2024-01-01T12:00:00.000Z"
 * @returns Object with compact date and time strings, or null if invalid
 *
 * @example
 * parseIsoString("2024-01-01T12:00:00.000Z")
 * // { date: "2024-01-01", time: "12:00:00" }
 *
 * parseIsoString("invalid") // null
 */
export function parseIsoString(isoString: string): { date: string; time: string } | null {
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
