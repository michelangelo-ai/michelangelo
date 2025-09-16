import { getObjectValue } from '#core/utils/object-utils';
import { MULTI_COLUMN_DATA_JOIN_STRING } from '../constants';
import { resolveColumnForRow } from './column-resolution-utils';

import type { AccessorFn } from '#core/types/common/studio-types';
import type { ColumnConfig } from '../types/column-types';
import type { TableData } from '../types/data-types';

/**
 * Creates a standardized accessor function for extracting data from table rows.
 *
 * For multi-cell columns (columns with 'items'), the data required for each item
 * is concatenated with MULTI_COLUMN_DATA_JOIN_STRING for searchability.
 *
 * @example
 * ```tsx
 * const column = { id: 'name', accessor: 'user.profile.name' };
 * const accessorFn = normalizeColumnAccessor(column);
 * const value = accessorFn({ user: { profile: { name: 'John' } } }); // 'John'
 *
 * // For multi-cell columns:
 * const multiColumn = {
 *   id: 'info',
 *   items: [
 *     { id: 'name', accessor: 'name' },
 *     { id: 'age', accessor: 'age' }
 *   ]
 * };
 * const multiAccessor = normalizeColumnAccessor(multiColumn);
 * const value = multiAccessor({ name: 'John', age: 30 }); // 'John__JOIN__30'
 * ```
 */
export function normalizeColumnAccessor<T extends TableData = TableData>(
  column: ColumnConfig<T>
): AccessorFn {
  return (row: T) => {
    const resolvedColumn = resolveColumnForRow<T>(column, row);

    if ('items' in resolvedColumn) {
      return resolvedColumn.items
        .map((item) => getObjectValue(row, item.accessor ?? item.id))
        .join(MULTI_COLUMN_DATA_JOIN_STRING);
    }

    return getObjectValue<unknown>(row, resolvedColumn.accessor ?? resolvedColumn.id);
  };
}
