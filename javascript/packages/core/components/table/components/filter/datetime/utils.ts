import type { DatetimeFilterValue } from './types';

/**
 * Converts string dates to Date objects. This is needed because the filter value is
 * stored as a string in localStorage.
 *
 * @param filterValue - The filter value to conversion
 * @returns The converted filter value
 */
export const convertStringParamsToDate = (
  filterValue: DatetimeFilterValue | undefined
): DatetimeFilterValue => {
  if (!filterValue) {
    return { operation: '', range: [], selection: [], description: '', exclude: false };
  }

  return {
    ...filterValue,
    range: filterValue?.range?.map((a) => new Date(a)) ?? [],
  };
};
