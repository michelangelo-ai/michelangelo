import { safeStringify } from '#core/utils/string-utils';

import type { Row } from '@tanstack/react-table';
import type { TableData } from '../types/data-types';

/**
 * Global filter that performs "includes string" matching on any data type by converting
 * complex structures (objects, arrays) to JSON strings. This enables searching within
 * nested object properties and array elements that would otherwise be unsearchable.
 *
 * @example
 * // Searches within: { metadata: { type: 'urgent' } } or ['frontend', 'react']
 * globalFilterFn(row, 'metadata', 'urgent') // finds 'urgent' in JSON string
 */
export function globalFilterFn<T extends TableData = TableData>(
  row: Row<T>,
  columnId: string,
  filterValue: string
): boolean {
  if (!row.getValue(columnId)) return false;

  const result = safeStringify(row.getValue(columnId))
    .toLowerCase()
    .includes(filterValue.toLowerCase());

  return result;
}

globalFilterFn.autoRemove = (val: unknown) => val === undefined || val === null;
