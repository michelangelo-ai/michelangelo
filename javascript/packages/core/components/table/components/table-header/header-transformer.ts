import { flexRender } from '@tanstack/react-table';

import { TableData } from '../../types/data-types';

import type { Header } from '@tanstack/react-table';
import type { TableHeader } from './types';

/**
 * Transforms TanStack Table headers into Michelangelo's TableHeader format.
 */
export function transformHeaders<T extends TableData = TableData>(
  tanstackHeaders: Header<T, unknown>[]
): TableHeader[] {
  return tanstackHeaders.map((header) => ({
    id: header.id,
    content: flexRender(header.column.columnDef.header, header.getContext()),
    canSort: header.column.getCanSort(),
    onToggleSort: header.column.getToggleSortingHandler(),
    sortDirection: header.column.getIsSorted(),
  }));
}
