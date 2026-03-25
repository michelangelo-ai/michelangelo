import { useCellToString } from '#core/components/cell/use-cell-to-string';
import { getCellValueForColumn } from './get-cell-value-for-column';

import type { Row } from '@tanstack/react-table';
import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';
import type { FilterHook } from '../types';

/**
 * Factory hook that returns a builder function for categorical filters.
 *
 * @returns A function that takes a column and returns a FilterHook for categorical filtering
 */
export function useCategoricalFilterFactory<T extends TableData = TableData>(): (
  column: ColumnConfig<T>
) => FilterHook<T, unknown[]> {
  const cellToString = useCellToString();

  return (column: ColumnConfig<T>): FilterHook<T, unknown[]> => {
    const isFilterInactive = (filterValue: unknown[]) => {
      return !filterValue?.length;
    };

    const getActiveFilter = (filterValue: unknown[]) => {
      if (!Array.isArray(filterValue) || !filterValue?.length) return '';

      return filterValue.map((f) => cellToString({ record: {}, value: f, column })).join(', ');
    };

    const getFilterSummary = (filterValue: unknown[]) => {
      if (!Array.isArray(filterValue) || !filterValue?.length) return '';

      return `(${filterValue.length}) ${column.label ? `${column.label}: ` : ''}${filterValue
        .map((f) => cellToString({ record: {}, value: f, column }))
        .join(', ')}`;
    };

    const buildTableFilterFn = () => {
      const filterFn = (row: Row<T>, id: string, filterValue: unknown[]) => {
        if (isFilterInactive(filterValue)) {
          return true;
        }

        return filterValue.includes(getCellValueForColumn(column, row, id));
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
