import React from 'react';

import { TableCellContent } from './table-cell-content';

import type { Row } from '@tanstack/react-table';
import type { TableData } from '#core/components/table/types/data-types';
import type { TableRow } from './types';

export function transformRows<T extends TableData = TableData>(
  tanstackRows: Row<T>[]
): TableRow<T>[] {
  return tanstackRows.map((row) => ({
    id: row.id,
    cells: row.getVisibleCells().map((cell, columnIndex) => ({
      id: cell.id,
      content: React.createElement(TableCellContent<T>, {
        cell,
        row,
        columnIndex,
      }),
    })),
    record: row.original,
    canSelect: row.getCanSelect(),
    isSelected: row.getIsSelected(),
    onToggleSelection: (selected: boolean) => row.toggleSelected(selected),
    canExpand: row.getCanExpand(),
    isExpanded: row.getIsExpanded(),
    onToggleExpanded: () => row.toggleExpanded(),
  }));
}
