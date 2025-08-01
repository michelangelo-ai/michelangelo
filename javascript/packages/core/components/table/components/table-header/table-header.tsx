import { StyledTableHead, StyledTableHeadRow } from 'baseui/table-semantic';

import { StyledTableHeadCell } from './styled-components';

import type { TableData } from '#core/components/table/types/data-types';
import type { TableHeaderProps } from './types';

export const TableHeader = <T extends TableData = TableData>({ headers }: TableHeaderProps<T>) => {
  return (
    <StyledTableHead>
      <StyledTableHeadRow>
        {headers.map((header) => (
          <StyledTableHeadCell key={header.id}>{header.content}</StyledTableHeadCell>
        ))}
      </StyledTableHeadRow>
    </StyledTableHead>
  );
};
