import { MULTI_COLUMN_DATA_JOIN_STRING } from '#core/components/table/constants';
import { ColumnConfig } from '#core/components/table/types/column-types';
import { resolveColumnForRow } from '#core/components/table/utils/column-resolution-utils';

import type { Row } from '@tanstack/react-table';
import type { TableData } from '#core/components/table/types/data-types';

/**
 * Extracts cell values for filtering from TanStack table rows.
 */
export function getCellValueForColumn<T extends TableData = TableData>(
  column: ColumnConfig<T>,
  row: Row<T>,
  columnId: string
): unknown {
  const effectiveColumn = resolveColumnForRow(column, row.original);
  const effectiveId = effectiveColumn.id || columnId;

  const rawValue = row.getValue<string | undefined>(effectiveId) ?? '';

  return 'items' in effectiveColumn
    ? (rawValue.split(MULTI_COLUMN_DATA_JOIN_STRING).at(0) ?? '')
    : rawValue;
}
