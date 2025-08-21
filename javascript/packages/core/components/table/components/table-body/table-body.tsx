import { useStyletron } from 'baseui';
import { StyledTableBodyCell, StyledTableBodyRow } from 'baseui/table-semantic';

import { StyledTableBody } from '#core/components/table/styled-components';
import { getSelectionColumnCellStyles } from '../table-selection-column/styled-components';
import { TableSelectionColumn } from '../table-selection-column/table-selection-column';

import type { TableData } from '#core/components/table/types/data-types';
import type { TableBodyProps } from './types';

export const TableBody = <T extends TableData = TableData>({
  rows,
  enableRowSelection,
}: TableBodyProps<T>) => {
  const [css, theme] = useStyletron();

  return (
    <StyledTableBody>
      {rows.map((row) => (
        <StyledTableBodyRow key={row.id}>
          {enableRowSelection && (
            <StyledTableBodyCell className={css(getSelectionColumnCellStyles(theme))}>
              <TableSelectionColumn
                canSelect={row.canSelect ?? false}
                isSelected={row.isSelected ?? false}
                onToggleSelection={row.onToggleSelection ?? (() => undefined)}
              />
            </StyledTableBodyCell>
          )}

          {row.cells.map((cell) => (
            <StyledTableBodyCell key={cell.id}>{cell.content}</StyledTableBodyCell>
          ))}

          {/* Placeholder action cell to align with column configuration button */}
          <StyledTableBodyCell />
        </StyledTableBodyRow>
      ))}
    </StyledTableBody>
  );
};
