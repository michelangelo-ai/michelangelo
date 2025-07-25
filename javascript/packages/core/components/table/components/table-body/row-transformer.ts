import { flexRender } from '@tanstack/react-table';

import type { Row } from '@tanstack/react-table';
import type { TableData } from '#core/components/table/types/data-types';
import type { TableRow } from './types';

export function transformRows<T extends TableData = TableData>(
  tanstackRows: Row<T>[]
): TableRow<T>[] {
  return tanstackRows.map((row) => ({
    id: row.id,
    cells: row.getVisibleCells().map((cell) => ({
      id: cell.id,
      content: flexRender(cell.column.columnDef.cell, cell.getContext()),
    })),
  }));
}
