import { getObjectValue } from '#core/utils/object-utils';

import type { AccessorFn } from '#core/types/common/studio-types';
import type { TableColumn } from '../types/column-types';
import type { TableData } from '../types/data-types';

/**
 * Creates a standardized accessor function for extracting data from table rows.
 *
 * @example
 * ```tsx
 * const column = { id: 'name', accessor: 'user.profile.name' };
 * const accessorFn = normalizeColumnAccessor(column);
 * const value = accessorFn({ user: { profile: { name: 'John' } } }); // 'John'
 * ```
 */
export function normalizeColumnAccessor<T extends TableData = TableData>(
  column: TableColumn<T>
): AccessorFn<T> {
  return (row) => {
    return getObjectValue<T>(row, column.accessor ?? column.id);
  };
}
