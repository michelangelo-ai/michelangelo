import { MULTI_COLUMN_DATA_JOIN_STRING } from '#core/components/table/constants';
import { ColumnConfig } from '#core/components/table/types/column-types';
import { resolveColumnForRow } from '#core/components/table/utils/column-resolution-utils';
import { safeStringify } from '#core/utils/string-utils';

import type { Row } from '@tanstack/react-table';
import type { TableData } from '#core/components/table/types/data-types';
import type { FilterableRow } from '../types';

/**
 * Extracts cell values for filtering from table rows (TanStack Row or FilterableRow).
 * For multi-cell columns (columns with 'items'), returns only the first item's value.
 */
export function getCellValueForColumn<T extends TableData = TableData>(
  column: ColumnConfig<T>,
  row: Row<T> | FilterableRow<T>,
  columnId: string
): unknown {
  const record = 'original' in row ? row.original : row.record;
  const effectiveColumn = resolveColumnForRow<T>(column, record);
  const effectiveId = effectiveColumn.id || columnId;

  const rawValue = row.getValue(effectiveId) ?? '';

  if ('items' in effectiveColumn) {
    // For multi-cell columns, normalizeColumnAccessor should have created a joined string
    if (typeof rawValue !== 'string') {
      console.warn(
        `Expected string from normalizeColumnAccessor for multi-cell column ${effectiveId}, got:`,
        typeof rawValue,
        rawValue
      );
      return safeStringify(rawValue).split(MULTI_COLUMN_DATA_JOIN_STRING).at(0) ?? '';
    }

    return rawValue.split(MULTI_COLUMN_DATA_JOIN_STRING).at(0) ?? '';
  }

  return rawValue;
}
