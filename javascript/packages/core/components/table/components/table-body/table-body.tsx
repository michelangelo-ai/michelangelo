import { useStyletron } from 'baseui';
import { StyledTableBodyCell, StyledTableBodyRow } from 'baseui/table-semantic';

import { StyledTableBody } from '#core/components/table/styled-components';
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
}: TableBodyProps<T>) => {
  const [css, theme] = useStyletron();

  const lastColumnIndex = rows[0].cells.length + 1;

  return (
    <StyledTableBody>
      {rows.map((row) => (
        <StickySidesTableBodyRow
          key={row.id}
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

          {row.cells.map((cell) => (
            <StyledTableBodyCell key={cell.id}>{cell.content}</StyledTableBodyCell>
          ))}

          {/* Placeholder action cell to align with column configuration button */}
          <StyledTableBodyCell />
        </StickySidesTableBodyRow>
      ))}
    </StyledTableBody>
  );
};
