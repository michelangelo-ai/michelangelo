import React from 'react';
import { useStyletron } from 'baseui';
import { StyledTableBodyRow } from 'baseui/table-semantic';

import { StyledTableBody, StyledTableBodyCell } from '#core/components/table/styled-components';
import { TableExpandIcon } from '../table-expand-icon/table-expand-icon';
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

  const lastColumnIndex = rows[0].cells.length + 1;

  return (
    <StyledTableBody>
      {rows.map((row) => (
        <React.Fragment key={row.id}>
          <StickySidesTableBodyRow
            enableStickySides={enableStickySides}
            enableRowSelection={enableRowSelection}
            lastColumnIndex={lastColumnIndex}
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

            {row.cells.map((cell, cellIndex) => (
              <StyledTableBodyCell
                key={cell.id}
                $columnNumber={cellIndex}
                $enableRowSelection={enableRowSelection}
              >
                {row.canExpand && cellIndex === 0 ? (
                  <div
                    className={css({
                      display: 'flex',
                      alignItems: 'center',
                      cursor: 'pointer',
                      gap: theme.sizing.scale100,
                    })}
                    onClick={row.onToggleExpanded}
                  >
                    <TableExpandIcon expanded={row.isExpanded} />
                    {cell.content}
                  </div>
                ) : (
                  cell.content
                )}
              </StyledTableBodyCell>
            ))}

            <StyledTableBodyCell>
              {actions && React.createElement(actions, { row })}
            </StyledTableBodyCell>
          </StickySidesTableBodyRow>

          {subRow && row.isExpanded && (
            <StyledTableBodyRow>
              <StyledTableBodyCell colSpan={row.cells.length + (enableRowSelection ? 2 : 1)}>
                {React.createElement(subRow, { row })}
              </StyledTableBodyCell>
            </StyledTableBodyRow>
          )}
        </React.Fragment>
      ))}
    </StyledTableBody>
  );
};
