import React from 'react';
import { useStyletron } from 'baseui';
import { StyledTableBodyRow } from 'baseui/table-semantic';

import {
  StyledActionCell,
  StyledTableBody,
  StyledTableBodyCell,
} from '#core/components/table/styled-components';
import { getSelectionColumnCellStyles } from '../table-selection-column/styled-components';
import { TableSelectionColumn } from '../table-selection-column/table-selection-column';
import { withStickySides } from '../with-sticky-sides/with-sticky-sides';

import type { TableData } from '#core/components/table/types/data-types';
import type { TableBodyProps } from './types';

const StickySidesTableBodyRow = withStickySides(StyledTableBodyRow);

export const TableBody = <T extends TableData = TableData>({
  rows,
  enableRowSelection,
  enableStickySides,
  scrollRatio,
  subRow,
  actions,
}: TableBodyProps<T>) => {
  const [css, theme] = useStyletron();

  const lastVisibleColumnIndex = rows[0].cells.filter((cell) => cell.isVisible).length + 1;

  return (
    <StyledTableBody>
      {rows.map((row) => (
        <React.Fragment key={row.id}>
          <StickySidesTableBodyRow
            enableStickySides={enableStickySides}
            enableRowSelection={enableRowSelection}
            lastColumnIndex={lastVisibleColumnIndex}
            scrollRatio={scrollRatio}
            role="row"
          >
            {enableRowSelection && (
              <StyledTableBodyCell className={css(getSelectionColumnCellStyles(theme))}>
                <TableSelectionColumn
                  canSelect={row.canSelect ?? false}
                  isSelected={row.isSelected ?? false}
                  onToggleSelection={row.onToggleSelection ?? (() => undefined)}
                />
              </StyledTableBodyCell>
            )}

            {row.cells
              .filter((cell) => cell.isVisible)
              .map((cell, cellIndex) => (
                <StyledTableBodyCell
                  key={cell.id}
                  $columnNumber={cellIndex}
                  $enableRowSelection={enableRowSelection}
                >
                  {cell.content}
                </StyledTableBodyCell>
              ))}

            <StyledTableBodyCell>
              <StyledActionCell>
                {actions && React.createElement(actions, { row })}
              </StyledActionCell>
            </StyledTableBodyCell>
          </StickySidesTableBodyRow>

          {subRow && row.isExpanded && (
            <StyledTableBodyRow>
              <StyledTableBodyCell
                colSpan={
                  row.cells.filter((cell) => cell.isVisible).length + (enableRowSelection ? 2 : 1)
                }
              >
                {React.createElement(subRow, { row })}
              </StyledTableBodyCell>
            </StyledTableBodyRow>
          )}
        </React.Fragment>
      ))}
    </StyledTableBody>
  );
};
