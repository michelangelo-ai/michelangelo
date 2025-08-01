import { DatetimeColumn } from 'baseui/data-table';

import { ColumnConfig } from '#core/components/table/types/column-types';
import { TableData } from '#core/components/table/types/data-types';
import { getDateFromEpochSeconds } from '#core/utils/time-utils';
import { getCellValueForColumn } from '../categorical/get-cell-value-for-column';

import type { Row } from '@tanstack/react-table';
import type { FilterHook } from '../types';
import type { DatetimeFilterValue } from './types';

/**
 * Factory hook that returns a builder function for datetime filters.
 *
 * @returns A function that takes a column and returns a FilterHook for datetime filtering
 */
export function useDatetimeFilterFactory<T extends TableData = TableData>(): (
  column: ColumnConfig<T>
) => FilterHook<T, DatetimeFilterValue> {
  return (column: ColumnConfig<T>): FilterHook<T, DatetimeFilterValue> => {
    const convertStringParamsToDate = (filterValue: DatetimeFilterValue): DatetimeFilterValue => {
      return {
        ...filterValue,
        range: filterValue?.range?.map((a) => new Date(a)) || [],
      };
    };

    const isFilterInactive = (filterValue: DatetimeFilterValue): boolean => {
      return !filterValue?.range?.length && !filterValue?.selection?.length;
    };

    const getActiveFilter = (filterValue: DatetimeFilterValue): string => {
      if (isFilterInactive(filterValue)) return '';
      return filterValue?.description ?? '';
    };

    const getFilterSummary = (filterValue: DatetimeFilterValue): string => {
      if (isFilterInactive(filterValue)) return '';
      return `${column.label ? `${column.label}: ` : ''}${getActiveFilter(filterValue)}`;
    };

    const buildTableFilterFn = () => {
      const filterFn = (row: Row<T>, id: string, filterValue: DatetimeFilterValue) => {
        // We don't support single date filters
        if (isFilterInactive(filterValue) || filterValue.range.length === 1) {
          return true;
        }

        // When column filters are persisted in local storage, they are transformed to strings
        const convertedFilterValue = filterValue.range.some((value) => typeof value === 'string')
          ? convertStringParamsToDate(filterValue)
          : filterValue;

        const cellValue = getCellValueForColumn(column, row, id);

        const cellDate = getDateFromEpochSeconds(
          typeof cellValue === 'number' ? cellValue : parseFloat(cellValue as string)
        );

        return DatetimeColumn({
          title: '',
          mapDataToValue: () => new Date(),
          // @ts-expect-error Michelangelo DatetimeFilterValue does not match BaseUI's FilterParameters type
        }).buildFilter(convertedFilterValue)(cellDate);
      };

      filterFn.autoRemove = isFilterInactive;
      return filterFn;
    };

    return {
      isFilterInactive,
      getActiveFilter,
      getFilterSummary,
      buildTableFilterFn,
    };
  };
}
