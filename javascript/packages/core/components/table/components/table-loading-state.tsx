import { Skeleton } from 'baseui/skeleton';

import {
  StyledTableBody,
  StyledTableBodyCell,
  StyledTableBodyRow,
} from '#core/components/table/styled-components';

export function TableLoadingState() {
  return (
    <StyledTableBody aria-label="Loading">
      {[1, 2, 3].map((row) => (
        <StyledTableBodyRow key={row}>
          <StyledTableBodyCell colSpan={100}>
            <Skeleton animation width="100%" height="22px" />
          </StyledTableBodyCell>
        </StyledTableBodyRow>
      ))}
    </StyledTableBody>
  );
}
