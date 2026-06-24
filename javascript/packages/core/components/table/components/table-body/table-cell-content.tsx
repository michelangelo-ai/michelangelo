import React from 'react';
import { flexRender } from '@tanstack/react-table';
import { useStyletron } from 'baseui';

import { TableExpandIcon } from '../table-expand-icon/table-expand-icon';

import type { Cell, Row } from '@tanstack/react-table';
import type { TableData } from '../../types/data-types';

export function TableCellContent<T extends TableData = TableData>({
  cell,
  row,
  columnIndex,
}: {
  cell: Cell<T, unknown>;
  row: Row<T>;
  columnIndex: number;
}): React.ReactNode {
  const [css, theme] = useStyletron();

  // Handle grouped cells or expandable rows - show expand/collapse controls with cell content
  if (cell.getIsGrouped() || (row.getCanExpand() && columnIndex === 0)) {
    return (
      <div
        className={css({
          display: 'flex',
          alignItems: 'center',
          cursor: 'pointer',
          gap: theme.sizing.scale600,
        })}
        onClick={row.getToggleExpandedHandler()}
      >
        <TableExpandIcon expanded={row.getIsExpanded()} />
        {flexRender(cell.column.columnDef.cell, cell.getContext())}
      </div>
    );
  }

  if (cell.getIsAggregated()) {
    return flexRender(
      cell.column.columnDef.aggregatedCell ?? cell.column.columnDef.cell,
      cell.getContext()
    );
  }

  if (cell.getIsPlaceholder()) {
    return null;
  }

  return flexRender(cell.column.columnDef.cell, cell.getContext());
}
