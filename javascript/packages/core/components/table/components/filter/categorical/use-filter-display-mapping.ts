import { useMemo } from 'react';

import { useCellToString } from '#core/components/cell/use-cell-to-string';
import { safeStringify } from '#core/utils/string-utils';
import { getCellValueForColumn } from './get-cell-value-for-column';

import type { FilterableRow } from '#core/components/table/components/filter/types';
import type { ColumnConfig } from '#core/components/table/types/column-types';

export function useFilterDisplayMapping<TData>({
  preFilteredRows,
  column,
}: {
  preFilteredRows: FilterableRow<TData>[];
  column: ColumnConfig<TData>;
}) {
  const cellToString = useCellToString();

  return useMemo(() => {
    const displayToFilter: Record<string, unknown> = {};
    const filterToDisplay: Record<string, string> = {};

    preFilteredRows.forEach((row) => {
      const rawValue = getCellValueForColumn(column, row, column.id);
      if (rawValue == null) return;

      const displayValue = cellToString({ value: rawValue, record: row.record as object, column });

      // Preserve empty strings as valid filter options when cellToString returns null/undefined
      const finalDisplayValue = displayValue == null && rawValue === '' ? '' : displayValue;

      if (finalDisplayValue == null || finalDisplayValue in displayToFilter) return;

      const filterKey = safeStringify(rawValue);
      displayToFilter[finalDisplayValue] = rawValue;
      filterToDisplay[filterKey] = finalDisplayValue;
    });

    return [filterToDisplay, displayToFilter] as const;
  }, [preFilteredRows, cellToString, column]);
}
