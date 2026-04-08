import { getDateFromEpochSeconds, getEpochSecondsFromDate } from '#core/utils/time-utils';
import { DateFormat } from './types';

import type { UseDateFormattersReturn } from './types';

/**
 * Returns `format` and `parse` functions that translate between date formats
 * (epoch seconds string or ISO string) and the Date objects expected by BaseUI's DatePicker.
 *
 * The format pipeline converts stored values to display values:
 *   Date format string → Date → timezone-adjusted Date
 *
 * The parse pipeline converts display values back to stored values:
 *   timezone-adjusted Date → UTC Date → Date format string
 *
 * @param dateFormat - Controls the persisted date format (epoch or ISO).
 */
export function useDateFormatters(
  dateFormat: DateFormat = DateFormat.ISO_DATE_STRING
): UseDateFormattersReturn {
  const toDate =
    dateFormat === DateFormat.EPOCH_SECONDS
      ? (value: string) => (value ? getDateFromEpochSeconds(parseInt(value)) : null)
      : (value: string) => (value ? new Date(value) : null);

  const fromDate =
    dateFormat === DateFormat.EPOCH_SECONDS
      ? (date: Date | null) => (date ? String(getEpochSecondsFromDate(date)) : '')
      : (date: Date | null) => (date ? date.toISOString() : '');

  return {
    format: (value: string) => translateUTCDateToUserTimezone(toDate(value)),
    parse: (value: Date | null) => fromDate(translateUserDateToUTC(value)),
  };
}

/**
 * Adjusts a UTC Date so its local representation matches the original UTC components
 */
function translateUTCDateToUserTimezone(date: Date | null): Date | null {
  if (!date) return null;

  const offsetMinutes = date.getTimezoneOffset();
  return new Date(date.getTime() + offsetMinutes * 60 * 1000);
}

/**
 * Adjusts a local-timezone Date so its UTC components match the original local display
 */
function translateUserDateToUTC(date: Date | null): Date | null {
  if (!date) return date;

  const offsetMinutes = date.getTimezoneOffset();
  return new Date(date.getTime() - offsetMinutes * 60 * 1000);
}
