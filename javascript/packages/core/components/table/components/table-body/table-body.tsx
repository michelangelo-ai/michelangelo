import { StyledTableBodyCell, StyledTableBodyRow } from 'baseui/table-semantic';

import { StyledTableBody } from '#core/components/table/styled-components';

import type { TableData } from '#core/components/table/types/data-types';
import type { TableBodyProps } from './types';

export const TableBody = <T extends TableData = TableData>({ rows }: TableBodyProps<T>) => {
  return (
    <StyledTableBody>
      {rows.map((row) => (
        <StyledTableBodyRow key={row.id}>
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
